package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/anthropic"
	"veloxmesh/internal/providers/gemini"
	"veloxmesh/internal/providers/openai"
)

type App struct {
	Config                 *config.Config
	Logger                 *slog.Logger
	Router                 http.Handler
	RuntimeProviderManager *controlstate.RuntimeProviderManager
	HotState               hotstate.Client
}

func (a *App) HealthStore() health.Store {
	return a.RuntimeProviderManager.HealthStore()
}

func New() (*App, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger := observability.SetupLogger(cfg.LogLevel)

	var hotStateClient hotstate.Client
	if cfg.RedisEnabled {
		redisClient, err := hotstate.NewRedisClient(context.Background(), cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisNamespace)
		if err != nil {
			if cfg.RedisDegradeToLocal {
				logger.Warn("redis unavailable; degrading to process-local hot state", "error", err)
				hotStateClient = hotstate.NewLocalHotState()
			} else {
				return nil, fmt.Errorf("failed to initialize redis: %w", err)
			}
		} else {
			hotStateClient = redisClient
		}
	} else {
		hotStateClient = hotstate.NewLocalHotState()
	}

	var healthStore health.Store
	if cfg.RedisEnabled && hotStateClient != nil {
		healthStore = health.NewRedisStore(hotStateClient, cfg.RedisHealthTTL)
	} else {
		healthStore = health.NewInMemoryStore()
	}

	m := controlstate.NewRuntimeProviderManager(cfg, logger, healthStore)

	var repo controlstate.Repository
	var cipher controlstate.SecretCipher
	ctx := context.Background()

	if cfg.ControlStateBackend != "disabled" {
		cipher, err = controlstate.NewAESGCMSecretCipher([]byte(cfg.ControlStateEncryptionKey), "v1")
		if err != nil {
			return nil, fmt.Errorf("failed to initialize secret cipher: %w", err)
		}

		if cfg.ControlStateBackend == "sqlite" {
			repo, err = sqlite.Open(cfg.ControlStateDSN)
		} else if cfg.ControlStateBackend == "postgres" {
			repo, err = postgres.Open(ctx, cfg.ControlStateDSN)
		} else {
			return nil, fmt.Errorf("unknown control state backend: %s", cfg.ControlStateBackend)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to open repository: %w", err)
		}

		if cfg.ControlStateMigrateOnStartup {
			if migrator, ok := repo.(interface{ Migrate(context.Context) error }); ok {
				if err := migrator.Migrate(ctx); err != nil {
					return nil, fmt.Errorf("failed to run migrations: %w", err)
				}
			}
		}

		if cfg.ControlStateLocalSeedEnabled {
			options := controlstate.SeedOptions{
				Enabled:       true,
				EncryptionKey: cfg.ControlStateEncryptionKey,
			}
			if err := controlstate.SeedFromStaticConfig(ctx, repo, cfg, cipher, options); err != nil {
				return nil, fmt.Errorf("failed to seed config: %w", err)
			}
		}
	}

	if cfg.ControlStateBackend == "disabled" {
		var adapters []providers.ProviderAdapter
		for _, p := range cfg.Providers {
			switch p.Type {
			case "openai-compatible":
				adapters = append(adapters, openai.NewAdapter(p.ID, p.BaseURL, p.ResolveAPIKey(), strings.Join(p.Models, ",")))
			case "anthropic":
				adapters = append(adapters, anthropic.NewAdapter(p.ID, p.BaseURL, p.ResolveAPIKey(), strings.Join(p.Models, ",")))
			case "gemini":
				adapters = append(adapters, gemini.NewAdapter(p.ID, p.BaseURL, p.ResolveAPIKey(), strings.Join(p.Models, ",")))
			}
		}
		if err := m.ActivateStatic(cfg.Providers, adapters); err != nil {
			return nil, fmt.Errorf("failed to initialize static providers: %w", err)
		}
	}

	var admissionCtrl admission.Controller
	if repo != nil {
		admissionCtrl = admission.NewCreditAdmissionController(repo)
	} else {
		admissionCtrl = admission.NewPassThroughController()
	}

	var semanticCache *cache.SemanticCacheService
	if cfg.SemanticCacheEnabled && repo != nil && cfg.SemanticCacheProvider != "" {
		if snapshot := m.Snapshot(); snapshot != nil && snapshot.Registry != nil {
			adapter, err := snapshot.Registry.Get(cfg.SemanticCacheProvider)
			if err == nil {
				if embedAdapter, ok := adapter.(providers.EmbedAdapter); ok {
					semanticCache = cache.NewSemanticCacheService(cache.SemanticCacheConfig{
						Enabled:       true,
						Threshold:     0.9,
						MaxCandidates: 10,
						TTL:           24 * time.Hour,
					}, repo.SemanticCache(), embedAdapter)
				} else {
					logger.Warn("semantic cache provider is not an embed adapter", "provider", cfg.SemanticCacheProvider)
				}
			} else {
				logger.Warn("semantic cache provider not found", "provider", cfg.SemanticCacheProvider)
			}
		} else {
			logger.Warn("cannot initialize semantic cache: provider registry not ready")
		}
	}

	gatewaySvc := gateway.NewService(m, admissionCtrl, m.HealthStore(), cfg.FallbackEnabled, cfg.MaxAttempts, repo, semanticCache)

	r := router.NewRouter(cfg, gatewaySvc, nil, hotStateClient, repo)

	application := &App{
		Config:                 cfg,
		Logger:                 logger,
		Router:                 r,
		RuntimeProviderManager: m,
		HotState:               hotStateClient,
	}

	if cfg.ControlStateBackend != "disabled" {
		if err := application.ReloadProviders(ctx, repo, cipher); err != nil {
			return nil, fmt.Errorf("failed initial provider reload: %w", err)
		}
		if err := application.StartConfigChangeSubscriber(ctx, repo, cipher); err != nil {
			return nil, fmt.Errorf("failed to start config subscriber: %w", err)
		}
	}

	return application, nil
}

