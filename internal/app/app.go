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
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/anthropic"
	"veloxmesh/internal/providers/gemini"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/storage"
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
		admissionCtrl = admission.NewLimitAdmissionController(repo, hotStateClient)
	} else {
		admissionCtrl = admission.NewPassThroughController()
	}

	var semanticCache *cache.SemanticCacheService
	if cfg.SemanticCacheEnabled && repo != nil && cfg.SemanticCacheProvider != "" {
		if snapshot := m.Snapshot(); snapshot != nil && snapshot.Registry != nil {
			adapter, err := snapshot.Registry.Get(cfg.SemanticCacheProvider)
			if err == nil {
				if embedAdapter, ok := adapter.(providers.EmbedAdapter); ok {
					var vectorAdapter storage.VectorAdapter
					if cfg.SemanticCacheVectorStore == "lancedb" {
						lancedbAdapter, err := storage.NewLanceDBVectorAdapter("data/lancedb")
						if err != nil {
							logger.Warn("failed to initialize LanceDB (Plan 3 Edge only); vector capabilities degraded", "error", err)
							vectorAdapter = storage.NewDegradedVectorAdapter()
						} else {
							vectorAdapter = lancedbAdapter
						}
					} else if cfg.SemanticCacheVectorStore == "qdrant" {
						qdrantAdapter, err := storage.NewQdrantVectorAdapter(cfg.QdrantAddr, cfg.QdrantAPIKey)
						if err != nil {
							logger.Warn("failed to initialize Qdrant; evaluating fallback", "error", err)
							if cfg.RedisEnabled {
								redisVSSAdapter, fallbackErr := storage.NewRedisVSSVectorAdapter(context.Background(), cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisNamespace)
								if fallbackErr != nil {
									logger.Warn("failed to initialize Redis VSS fallback; vector capabilities degraded", "error", fallbackErr)
									vectorAdapter = storage.NewDegradedVectorAdapter()
								} else {
									logger.Info("activated Redis VSS fallback for vector store")
									vectorAdapter = redisVSSAdapter
								}
							} else {
								vectorAdapter = storage.NewDegradedVectorAdapter()
							}
						} else {
							vectorAdapter = qdrantAdapter
						}
					} else {
						vectorAdapter = storage.NewNoopVectorAdapter()
					}
					
					semanticCache = cache.NewSemanticCacheService(cache.SemanticCacheConfig{
						Enabled:       true,
						Threshold:     0.9,
						MaxCandidates: 10,
						TTL:           24 * time.Hour,
					}, repo.SemanticCache(), vectorAdapter, embedAdapter)
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

	var adminProvHandler *handlers.AdminProvidersHandler
	var adminCombosHandler *handlers.AdminCombosHandler
	var adminSemanticRulesHandler *handlers.AdminSemanticRulesHandler
	if repo != nil {
		adminSvc := controlstate.NewAdminProviderService(repo, cipher, m, hotStateClient)
		adminProvHandler = handlers.NewAdminProvidersHandler(adminSvc)

		adminComboSvc := controlstate.NewAdminComboService(repo, m, cipher, hotStateClient)
		adminCombosHandler = handlers.NewAdminCombosHandler(adminComboSvc)
		
		adminSemanticRulesSvc := controlstate.NewAdminSemanticRulesService(repo, hotStateClient)
		adminSemanticRulesHandler = handlers.NewAdminSemanticRulesHandler(adminSemanticRulesSvc)
	}

	gatewaySvc := gateway.NewService(m, admissionCtrl, m.HealthStore(), cfg.FallbackEnabled, cfg.MaxAttempts, repo, semanticCache, pipeline.DefaultRegistry(), m, hotStateClient)

	r := router.NewRouter(cfg, gatewaySvc, adminProvHandler, adminCombosHandler, adminSemanticRulesHandler, hotStateClient, repo)

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

	var combos []providers.Combo
	if repo.Combos() != nil {
		enabled := true
		comboRecords, err := repo.Combos().List(ctx, controlstate.ComboFilter{Enabled: &enabled})
		if err != nil {
			return fmt.Errorf("failed to load combos: %w", err)
		}
		for _, rec := range comboRecords {
			combos = append(combos, providers.Combo{
				ID:       rec.ID,
				Name:     rec.Name,
				Strategy: rec.Strategy,
				Members:  rec.Members,
				Judge:    rec.Judge,
			})
		}
	}

	var semRules *controlstate.SemanticRuleSnapshot
	if repo.SemanticRules() != nil {
		global, err := repo.SemanticRules().GetGlobalDefaults(ctx)
		if err != nil {
			a.Logger.Error("failed to load global semantic rules", "error", err)
		} else {
			users, err := repo.SemanticRules().ListUserConfigs(ctx)
			if err != nil {
				a.Logger.Error("failed to load user semantic rules", "error", err)
			} else {
				semRules = &controlstate.SemanticRuleSnapshot{
					Global: global,
					Users:  users,
				}
			}
		}
	}

	return a.RuntimeProviderManager.ActivateDurable(ctx, records, secrets, rCfg, combos, semRules, nil)
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
				a.Logger.Info("received config change notification", "type", msg.Type, "target_id", msg.TargetID, "action", msg.Action, "revision", msg.Revision)
				
				switch msg.Type {
				case hotstate.EventProvider, hotstate.EventCombo:
					if err := a.ReloadProviders(ctx, repo, cipher); err != nil {
						a.Logger.Error("failed to reload providers on config change", "error", err)
					}
				case hotstate.EventSemanticRules:
					if err := a.ReloadSemanticRules(ctx, repo); err != nil {
						a.Logger.Error("failed to reload semantic rules on config change", "error", err)
					}
				case hotstate.EventAPIKey:
					// Invalidate hot cache for API key
					if err := a.HotState.Delete(ctx, hotstate.NamespacedKey(a.Config.RedisNamespace, "auth", msg.TargetID)); err != nil {
						a.Logger.Error("failed to invalidate api key cache", "error", err)
					}
				case hotstate.EventLimitRule:
					a.Logger.Info("limit rule changed, no in-memory reload needed")
				case hotstate.EventVectorPolicy:
					a.Logger.Info("vector policy changed, requires restart for now")
				default:
					a.Logger.Warn("unknown event type, falling back to full reload", "type", msg.Type)
					if err := a.ReloadProviders(ctx, repo, cipher); err != nil {
						a.Logger.Error("failed to reload providers on unknown config change", "error", err)
					}
				}
			}
		}
	}()

	return nil
}

func (a *App) ReloadSemanticRules(ctx context.Context, repo controlstate.Repository) error {
	if repo.SemanticRules() == nil {
		return nil
	}

	global, err := repo.SemanticRules().GetGlobalDefaults(ctx)
	if err != nil {
		return fmt.Errorf("failed to load global semantic rules: %w", err)
	}

	users, err := repo.SemanticRules().ListUserConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to load user semantic rules: %w", err)
	}

	semRules := &controlstate.SemanticRuleSnapshot{
		Global: global,
		Users:  users,
	}

	a.RuntimeProviderManager.UpdateSemanticRules(semRules)
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
