package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/controlstate/replication"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/coordination"
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
	"veloxmesh/internal/scheduler"

	"github.com/redis/go-redis/v9"
)

type App struct {
	Config                       *config.Config
	Logger                       *slog.Logger
	Router                       http.Handler
	RuntimeProviderManager       *controlstate.RuntimeProviderManager
	HotState                     hotstate.Client
	Coordinator                  coordination.Coordinator
	ShutdownTracing              func(context.Context) error
	SchedulerRunner              *scheduler.SynchronousRunner
	SchedulerQueueBackend        string
	SchedulerFeedbackOn          bool
	SchedulerSemanticNeighborsOn bool
}

const (
	controlRedisDialTimeout     = 100 * time.Millisecond
	controlRedisReadTimeout     = 1500 * time.Millisecond
	controlRedisWriteTimeout    = 500 * time.Millisecond
	controlRedisMaxRetries      = 1
	controlRedisMinRetryBackoff = 50 * time.Millisecond
	controlRedisMaxRetryBackoff = 100 * time.Millisecond
)

func newControlRedisClient(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:                  cfg.RedisAddr,
		Password:              cfg.RedisPassword,
		DB:                    cfg.RedisDB,
		DialTimeout:           controlRedisDialTimeout,
		ReadTimeout:           controlRedisReadTimeout,
		WriteTimeout:          controlRedisWriteTimeout,
		MaxRetries:            controlRedisMaxRetries,
		MinRetryBackoff:       controlRedisMinRetryBackoff,
		MaxRetryBackoff:       controlRedisMaxRetryBackoff,
		ContextTimeoutEnabled: true,
	})
}

func (a *App) HealthStore() health.Store {
	return a.RuntimeProviderManager.HealthStore()
}

func newSchedulerRunner(ctx context.Context, cfg *config.Config, hotState hotstate.Client, logger *slog.Logger, recorder *scheduler.TrainingRecorder, quality *scheduler.PredictionQualityRecorder, rollout *scheduler.SchedulerRolloutController, semanticNeighbors scheduler.SemanticNeighborEnricher) (*scheduler.SynchronousRunner, string) {
	queue, backend := newSchedulerQueue(ctx, cfg, logger)
	scorer, err := scheduler.NewScorerWithController(ctx, cfg.Scheduler, rollout)
	if err != nil {
		logger.Warn("scheduler scorer unavailable; using FIFO fallback", "error", err)
		scorer = scheduler.FIFOScorer{Reason: "disabled"}
	}
	observability.DefaultMetrics.RecordSchedulerBreakerState("closed")
	registry := scheduler.NewResultRegistry()
	intake := &scheduler.TaskIntake{
		Queue:    queue,
		Guard:    scheduler.QueueGuard{SoftLimit: int64(cfg.Scheduler.QueueSoftLimit), HardLimit: int64(cfg.Scheduler.QueueHardLimit)},
		Scorer:   scorer,
		Registry: registry,
		Priority: scheduler.NewPriorityResolver(hotState),
		Policy: scheduler.PriorityPolicy{
			Default:            scheduler.NormalizePriority(cfg.Scheduler.DefaultPriority),
			Max:                scheduler.NormalizePriority(cfg.Scheduler.MaxPriority),
			HighQuotaPerMinute: int64(cfg.Scheduler.HighQuotaPerMinute),
			Strict:             cfg.Scheduler.Strict,
		},
		Metrics:           observability.DefaultMetrics,
		Backend:           backend,
		SemanticNeighbors: semanticNeighbors,
	}
	if timeout, err := time.ParseDuration(cfg.Scheduler.SemanticNeighborsTaskTimeout); err == nil {
		intake.SemanticNeighborTaskTimeout = timeout
	}
	executor := &scheduler.Executor{Queue: queue, Registry: registry, Metrics: observability.DefaultMetrics}
	runner := scheduler.NewSynchronousRunner(intake, executor, registry)
	runner.Recorder = recorder
	runner.Quality = quality
	if indexer, ok := semanticNeighbors.(scheduler.SemanticNeighborIndexer); ok {
		runner.Indexer = indexer
	}
	return runner, backend
}

