package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

type ProviderAuthConfig struct {
	APIKeyEnv string `json:"api_key_env"`
}

type ProviderHealthCheckConfig struct {
	Enabled          *bool  `json:"enabled"`
	Interval         string `json:"interval"`
	Timeout          string `json:"timeout"`
	InitialDelay     string `json:"initial_delay"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
}

type HealthCheckConfig struct {
	Enabled          *bool  `json:"enabled"`
	Interval         string `json:"interval"`
	Timeout          string `json:"timeout"`
	InitialDelay     string `json:"initial_delay"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
	StaleAfter       string `json:"stale_after"`
	MaxConcurrency   int    `json:"max_concurrency"`
}

type ProviderConfig struct {
	ID           string                     `json:"id"`
	Type         string                     `json:"type"` // e.g. "openai-compatible"
	BaseURL      string                     `json:"base_url"`
	APIKey       string                     `json:"api_key"`
	Auth         *ProviderAuthConfig        `json:"auth"`
	Models       []string                   `json:"models"`
	DefaultModel string                     `json:"default_model"`
	Timeout      string                     `json:"timeout"`
	Weight       int                        `json:"weight"`
	HealthCheck  *ProviderHealthCheckConfig `json:"health_check"`
}

func (p *ProviderConfig) ResolveAPIKey() string {
	if p.Auth != nil && p.Auth.APIKeyEnv != "" {
		if val, exists := os.LookupEnv(p.Auth.APIKeyEnv); exists {
			return val
		}
	}
	return p.APIKey
}

type Config struct {
	GatewayDataAddr    string
	GatewayAdminAddr   string
	GatewayMetricsAddr string
	LogLevel           string
	DevAPIKey          string

	MultiNodeEnabled bool   `json:"multi_node_enabled"`
	NodeID           string `json:"node_id"`

	RoutingStrategy string // e.g. "round-robin", "least-latency"
	DefaultProvider string

	FallbackEnabled bool
	MaxAttempts     int

	HealthCheck HealthCheckConfig

	Providers []ProviderConfig

	// Phase 3 Control State Fields
	ControlStateBackend          string `json:"control_state_backend"`
	ControlStateDSN              string `json:"control_state_dsn"`
	ControlStateMigrateOnStartup bool   `json:"control_state_migrate_on_startup"`
	ControlStateLocalSeedEnabled bool   `json:"control_state_local_seed_enabled"`
	ControlStateEncryptionKey    string `json:"control_state_encryption_key"`
	AdminAPIKey                  string `json:"admin_api_key"`
	AuditRetention               string `json:"audit_retention"`

	// Phase 3 Hot State Fields
	RedisEnabled        bool   `json:"redis_enabled"`
	RedisAddr           string `json:"redis_addr"`
	RedisPassword       string `json:"redis_password"`
	RedisDB             int    `json:"redis_db"`
	RedisNamespace      string `json:"redis_namespace"`
	RedisHealthTTL      string `json:"redis_health_ttl"`
	RedisAuthCacheTTL   string `json:"redis_auth_cache_ttl"`
	RedisDegradeToLocal bool   `json:"redis_degrade_to_local"`

	// Phase 4 Semantic Cache Fields
	SemanticCacheEnabled         bool   `json:"semantic_cache_enabled"`
	SemanticCacheProvider        string `json:"semantic_cache_provider"`
	SemanticCacheVectorStore     string `json:"semantic_cache_vector_store"`
	SemanticCacheVectorDimension int    `json:"semantic_cache_vector_dimension"`
	PGVectorIndexType            string `json:"pgvector_index_type"`
	PGVectorHNSWM                int    `json:"pgvector_hnsw_m"`
	PGVectorHNSWEFConstruction   int    `json:"pgvector_hnsw_ef_construction"`
	PGVectorSearchEF             int    `json:"pgvector_search_ef"`
	QdrantAddr                   string `json:"qdrant_addr"`
	QdrantAPIKey                 string `json:"qdrant_api_key"`

	// Phase 8 Semantic Pipeline
	SemanticPipelineConfigFile string `json:"semantic_pipeline_config_file"`

	// Phase 14 Scheduler
	Scheduler SchedulerConfig `json:"scheduler"`
}

