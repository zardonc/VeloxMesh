package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
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

	if cfg.ControlStateBackend == "disabled" {
		if err := m.ActivateStatic(cfg.Providers, adapters); err != nil {
			return nil, fmt.Errorf("failed to initialize static providers: %w", err)
		}
	}

	admissionCtrl := admission.NewPassThroughController()
	gatewaySvc := gateway.NewService(m, admissionCtrl, m.HealthStore(), cfg.FallbackEnabled, cfg.MaxAttempts)

	r := router.NewRouter(cfg, gatewaySvc, nil, hotStateClient)

	return &App{
		Config:                 cfg,
		Logger:                 logger,
		Router:                 r,
		RuntimeProviderManager: m,
		HotState:               hotStateClient,
	}, nil
}

func (a *App) ReloadProviders(ctx context.Context, repo controlstate.Repository, cipher controlstate.SecretCipher) error {
	records, err := controlstate.LoadActiveProviderRecords(ctx, repo.Providers())
	if err != nil {
		return fmt.Errorf("failed to load active provider records: %w", err)
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

	return a.RuntimeProviderManager.ActivateProviderSet(ctx, records, secrets, nil)
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