func newSchedulerQueue(ctx context.Context, cfg *config.Config, logger *slog.Logger) (scheduler.QueueBackend, string) {
	memoryQueue := scheduler.NewMemoryQueue()
	backend := strings.ToLower(cfg.Scheduler.QueueBackend)
	if backend == "memory" || !cfg.RedisEnabled {
		return memoryQueue, "memory"
	}
	redisClient := newControlRedisClient(cfg)
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("scheduler redis queue unavailable; using memory queue", "error", err)
		_ = redisClient.Close()
		return memoryQueue, "memory"
	}
	redisQueue := scheduler.NewRedisQueue(redisClient, cfg.RedisNamespace, "gateway")
	return scheduler.NewFallbackQueue(redisQueue, memoryQueue), "redis"
}

func New() (*App, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger := observability.SetupLogger(cfg.LogLevel)

	observability.InitPrometheusMetrics()

	shutdownTracing, err := observability.SetupTracing(context.Background())
	if err != nil {
		logger.Warn("failed to initialize tracing", "error", err)
		shutdownTracing = func(context.Context) error { return nil }
	}

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

	var coord coordination.Coordinator
	if cfg.MultiNodeEnabled && cfg.RedisEnabled {
		rdb := newControlRedisClient(cfg)
		coord = coordination.NewRedisCoordinator(rdb, cfg.RedisNamespace, cfg.NodeID)
	} else {
		coord = coordination.NewNoopCoordinator()
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

	semanticCache := newSemanticCacheService(context.Background(), cfg, logger, m, repo)

	var lagReporter handlers.LagReporter
	var consumer *replication.Consumer
	if cfg.MultiNodeEnabled && cfg.RedisEnabled && repo != nil {
		rdb := newControlRedisClient(cfg)

		producer := replication.NewRedisStreamProducer(rdb, replication.ControlStreamName)
		wrappedRepo := replication.NewRepository(repo, coord, producer)

		groupName := "gateway-group-" + cfg.NodeID
		consumer = replication.NewConsumer(rdb, replication.ControlStreamName, groupName, cfg.NodeID, repo, repo.FallbackLog())
		consumer.Start(ctx)
		lagReporter = consumer

		worker := replication.NewRecoveryWorker(repo.FallbackLog(), consumer, producer)
		worker.Start(ctx)

		repo = wrappedRepo
	}

	var adminProvHandler *handlers.AdminProvidersHandler
	var adminCombosHandler *handlers.AdminCombosHandler
	var adminSemanticRulesHandler *handlers.AdminSemanticRulesHandler
	var adminSchedulerHandler *handlers.AdminSchedulerHandler
	rolloutController := scheduler.NewSchedulerRolloutController(cfg.Scheduler)
	if repo != nil {
		adminSvc := controlstate.NewAdminProviderService(repo, cipher, m, hotStateClient)
		adminProvHandler = handlers.NewAdminProvidersHandler(adminSvc)

		adminComboSvc := controlstate.NewAdminComboService(repo, m, cipher, hotStateClient)
		adminCombosHandler = handlers.NewAdminCombosHandler(adminComboSvc)

		adminSemanticRulesSvc := controlstate.NewAdminSemanticRulesService(repo, hotStateClient)
		adminSemanticRulesHandler = handlers.NewAdminSemanticRulesHandler(adminSemanticRulesSvc)

		adminSchedulerSvc := scheduler.NewAdminSchedulerService(repo, rolloutController)
		adminSchedulerHandler = handlers.NewAdminSchedulerHandler(adminSchedulerSvc)
	}

	schedulerFeedbackOn := cfg.Scheduler.FeedbackEnabled && repo != nil
	if cfg.Scheduler.FeedbackEnabled && repo == nil {
		logger.Warn("scheduler feedback disabled; durable control state is unavailable")
	}
	var trainingRecorder *scheduler.TrainingRecorder
	if schedulerFeedbackOn {
		trainingRecorder = &scheduler.TrainingRecorder{Repo: repo.SchedulerTrainingSamples()}
	}
	var qualityRecorder *scheduler.PredictionQualityRecorder
	if repo != nil {
		qualityRecorder = &scheduler.PredictionQualityRecorder{Repo: repo.SchedulerQualityRollups(), Metrics: observability.DefaultMetrics, Controller: rolloutController}
	}
	semanticNeighbors := newSemanticNeighborService(ctx, cfg, logger, m, repo)
	schedulerRunner, schedulerBackend := newSchedulerRunner(ctx, cfg, hotStateClient, logger, trainingRecorder, qualityRecorder, rolloutController, semanticNeighbors)
	gatewaySvc := gateway.NewService(m, admissionCtrl, m.HealthStore(), cfg.FallbackEnabled, cfg.MaxAttempts, repo, semanticCache, pipeline.DefaultRegistry(), m, hotStateClient)
	gatewaySvc.SetSchedulerRunner(schedulerRunner)

	r := router.NewRouter(cfg, gatewaySvc, adminProvHandler, adminCombosHandler, adminSemanticRulesHandler, adminSchedulerHandler, hotStateClient, repo, coord, lagReporter)

	application := &App{
		Config:                       cfg,
		Logger:                       logger,
		Router:                       r,
		RuntimeProviderManager:       m,
		HotState:                     hotStateClient,
		Coordinator:                  coord,
		ShutdownTracing:              shutdownTracing,
		SchedulerRunner:              schedulerRunner,
		SchedulerQueueBackend:        schedulerBackend,
		SchedulerFeedbackOn:          schedulerFeedbackOn,
		SchedulerSemanticNeighborsOn: semanticNeighbors != nil,
	}

	if cfg.ControlStateBackend != "disabled" {
		if err := application.ReloadProviders(ctx, repo, cipher); err != nil {
			return nil, fmt.Errorf("failed initial provider reload: %w", err)
		}
		if err := application.StartConfigChangeSubscriber(ctx, repo, cipher); err != nil {
			return nil, fmt.Errorf("failed to start config subscriber: %w", err)
		}

		if consumer != nil {
			consumer.OnApplied = func(evt replication.ChangeEvent) {
				switch evt.Repository {
				case "providers", "providers_secrets", "combos", "routing":
					if err := application.ReloadProviders(context.Background(), repo, cipher); err != nil {
						application.Logger.Error("failed to reload providers after replication", "error", err)
					}
				case "semantic_rules":
					if err := application.ReloadSemanticRules(context.Background(), repo); err != nil {
						application.Logger.Error("failed to reload semantic rules after replication", "error", err)
					}
				case "api_keys":
					if err := application.HotState.Delete(context.Background(), hotstate.NamespacedKey(application.Config.RedisNamespace, "auth", evt.TargetID)); err != nil {
						application.Logger.Error("failed to invalidate api key cache", "error", err)
					}
				}
			}
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

	rates := make(map[string]float64)
	if repo.Rates() != nil {
		for _, r := range records {
			if !r.Enabled {
				continue
			}
			for _, m := range r.Models {
				if rate, err := repo.Rates().Get(ctx, r.ID, m); err == nil && rate != nil {
					rates[r.ID+":"+m] = float64(rate.InputCreditRate + rate.OutputCreditRate)
				}
			}
		}
	}

	return a.RuntimeProviderManager.ActivateDurable(ctx, records, secrets, rCfg, combos, semRules, rates, nil)
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
				case hotstate.EventProvider, hotstate.EventCombo, hotstate.EventRouting:
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
	a.Coordinator.Start(ctx)
	defer a.Coordinator.Stop(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- http.ListenAndServe(a.Config.GatewayDataAddr, a.Router)
	}()

	select {
	case err := <-errChan:
		a.ShutdownTracing(context.Background())
		return err
	case <-ctx.Done():
		a.ShutdownTracing(context.Background())
		return nil
	}
}