type SchedulerConfig struct {
	Enabled                  bool    `json:"enabled"`
	Endpoint                 string  `json:"endpoint"`
	Timeout                  string  `json:"timeout"`
	Strict                   bool    `json:"strict"`
	BreakerFailureThreshold  int     `json:"breaker_failure_threshold"`
	BreakerRecoveryTimeout   string  `json:"breaker_recovery_timeout"`
	QueueBackend             string  `json:"queue_backend"`
	QueueSoftLimit           int     `json:"queue_soft_limit"`
	QueueHardLimit           int     `json:"queue_hard_limit"`
	QueuePopTimeout          string  `json:"queue_pop_timeout"`
	ExecutorConcurrency      int     `json:"executor_concurrency"`
	DefaultPriority          string  `json:"default_priority"`
	MaxPriority              string  `json:"max_priority"`
	HighQuotaPerMinute       int     `json:"high_quota_per_minute"`
	ScoreUncertaintyPenaltyK float64 `json:"score_uncertainty_penalty_k"`
	HeuristicConfigFile      string  `json:"heuristic_config_file"`
	FeedbackEnabled          bool    `json:"feedback_enabled"`
	Mode                     string  `json:"mode"`
	ONNXArtifactDir          string  `json:"onnx_artifact_dir"`
}

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
		Scheduler: SchedulerConfig{
			Enabled:                  getEnv("SCHEDULER_ENABLED", "false") == "true",
			Endpoint:                 getEnv("SCHEDULER_ENDPOINT", ""),
			Timeout:                  getEnv("SCHEDULER_TIMEOUT", "15ms"),
			Strict:                   getEnv("SCHEDULER_STRICT", "false") == "true",
			BreakerFailureThreshold:  getEnvInt("SCHEDULER_BREAKER_FAILURE_THRESHOLD", 3),
			BreakerRecoveryTimeout:   getEnv("SCHEDULER_BREAKER_RECOVERY_TIMEOUT", "1m"),
			QueueBackend:             getEnv("SCHEDULER_QUEUE_BACKEND", "auto"),
			QueueSoftLimit:           getEnvInt("SCHEDULER_QUEUE_SOFT_LIMIT", 0),
			QueueHardLimit:           getEnvInt("SCHEDULER_QUEUE_HARD_LIMIT", 0),
			QueuePopTimeout:          getEnv("SCHEDULER_QUEUE_POP_TIMEOUT", "100ms"),
			ExecutorConcurrency:      getEnvInt("SCHEDULER_EXECUTOR_CONCURRENCY", 1),
			DefaultPriority:          getEnv("SCHEDULER_DEFAULT_PRIORITY", "normal"),
			MaxPriority:              getEnv("SCHEDULER_MAX_PRIORITY", "high"),
			HighQuotaPerMinute:       getEnvInt("SCHEDULER_HIGH_QUOTA_PER_MINUTE", 0),
			ScoreUncertaintyPenaltyK: getEnvFloat("SCHEDULER_SCORE_UNCERTAINTY_PENALTY_K", 0.2),
			HeuristicConfigFile:      getEnv("SCHEDULER_HEURISTIC_CONFIG_FILE", ""),
			FeedbackEnabled:          getEnv("SCHEDULER_FEEDBACK_ENABLED", "false") == "true",
			Mode:                     getEnv("SCHEDULER_MODE", "heuristic"),
			ONNXArtifactDir:          getEnv("SCHEDULER_ONNX_ARTIFACT_DIR", ""),
		},
	}

	configFile := getEnv("CONFIG_FILE", "")
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %v", err)
		}

		var fileCfg struct {
			MultiNodeEnabled *bool             `json:"multi_node_enabled"`
			NodeID           string            `json:"node_id"`
			RoutingStrategy  string            `json:"routing_strategy"`
			DefaultProvider  string            `json:"default_provider"`
			FallbackEnabled  *bool             `json:"fallback_enabled"`
			MaxAttempts      *int              `json:"max_attempts"`
			HealthCheck      HealthCheckConfig `json:"health_check"`
			Providers        []ProviderConfig  `json:"providers"`

			ControlStateBackend          string `json:"control_state_backend"`
			ControlStateDSN              string `json:"control_state_dsn"`
			ControlStateMigrateOnStartup *bool  `json:"control_state_migrate_on_startup"`
			ControlStateLocalSeedEnabled *bool  `json:"control_state_local_seed_enabled"`
			ControlStateEncryptionKey    string `json:"control_state_encryption_key"`
			AdminAPIKey                  string `json:"admin_api_key"`
			AuditRetention               string `json:"audit_retention"`

			RedisEnabled        *bool  `json:"redis_enabled"`
			RedisAddr           string `json:"redis_addr"`
			RedisPassword       string `json:"redis_password"`
			RedisDB             *int   `json:"redis_db"`
			RedisNamespace      string `json:"redis_namespace"`
			RedisHealthTTL      string `json:"redis_health_ttl"`
			RedisAuthCacheTTL   string `json:"redis_auth_cache_ttl"`
			RedisDegradeToLocal *bool  `json:"redis_degrade_to_local"`

			SemanticCacheEnabled         *bool  `json:"semantic_cache_enabled"`
			SemanticCacheProvider        string `json:"semantic_cache_provider"`
			SemanticCacheVectorStore     string `json:"semantic_cache_vector_store"`
			SemanticCacheVectorDimension *int   `json:"semantic_cache_vector_dimension"`
			PGVectorIndexType            string `json:"pgvector_index_type"`
			PGVectorHNSWM                *int   `json:"pgvector_hnsw_m"`
			PGVectorHNSWEFConstruction   *int   `json:"pgvector_hnsw_ef_construction"`
			PGVectorSearchEF             *int   `json:"pgvector_search_ef"`
			QdrantAddr                   string `json:"qdrant_addr"`
			QdrantAPIKey                 string `json:"qdrant_api_key"`

			SemanticPipelineConfigFile string          `json:"semantic_pipeline_config_file"`
			Scheduler                  SchedulerConfig `json:"scheduler"`
		}
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %v", err)
		}

		fallbackEnabledSet := false
		if fileCfg.FallbackEnabled != nil {
			cfg.FallbackEnabled = *fileCfg.FallbackEnabled
			fallbackEnabledSet = true
		}
		if fileCfg.MaxAttempts != nil {
			cfg.MaxAttempts = *fileCfg.MaxAttempts
		}

		if fileCfg.MultiNodeEnabled != nil {
			cfg.MultiNodeEnabled = *fileCfg.MultiNodeEnabled
		}
		if fileCfg.NodeID != "" {
			cfg.NodeID = fileCfg.NodeID
		}

		if fileCfg.RoutingStrategy != "" {
			cfg.RoutingStrategy = fileCfg.RoutingStrategy
		}
		if fileCfg.DefaultProvider != "" {
			cfg.DefaultProvider = fileCfg.DefaultProvider
		}
		cfg.HealthCheck = fileCfg.HealthCheck
		cfg.Providers = fileCfg.Providers

		if fileCfg.ControlStateBackend != "" {
			cfg.ControlStateBackend = fileCfg.ControlStateBackend
		}
		if fileCfg.ControlStateDSN != "" {
			cfg.ControlStateDSN = fileCfg.ControlStateDSN
		}
		if fileCfg.ControlStateMigrateOnStartup != nil {
			cfg.ControlStateMigrateOnStartup = *fileCfg.ControlStateMigrateOnStartup
		}
		if fileCfg.ControlStateLocalSeedEnabled != nil {
			cfg.ControlStateLocalSeedEnabled = *fileCfg.ControlStateLocalSeedEnabled
		}
		if fileCfg.ControlStateEncryptionKey != "" {
			cfg.ControlStateEncryptionKey = fileCfg.ControlStateEncryptionKey
		}
		if fileCfg.AdminAPIKey != "" {
			cfg.AdminAPIKey = fileCfg.AdminAPIKey
		}
		if fileCfg.AuditRetention != "" {
			cfg.AuditRetention = fileCfg.AuditRetention
		}

		if fileCfg.RedisEnabled != nil {
			cfg.RedisEnabled = *fileCfg.RedisEnabled
		}
		if fileCfg.RedisAddr != "" {
			cfg.RedisAddr = fileCfg.RedisAddr
		}
		if fileCfg.RedisPassword != "" {
			cfg.RedisPassword = fileCfg.RedisPassword
		}
		if fileCfg.RedisDB != nil {
			cfg.RedisDB = *fileCfg.RedisDB
		}
		if fileCfg.RedisNamespace != "" {
			cfg.RedisNamespace = fileCfg.RedisNamespace
		}
		if fileCfg.RedisHealthTTL != "" {
			cfg.RedisHealthTTL = fileCfg.RedisHealthTTL
		}
		if fileCfg.RedisAuthCacheTTL != "" {
			cfg.RedisAuthCacheTTL = fileCfg.RedisAuthCacheTTL
		}
		if fileCfg.RedisDegradeToLocal != nil {
			cfg.RedisDegradeToLocal = *fileCfg.RedisDegradeToLocal
		}

		if fileCfg.SemanticCacheEnabled != nil {
			cfg.SemanticCacheEnabled = *fileCfg.SemanticCacheEnabled
		}
		if fileCfg.SemanticCacheProvider != "" {
			cfg.SemanticCacheProvider = fileCfg.SemanticCacheProvider
		}
		if fileCfg.SemanticCacheVectorStore != "" {
			cfg.SemanticCacheVectorStore = fileCfg.SemanticCacheVectorStore
		}
		if fileCfg.SemanticCacheVectorDimension != nil {
			cfg.SemanticCacheVectorDimension = *fileCfg.SemanticCacheVectorDimension
		}
		if fileCfg.PGVectorIndexType != "" {
			cfg.PGVectorIndexType = fileCfg.PGVectorIndexType
		}
		if fileCfg.PGVectorHNSWM != nil {
			cfg.PGVectorHNSWM = *fileCfg.PGVectorHNSWM
		}
		if fileCfg.PGVectorHNSWEFConstruction != nil {
			cfg.PGVectorHNSWEFConstruction = *fileCfg.PGVectorHNSWEFConstruction
		}
		if fileCfg.PGVectorSearchEF != nil {
			cfg.PGVectorSearchEF = *fileCfg.PGVectorSearchEF
		}
		if fileCfg.QdrantAddr != "" {
			cfg.QdrantAddr = fileCfg.QdrantAddr
		}
		if fileCfg.QdrantAPIKey != "" {
			cfg.QdrantAPIKey = fileCfg.QdrantAPIKey
		}

		if fileCfg.SemanticPipelineConfigFile != "" {
			cfg.SemanticPipelineConfigFile = fileCfg.SemanticPipelineConfigFile
		}
		mergeSchedulerConfig(&cfg.Scheduler, fileCfg.Scheduler)

		if !fallbackEnabledSet {
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

	applySemanticDefaults(cfg)
	applySchedulerDefaults(&cfg.Scheduler)
}

