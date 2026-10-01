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
	"veloxmesh/internal/redisconn"
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
	lifecycleCtx                 context.Context
	lifecycleCancel              context.CancelFunc
	semanticCache                *cache.SemanticCacheService
}

const (
	controlRedisDialTimeout     = 100 * time.Millisecond
	controlRedisReadTimeout     = 1500 * time.Millisecond
	controlRedisWriteTimeout    = 500 * time.Millisecond
	controlRedisMaxRetries      = 1
	controlRedisMinRetryBackoff = 50 * time.Millisecond
	controlRedisMaxRetryBackoff = 100 * time.Millisecond
	localSchedulerQueueNode     = "local"
)

func newControlRedisClient(cfg *config.Config, logger *slog.Logger) (*redis.Client, error) {
	opts, err := redisconn.Options(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return nil, err
	}
	redisconn.WarnPlaintextCredentials(logger, "control", opts)
	opts.DialTimeout = controlRedisDialTimeout
	opts.ReadTimeout = controlRedisReadTimeout
	opts.WriteTimeout = controlRedisWriteTimeout
	opts.MaxRetries = controlRedisMaxRetries
	opts.MinRetryBackoff = controlRedisMinRetryBackoff
	opts.MaxRetryBackoff = controlRedisMaxRetryBackoff
	opts.ContextTimeoutEnabled = true
	return redis.NewClient(opts), nil
}

func (a *App) HealthStore() health.Store {
	return a.RuntimeProviderManager.HealthStore()
}

type schedulerRunnerDeps struct {
	ctx               context.Context
	cfg               *config.Config
	hotState          hotstate.Client
	logger            *slog.Logger
	repo              controlstate.Repository
	recorder          *scheduler.TrainingRecorder
	quality           *scheduler.PredictionQualityRecorder
	rollout           *scheduler.SchedulerRolloutController
	semanticNeighbors scheduler.SemanticNeighborEnricher
}

type slaPromoterDeps struct {
	cfg      config.SchedulerConfig
	queue    scheduler.QueueBackend
	registry *scheduler.ResultRegistry
	audit    controlstate.AuditRepository
	logger   *slog.Logger
	metrics  observability.Metrics
}

func newSchedulerRunner(deps schedulerRunnerDeps) (*scheduler.SynchronousRunner, string) {
	ctx, cfg, logger := deps.ctx, deps.cfg, deps.logger
	queue, backend := newSchedulerQueue(ctx, cfg, logger)
	scorer, err := scheduler.NewScorerWithController(ctx, cfg.Scheduler, deps.rollout)
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
		Priority: scheduler.NewPriorityResolver(deps.hotState),
		Policy: scheduler.PriorityPolicy{
			Default:            scheduler.NormalizePriority(cfg.Scheduler.DefaultPriority),
			Max:                scheduler.NormalizePriority(cfg.Scheduler.MaxPriority),
			HighQuotaPerMinute: int64(cfg.Scheduler.HighQuotaPerMinute),
			Strict:             cfg.Scheduler.Strict,
		},
		Metrics:           observability.DefaultMetrics,
		Backend:           backend,
		SemanticNeighbors: deps.semanticNeighbors,
	}
	if timeout, err := time.ParseDuration(cfg.Scheduler.SemanticNeighborsTaskTimeout); err == nil {
		intake.SemanticNeighborTaskTimeout = timeout
	}
	if timeout, err := time.ParseDuration(cfg.Scheduler.QueuePopTimeout); err == nil {
		intake.ThrottleWait = timeout
	}
	executor := &scheduler.Executor{
		Queue:    queue,
		Registry: registry,
		Metrics:  observability.DefaultMetrics,
		Promoter: newSLAPromoter(slaPromoterDeps{
			cfg:      cfg.Scheduler,
			queue:    queue,
			registry: registry,
			audit:    schedulerAudit(deps.repo),
			logger:   logger,
			metrics:  observability.DefaultMetrics,
		}),
	}
	runner := scheduler.NewSynchronousRunnerWithConcurrency(intake, executor, registry, cfg.Scheduler.ExecutorConcurrency)
	runner.Recorder = deps.recorder
	runner.Quality = deps.quality
	if indexer, ok := deps.semanticNeighbors.(scheduler.SemanticNeighborIndexer); ok {
		runner.Indexer = indexer
	}
	return runner, backend
}

func newOptionalSchedulerRunner(deps schedulerRunnerDeps) (*scheduler.SynchronousRunner, string) {
	if !deps.cfg.Scheduler.Enabled {
		return nil, "disabled"
	}
	return newSchedulerRunner(deps)
}

