package config

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func LoadConfig() (*Config, error) {
	cfg := &Config{
		GatewayDataAddr:    getEnv("GATEWAY_DATA_ADDR", ""),
		GatewayAdminAddr:   getEnv("GATEWAY_ADMIN_ADDR", ""),
		GatewayMetricsAddr: getEnv("GATEWAY_METRICS_ADDR", ""),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		DevAPIKey:          getEnv("DEV_API_KEY", ""),
		MultiNodeEnabled:   getEnv("MULTI_NODE_ENABLED", "false") == "true",
		NodeID:             getEnv("NODE_ID", ""),
		RoutingStrategy:    getEnv("ROUTING_STRATEGY", "least-latency"),

		ControlStateBackend:          getEnv("CONTROL_STATE_BACKEND", "disabled"),
		ControlStateDSN:              getEnv("CONTROL_STATE_DSN", ""),
		ControlStateMigrateOnStartup: getEnv("CONTROL_STATE_MIGRATE_ON_STARTUP", "false") == "true",
		ControlStateLocalSeedEnabled: getEnv("CONTROL_STATE_LOCAL_SEED_ENABLED", "false") == "true",
		ControlStateEncryptionKey:    getEnv("CONTROL_STATE_ENCRYPTION_KEY", ""),
		AdminAPIKey:                  getEnv("ADMIN_API_KEY", ""),
		AuditRetention:               getEnv("AUDIT_RETENTION", "720h"),

		RedisEnabled:        getEnv("REDIS_ENABLED", "false") == "true",
		RedisAddr:           getEnv("REDIS_ADDR", ""),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisDB:             0, // Simplification, can override via JSON if needed, or parse env
		RedisNamespace:      getEnv("REDIS_NAMESPACE", ""),
		RedisHealthTTL:      getEnv("REDIS_HEALTH_TTL", "1m"),
		RedisAuthCacheTTL:   getEnv("REDIS_AUTH_CACHE_TTL", "5m"),
		RedisDegradeToLocal: getEnv("REDIS_DEGRADE_TO_LOCAL", "true") == "true",

		SemanticCacheEnabled:         getEnv("SEMANTIC_CACHE_ENABLED", "false") == "true",
		SemanticCacheProvider:        getEnv("SEMANTIC_CACHE_PROVIDER", ""),
		SemanticCacheVectorStore:     getEnv("SEMANTIC_CACHE_VECTOR_STORE", ""),
		SemanticCacheVectorDimension: getEnvInt("SEMANTIC_CACHE_VECTOR_DIMENSION", defaultSemanticCacheVectorDimension),
		PGVectorIndexType:            getEnv("PGVECTOR_INDEX_TYPE", defaultPGVectorIndexType),
		PGVectorHNSWM:                getEnvInt("PGVECTOR_HNSW_M", defaultPGVectorHNSWM),
		PGVectorHNSWEFConstruction:   getEnvInt("PGVECTOR_HNSW_EF_CONSTRUCTION", defaultPGVectorHNSWEFConstruction),
		PGVectorSearchEF:             getEnvInt("PGVECTOR_SEARCH_EF", defaultPGVectorSearchEF),
		QdrantAddr:                   getEnv("QDRANT_ADDR", ""),
		QdrantAPIKey:                 getEnv("QDRANT_API_KEY", ""),

		SemanticPipelineConfigFile: getEnv("SEMANTIC_PIPELINE_CONFIG_FILE", ""),
		SchedulerConfigFile:        getEnv("SCHEDULER_CONFIG_FILE", ""),
		CacheConfigFile:            getEnv("CACHE_CONFIG_FILE", ""),
		Scheduler: SchedulerConfig{
			Enabled:                         getEnv("SCHEDULER_ENABLED", "false") == "true",
			Endpoint:                        getEnv("SCHEDULER_ENDPOINT", ""),
			HeuristicEndpoint:               getEnv("SCHEDULER_HEURISTIC_ENDPOINT", ""),
			ONNXEndpoint:                    getEnv("SCHEDULER_ONNX_ENDPOINT", ""),
			ONNXRolloutPercent:              getEnvInt("SCHEDULER_ONNX_ROLLOUT_PERCENT", 0),
			QualityMAPEAlertPercent:         getEnvFloat("SCHEDULER_QUALITY_MAPE_ALERT_PERCENT", 25),
			ErrorSpikeAlertRate:             getEnvFloat("SCHEDULER_ERROR_SPIKE_ALERT_RATE", 0.05),
			Timeout:                         getEnv("SCHEDULER_TIMEOUT", "15ms"),
			Strict:                          getEnv("SCHEDULER_STRICT", "false") == "true",
			BreakerFailureThreshold:         getEnvInt("SCHEDULER_BREAKER_FAILURE_THRESHOLD", 3),
			BreakerRecoveryTimeout:          getEnv("SCHEDULER_BREAKER_RECOVERY_TIMEOUT", "1m"),
			ScorerMaxConcurrency:            getEnvInt("SCHEDULER_SCORER_MAX_CONCURRENCY", 4),
			ScorerSlowThreshold:             getEnv("SCHEDULER_SCORER_SLOW_THRESHOLD", ""),
			QueueBackend:                    getEnv("SCHEDULER_QUEUE_BACKEND", "auto"),
			QueueSoftLimit:                  getEnvInt("SCHEDULER_QUEUE_SOFT_LIMIT", 0),
			QueueHardLimit:                  getEnvInt("SCHEDULER_QUEUE_HARD_LIMIT", 0),
			QueuePopTimeout:                 getEnv("SCHEDULER_QUEUE_POP_TIMEOUT", "100ms"),
			ExecutorConcurrency:             getEnvInt("SCHEDULER_EXECUTOR_CONCURRENCY", 1),
			DefaultPriority:                 getEnv("SCHEDULER_DEFAULT_PRIORITY", "normal"),
			MaxPriority:                     getEnv("SCHEDULER_MAX_PRIORITY", "high"),
			HighQuotaPerMinute:              getEnvInt("SCHEDULER_HIGH_QUOTA_PER_MINUTE", 0),
			ScoreUncertaintyPenaltyK:        getEnvFloat("SCHEDULER_SCORE_UNCERTAINTY_PENALTY_K", 0.2),
			HeuristicConfigFile:             getEnv("SCHEDULER_HEURISTIC_CONFIG_FILE", ""),
			FeedbackEnabled:                 getEnv("SCHEDULER_FEEDBACK_ENABLED", "false") == "true",
			Mode:                            getEnv("SCHEDULER_MODE", "heuristic"),
			ONNXArtifactDir:                 getEnv("SCHEDULER_ONNX_ARTIFACT_DIR", ""),
			SemanticNeighborsEnabled:        getEnv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "false") == "true",
			SemanticNeighborsEmbeddingModel: getEnv("SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL", defaultSemanticNeighborEmbeddingModel),
			SemanticNeighborsMinCount:       getEnvInt("SCHEDULER_SEMANTIC_NEIGHBORS_MIN_COUNT", 20),
			SemanticNeighborsInputMaxChars:  getEnvInt("SCHEDULER_SEMANTIC_NEIGHBORS_INPUT_MAX_CHARS", defaultSemanticNeighborInputMaxChars),
			SemanticNeighborsTaskTimeout:    getEnv("SCHEDULER_SEMANTIC_NEIGHBORS_TASK_TIMEOUT", "5ms"),
			SemanticNeighborsBatchTimeout:   getEnv("SCHEDULER_SEMANTIC_NEIGHBORS_BATCH_TIMEOUT", "15ms"),
			SLAPromotionEnabled:             getEnv("SCHEDULER_SLA_PROMOTION_ENABLED", "false") == "true",
			SLAPromotionCandidateWindow:     getEnvInt("SCHEDULER_SLA_PROMOTION_CANDIDATE_WINDOW", defaultSLAPromotionCandidateWindow),
		},
	}
	cfg.ControlState = controlStateConfigFromEnv()
	cfg.Redis = redisConfigFromEnv()
	cfg.Cache = cacheConfigFromEnv()
	syncLegacyConfigFields(cfg)

	configFile := getEnv("CONFIG_FILE", "")
	if configFile != "" {
		fileCfg, err := readFileConfig(configFile)
		if err != nil {
			return nil, err
		}
		applyFileConfig(cfg, fileCfg)
		if fileCfg.FallbackEnabled == nil {
			cfg.FallbackEnabled = len(cfg.Providers) > 1
		}
	} else {
		// Fallback to backward-compatible env config
		providerID := getEnv("DEFAULT_PROVIDER", "")
		cfg.DefaultProvider = providerID

		modelsRaw := getEnv("OPENAI_PRIMARY_MODELS", "")
		var models []string
		for _, m := range strings.Split(modelsRaw, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}

		cfg.Providers = []ProviderConfig{
			{
				ID:           providerID,
				Type:         "openai-compatible",
				BaseURL:      getEnv("OPENAI_PRIMARY_BASE_URL", ""),
				APIKey:       getEnv("OPENAI_PRIMARY_API_KEY", ""),
				Models:       models,
				DefaultModel: getEnv("OPENAI_PRIMARY_DEFAULT_MODEL", ""),
				Timeout:      "30s",
			},
		}
	}
	if err := applyComponentConfigFiles(cfg); err != nil {
		return nil, err
	}

	applyDefaults(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.MultiNodeEnabled && cfg.NodeID == "" {
		cfg.NodeID = uuid.NewString()
	}

	if cfg.HealthCheck.Enabled == nil {
		enabled := len(cfg.Providers) > 1
		cfg.HealthCheck.Enabled = &enabled
	}
	if cfg.HealthCheck.Interval == "" {
		cfg.HealthCheck.Interval = "30s"
	}
	if cfg.HealthCheck.Timeout == "" {
		cfg.HealthCheck.Timeout = "2s"
	}
	if cfg.HealthCheck.InitialDelay == "" {
		cfg.HealthCheck.InitialDelay = "0s"
	}
	if cfg.HealthCheck.FailureThreshold == 0 {
		cfg.HealthCheck.FailureThreshold = 3
	}
	if cfg.HealthCheck.SuccessThreshold == 0 {
		cfg.HealthCheck.SuccessThreshold = 1
	}
	if cfg.HealthCheck.StaleAfter == "" {
		cfg.HealthCheck.StaleAfter = "0s"
	}
	if cfg.HealthCheck.MaxConcurrency == 0 {
		cfg.HealthCheck.MaxConcurrency = 4
	}

	// Default fallback config
	if cfg.MaxAttempts == 0 {
		if cfg.FallbackEnabled {
			cfg.MaxAttempts = 2
			if len(cfg.Providers) < 2 {
				cfg.MaxAttempts = 1
			}
		} else {
			cfg.MaxAttempts = 1
		}
	}

	normalizeConfigBlocks(cfg)
	applySemanticDefaults(&cfg.Cache)
	applySchedulerDefaults(&cfg.Scheduler)
	syncLegacyConfigFields(cfg)
}