func (a *App) ReloadProviders(ctx context.Context, repo controlstate.Repository, cipher controlstate.SecretCipher) error {
	records, err := controlstate.LoadActiveProviderRecords(ctx, repo.Providers())
	if err != nil {
		return fmt.Errorf("failed to load active provider records: %w", err)
	}

	var rCfg *controlstate.RoutingConfig
	if repo.Routing() != nil {
		rCfg, err = repo.Routing().Get(ctx)
		if err != nil && err != controlstate.ErrRoutingConfigNotFound {
			return fmt.Errorf("failed to load routing config: %w", err)
		}
		// If ErrRoutingConfigNotFound, rCfg stays nil, meaning no routing config applied yet.
	}

	secrets := make(map[string]string)
	for _, r := range records {
		if !r.Enabled {
			continue
		}
		if !r.Secret.SecretConfigured {
			return fmt.Errorf("provider %s has no secret configured", r.ID)
		}
		ciphertext, nonce, keyID, err := repo.Providers().GetEncryptedSecret(ctx, r.ID)
		if err != nil {
			return fmt.Errorf("failed to get encrypted secret for %s: %w", r.ID, err)
		}
		decrypted, err := cipher.DecryptProviderSecret(&controlstate.EncryptedSecret{
			Ciphertext: ciphertext,
			Nonce:      nonce,
			KeyID:      keyID,
		})
		if err != nil {
			return fmt.Errorf("failed to decrypt secret for %s: %w", r.ID, err)
		}
		secrets[r.ID] = string(decrypted)
	}

	return a.RuntimeProviderManager.ActivateDurable(ctx, records, secrets, rCfg, nil)
}

func (a *App) StartConfigChangeSubscriber(ctx context.Context, repo controlstate.Repository, cipher controlstate.SecretCipher) error {
	sub, err := a.HotState.SubscribeConfigChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to config changes: %w", err)
	}

	a.Logger.Info("starting config change subscriber")

	go func() {
		defer sub.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-sub.Channel():
				if msg == nil {
					return
				}
				a.Logger.Info("received config change notification", "provider_id", msg.ProviderID, "action", msg.Action, "revision", msg.Revision)
				if err := a.ReloadProviders(ctx, repo, cipher); err != nil {
					a.Logger.Error("failed to reload providers on config change", "error", err)
				}
			}
		}
	}()

	return nil
}

func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("starting gateway", "addr", a.Config.GatewayDataAddr)

	a.RuntimeProviderManager.Start(ctx)

	errChan := make(chan error, 1)
	go func() {
		errChan <- http.ListenAndServe(a.Config.GatewayDataAddr, a.Router)
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