func newSLAPromoter(deps slaPromoterDeps) *scheduler.SLAPromoter {
	if !deps.cfg.SLAPromotionEnabled {
		return nil
	}
	return &scheduler.SLAPromoter{
		Enabled:         true,
		CandidateWindow: deps.cfg.SLAPromotionCandidateWindow,
		Rules:           deps.cfg.SLAPromotionRules,
		Queue:           deps.queue,
		Registry:        deps.registry,
		Audit:           deps.audit,
		Logger:          deps.logger,
		Metrics:         deps.metrics,
	}
}

func schedulerAudit(repo controlstate.Repository) controlstate.AuditRepository {
	if repo == nil {
		return nil
	}
	return repo.Audit()
}

func newSchedulerQueue(ctx context.Context, cfg *config.Config, logger *slog.Logger) (scheduler.QueueBackend, string) {
	memoryQueue := scheduler.NewMemoryQueue()
	backend := strings.ToLower(cfg.Scheduler.QueueBackend)
	if backend != "redis" || !cfg.RedisEnabled {
		return memoryQueue, "memory"
	}
	redisClient, err := newControlRedisClient(cfg, logger)
	if err != nil {
		logger.Warn("scheduler redis queue unavailable; using memory queue", "error", err)
		return memoryQueue, "memory"
	}
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Warn("scheduler redis queue unavailable; using memory queue", "error", err)
		_ = redisClient.Close()
		return memoryQueue, "memory"
	}
	redisQueue := scheduler.NewRedisQueue(redisClient, cfg.RedisNamespace, schedulerRedisQueueName(cfg))
	return scheduler.NewFallbackQueue(redisQueue, memoryQueue), "redis+fallback"
}

func schedulerRedisQueueName(cfg *config.Config) string {
	nodeID := strings.TrimSpace(cfg.NodeID)
	if nodeID == "" {
		nodeID = localSchedulerQueueNode
	}
	return "gateway-" + nodeID
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
		rdb, err := newControlRedisClient(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize redis coordination: %w", err)
		}
		coord = coordination.NewRedisCoordinator(rdb, cfg.RedisNamespace, cfg.NodeID)
	} else {
		coord = coordination.NewNoopCoordinator()
	}

	m := controlstate.NewRuntimeProviderManager(cfg, logger, healthStore)

	var repo controlstate.Repository
	var cipher controlstate.SecretCipher
	ctx, cancel := context.WithCancel(context.Background())
	initialized := false
	defer func() {
		if !initialized {
			cancel()
		}
	}()

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
				adapters = append(adapters, anthropic.NewAdapter(anthropic.AdapterConfig{ID: p.ID, BaseURL: p.BaseURL, APIKey: p.ResolveAPIKey(), ModelsCSV: strings.Join(p.Models, ",")}))
			case "gemini":
				adapters = append(adapters, gemini.NewAdapter(gemini.AdapterConfig{ID: p.ID, BaseURL: p.BaseURL, APIKey: p.ResolveAPIKey(), ModelsCSV: strings.Join(p.Models, ",")}))
			}
		}
		if err := m.ActivateStatic(cfg.Providers, adapters); err != nil {
			return nil, fmt.Errorf("failed to initialize static providers: %w", err)
		}
	} else {
		bootstrap := &App{Logger: logger, RuntimeProviderManager: m}
		if err := bootstrap.ReloadProviders(ctx, repo, cipher); err != nil {
			return nil, fmt.Errorf("failed initial provider reload: %w", err)
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
		rdb, err := newControlRedisClient(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize redis replication: %w", err)
		}

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
	schedulerRunner, schedulerBackend := newOptionalSchedulerRunner(schedulerRunnerDeps{
		ctx:               ctx,
		cfg:               cfg,
		hotState:          hotStateClient,
		logger:            logger,
		repo:              repo,
		recorder:          trainingRecorder,
		quality:           qualityRecorder,
		rollout:           rolloutController,
		semanticNeighbors: semanticNeighbors,
	})
	if repo != nil {
		adminSchedulerSvc := scheduler.NewAdminSchedulerService(repo, rolloutController, schedulerRunner)
		adminSchedulerHandler = handlers.NewAdminSchedulerHandler(adminSchedulerSvc)
	}
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
		lifecycleCtx:                 ctx,
		lifecycleCancel:              cancel,
		semanticCache:                semanticCache,
	}

	if cfg.ControlStateBackend != "disabled" {
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

	initialized = true
	return application, nil
}

func (a *App) Close() {
	if a == nil || a.lifecycleCancel == nil {
		return
	}
	a.lifecycleCancel()
	if a.semanticCache != nil {
		a.semanticCache.Close()
	}
}

func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("starting gateway", "addr", a.Config.GatewayDataAddr)
	defer a.Close()

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