func applySemanticDefaults(cache *CacheConfig) {
	if cache.VectorDimension == 0 {
		cache.VectorDimension = defaultSemanticCacheVectorDimension
	}
	if cache.PGVector.IndexType == "" {
		cache.PGVector.IndexType = defaultPGVectorIndexType
	}
	if cache.PGVector.HNSWM == 0 {
		cache.PGVector.HNSWM = defaultPGVectorHNSWM
	}
	if cache.PGVector.HNSWEFConstruct == 0 {
		cache.PGVector.HNSWEFConstruct = defaultPGVectorHNSWEFConstruction
	}
	if cache.PGVector.SearchEF == 0 {
		cache.PGVector.SearchEF = defaultPGVectorSearchEF
	}
}

func mergeSchedulerConfig(dst *SchedulerConfig, src SchedulerConfig) {
	if src.Enabled {
		dst.Enabled = true
	}
	if src.Endpoint != "" {
		dst.Endpoint = src.Endpoint
	}
	if src.HeuristicEndpoint != "" {
		dst.HeuristicEndpoint = src.HeuristicEndpoint
	}
	if src.ONNXEndpoint != "" {
		dst.ONNXEndpoint = src.ONNXEndpoint
	}
	if src.ONNXRolloutPercent != 0 {
		dst.ONNXRolloutPercent = src.ONNXRolloutPercent
	}
	if src.QualityMAPEAlertPercent != 0 {
		dst.QualityMAPEAlertPercent = src.QualityMAPEAlertPercent
	}
	if src.ErrorSpikeAlertRate != 0 {
		dst.ErrorSpikeAlertRate = src.ErrorSpikeAlertRate
	}
	if src.Timeout != "" {
		dst.Timeout = src.Timeout
	}
	if src.Strict {
		dst.Strict = true
	}
	if src.BreakerFailureThreshold != 0 {
		dst.BreakerFailureThreshold = src.BreakerFailureThreshold
	}
	if src.BreakerRecoveryTimeout != "" {
		dst.BreakerRecoveryTimeout = src.BreakerRecoveryTimeout
	}
	if src.ScorerMaxConcurrency != 0 {
		dst.ScorerMaxConcurrency = src.ScorerMaxConcurrency
	}
	if src.ScorerSlowThreshold != "" {
		dst.ScorerSlowThreshold = src.ScorerSlowThreshold
	}
	if src.QueueBackend != "" {
		dst.QueueBackend = src.QueueBackend
	}
	if src.QueueSoftLimit != 0 {
		dst.QueueSoftLimit = src.QueueSoftLimit
	}
	if src.QueueHardLimit != 0 {
		dst.QueueHardLimit = src.QueueHardLimit
	}
	if src.QueuePopTimeout != "" {
		dst.QueuePopTimeout = src.QueuePopTimeout
	}
	if src.ExecutorConcurrency != 0 {
		dst.ExecutorConcurrency = src.ExecutorConcurrency
	}
	if src.DefaultPriority != "" {
		dst.DefaultPriority = src.DefaultPriority
	}
	if src.MaxPriority != "" {
		dst.MaxPriority = src.MaxPriority
	}
	if src.HighQuotaPerMinute != 0 {
		dst.HighQuotaPerMinute = src.HighQuotaPerMinute
	}
	if src.ScoreUncertaintyPenaltyK != 0 {
		dst.ScoreUncertaintyPenaltyK = src.ScoreUncertaintyPenaltyK
	}
	if src.HeuristicConfigFile != "" {
		dst.HeuristicConfigFile = src.HeuristicConfigFile
	}
	if src.FeedbackEnabled {
		dst.FeedbackEnabled = true
	}
	if src.Mode != "" {
		dst.Mode = src.Mode
	}
	if src.ONNXArtifactDir != "" {
		dst.ONNXArtifactDir = src.ONNXArtifactDir
	}
	if src.SemanticNeighborsEnabled {
		dst.SemanticNeighborsEnabled = true
	}
	if src.SemanticNeighborsEmbeddingModel != "" {
		dst.SemanticNeighborsEmbeddingModel = src.SemanticNeighborsEmbeddingModel
	}
	if src.SemanticNeighborsMinCount != 0 {
		dst.SemanticNeighborsMinCount = src.SemanticNeighborsMinCount
	}
	if src.SemanticNeighborsInputMaxChars != 0 {
		dst.SemanticNeighborsInputMaxChars = src.SemanticNeighborsInputMaxChars
	}
	if src.SemanticNeighborsTaskTimeout != "" {
		dst.SemanticNeighborsTaskTimeout = src.SemanticNeighborsTaskTimeout
	}
	if src.SemanticNeighborsBatchTimeout != "" {
		dst.SemanticNeighborsBatchTimeout = src.SemanticNeighborsBatchTimeout
	}
	if src.SLAPromotionEnabled {
		dst.SLAPromotionEnabled = true
	}
	if src.SLAPromotionCandidateWindow != 0 {
		dst.SLAPromotionCandidateWindow = src.SLAPromotionCandidateWindow
	}
	if len(src.SLAPromotionRules) != 0 {
		dst.SLAPromotionRules = src.SLAPromotionRules
	}
}