func applySemanticDefaults(cfg *Config) {
	if cfg.SemanticCacheVectorDimension == 0 {
		cfg.SemanticCacheVectorDimension = defaultSemanticCacheVectorDimension
	}
	if cfg.PGVectorIndexType == "" {
		cfg.PGVectorIndexType = defaultPGVectorIndexType
	}
	if cfg.PGVectorHNSWM == 0 {
		cfg.PGVectorHNSWM = defaultPGVectorHNSWM
	}
	if cfg.PGVectorHNSWEFConstruction == 0 {
		cfg.PGVectorHNSWEFConstruction = defaultPGVectorHNSWEFConstruction
	}
	if cfg.PGVectorSearchEF == 0 {
		cfg.PGVectorSearchEF = defaultPGVectorSearchEF
	}
}

func mergeSchedulerConfig(dst *SchedulerConfig, src SchedulerConfig) {
	if src.Enabled {
		dst.Enabled = true
	}
	if src.Endpoint != "" {
		dst.Endpoint = src.Endpoint
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
}

func applySchedulerDefaults(s *SchedulerConfig) {
	if s.Timeout == "" {
		s.Timeout = "15ms"
	}
	if s.BreakerFailureThreshold == 0 {
		s.BreakerFailureThreshold = 3
	}
	if s.BreakerRecoveryTimeout == "" {
		s.BreakerRecoveryTimeout = "1m"
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
}

func (c *Config) Validate() error {
	applySemanticDefaults(c)
	applySchedulerDefaults(&c.Scheduler)

	if c.RoutingStrategy != "round-robin" && c.RoutingStrategy != "least-latency" {
		return fmt.Errorf("invalid routing strategy")
	}

	if err := validateFallback(c); err != nil {
		return err
	}

	if c.ControlStateBackend != "disabled" && c.ControlStateBackend != "sqlite" && c.ControlStateBackend != "postgres" {
		return fmt.Errorf("invalid control state backend: %s. Must be 'sqlite', 'postgres', or 'disabled'", c.ControlStateBackend)
	}

	if c.ControlStateBackend == "sqlite" {
		if c.ControlStateDSN == "" {
			return fmt.Errorf("sqlite control state backend requires a DSN (e.g. file:veloxmesh.db?cache=shared). This is the default Plan 1 deployment")
		}
	}
	if c.ControlStateBackend == "postgres" && c.ControlStateDSN == "" {
		return fmt.Errorf("postgres control state backend requires a DSN")
	}
	if c.ControlStateBackend == "sqlite" || c.ControlStateBackend == "postgres" {
		if c.ControlStateEncryptionKey != "" && len(c.ControlStateEncryptionKey) != 32 {
			return fmt.Errorf("control state encryption key must be exactly 32 bytes (required when durable backend is used)")
		}
		if c.ControlStateEncryptionKey == "" {
			return fmt.Errorf("control state encryption key is required when a durable backend (%s) is used", c.ControlStateBackend)
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