func applySchedulerDefaults(s *SchedulerConfig) {
	if s.HeuristicEndpoint == "" {
		s.HeuristicEndpoint = s.Endpoint
	}
	if s.Endpoint == "" {
		s.Endpoint = s.HeuristicEndpoint
	}
	if s.QualityMAPEAlertPercent == 0 {
		s.QualityMAPEAlertPercent = 25
	}
	if s.ErrorSpikeAlertRate == 0 {
		s.ErrorSpikeAlertRate = 0.05
	}
	if s.Timeout == "" {
		s.Timeout = "15ms"
	}
	if s.BreakerFailureThreshold == 0 {
		s.BreakerFailureThreshold = 3
	}
	if s.BreakerRecoveryTimeout == "" {
		s.BreakerRecoveryTimeout = "1m"
	}
	if s.ScorerMaxConcurrency == 0 {
		s.ScorerMaxConcurrency = 4
	}
	if s.ScorerSlowThreshold == "" {
		s.ScorerSlowThreshold = s.Timeout
	}
	if s.QueueBackend == "" {
		s.QueueBackend = "auto"
	}
	if s.QueuePopTimeout == "" {
		s.QueuePopTimeout = "100ms"
	}
	if s.ExecutorConcurrency == 0 {
		s.ExecutorConcurrency = 1
	}
	if s.DefaultPriority == "" {
		s.DefaultPriority = "normal"
	}
	if s.MaxPriority == "" {
		s.MaxPriority = "high"
	}
	if s.ScoreUncertaintyPenaltyK == 0 {
		s.ScoreUncertaintyPenaltyK = 0.2
	}
	if s.Mode == "" {
		s.Mode = "heuristic"
	}
	if s.SemanticNeighborsMinCount == 0 {
		s.SemanticNeighborsMinCount = 20
	}
	if s.SemanticNeighborsEmbeddingModel == "" {
		s.SemanticNeighborsEmbeddingModel = defaultSemanticNeighborEmbeddingModel
	}
	if s.SemanticNeighborsInputMaxChars == 0 {
		s.SemanticNeighborsInputMaxChars = defaultSemanticNeighborInputMaxChars
	}
	if s.SemanticNeighborsTaskTimeout == "" {
		s.SemanticNeighborsTaskTimeout = "5ms"
	}
	if s.SemanticNeighborsBatchTimeout == "" {
		s.SemanticNeighborsBatchTimeout = "15ms"
	}
	if s.SLAPromotionCandidateWindow == 0 {
		s.SLAPromotionCandidateWindow = defaultSLAPromotionCandidateWindow
	}
}

func (c *Config) Validate() error {
	normalizeConfigBlocks(c)
	applySemanticDefaults(&c.Cache)
	applySchedulerDefaults(&c.Scheduler)
	syncLegacyConfigFields(c)

	if c.RoutingStrategy != "round-robin" && c.RoutingStrategy != "least-latency" {
		return fmt.Errorf("invalid routing strategy")
	}

	if err := validateFallback(c); err != nil {
		return err
	}

	if c.ControlState.Backend != "disabled" && c.ControlState.Backend != "sqlite" && c.ControlState.Backend != "postgres" {
		return fmt.Errorf("invalid control state backend: %s. Must be 'sqlite', 'postgres', or 'disabled'", c.ControlState.Backend)
	}

	if c.ControlState.Backend == "sqlite" {
		if c.ControlState.DSN == "" {
			return fmt.Errorf("sqlite control state backend requires a DSN (e.g. file:veloxmesh.db?cache=shared). This is the default Plan 1 deployment")
		}
	}
	if c.ControlState.Backend == "postgres" && c.ControlState.DSN == "" {
		return fmt.Errorf("postgres control state backend requires a DSN")
	}
	if c.ControlState.Backend == "sqlite" || c.ControlState.Backend == "postgres" {
		if c.ControlState.EncryptionKey != "" && len(c.ControlState.EncryptionKey) != 32 {
			return fmt.Errorf("control state encryption key must be exactly 32 bytes (required when durable backend is used)")
		}
		if c.ControlState.EncryptionKey == "" {
			return fmt.Errorf("control state encryption key is required when a durable backend (%s) is used", c.ControlState.Backend)
		}
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	seen := make(map[string]bool)
	defaultFound := false

	if err := validateSemanticCacheConfig(c); err != nil {
		return err
	}
	if err := validateSchedulerConfig(c.Scheduler); err != nil {
		return err
	}

	for i := range c.Providers {
		p := &c.Providers[i]
		if err := validateProvider(p); err != nil {
			return err
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate provider id: %s", p.ID)
		}
		seen[p.ID] = true

		if p.ID == c.DefaultProvider {
			defaultFound = true
		}
	}

	if !defaultFound && c.DefaultProvider != "" {
		return fmt.Errorf("default provider not found")
	}

	if err := validateHealthCheckConfig(&c.HealthCheck); err != nil {
		return err
	}

	for i := range c.Providers {
		p := &c.Providers[i]
		if p.HealthCheck != nil {
			if err := validateProviderHealthCheck(p); err != nil {
				return err
			}
		}
	}

	return nil
}
