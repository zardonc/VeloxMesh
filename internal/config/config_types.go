package config

import "os"

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
	Type         string                     `json:"type"`
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

	RoutingStrategy string
	DefaultProvider string

	FallbackEnabled bool
	MaxAttempts     int

	HealthCheck HealthCheckConfig
	Providers   []ProviderConfig

	ControlState ControlStateConfig `json:"control_state"`
	Redis        RedisConfig        `json:"redis"`
	Cache        CacheConfig        `json:"cache"`

	ControlStateBackend          string `json:"control_state_backend"`
	ControlStateDSN              string `json:"control_state_dsn"`
	ControlStateMigrateOnStartup bool   `json:"control_state_migrate_on_startup"`
	ControlStateLocalSeedEnabled bool   `json:"control_state_local_seed_enabled"`
	ControlStateEncryptionKey    string `json:"control_state_encryption_key"`
	AdminAPIKey                  string `json:"admin_api_key"`
	AuditRetention               string `json:"audit_retention"`

	RedisEnabled        bool   `json:"redis_enabled"`
	RedisAddr           string `json:"redis_addr"`
	RedisPassword       string `json:"redis_password"`
	RedisDB             int    `json:"redis_db"`
	RedisNamespace      string `json:"redis_namespace"`
	RedisHealthTTL      string `json:"redis_health_ttl"`
	RedisAuthCacheTTL   string `json:"redis_auth_cache_ttl"`
	RedisDegradeToLocal bool   `json:"redis_degrade_to_local"`

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

	SemanticPipelineConfigFile string `json:"semantic_pipeline_config_file"`

	Scheduler           SchedulerConfig `json:"scheduler"`
	SchedulerConfigFile string          `json:"scheduler_config_file"`
	CacheConfigFile     string          `json:"cache_config_file"`
}

type ControlStateConfig struct {
	Backend          string `json:"backend"`
	DSN              string `json:"dsn"`
	MigrateOnStartup bool   `json:"migrate_on_startup"`
	LocalSeedEnabled bool   `json:"local_seed_enabled"`
	EncryptionKey    string `json:"encryption_key"`
	AdminAPIKey      string `json:"admin_api_key"`
	AuditRetention   string `json:"audit_retention"`
}

type RedisConfig struct {
	Enabled        bool   `json:"enabled"`
	Addr           string `json:"addr"`
	Password       string `json:"password"`
	DB             int    `json:"db"`
	Namespace      string `json:"namespace"`
	HealthTTL      string `json:"health_ttl"`
	AuthCacheTTL   string `json:"auth_cache_ttl"`
	DegradeToLocal bool   `json:"degrade_to_local"`
}

type CacheConfig struct {
	Enabled         bool           `json:"enabled"`
	Provider        string         `json:"provider"`
	VectorStore     string         `json:"vector_store"`
	VectorDimension int            `json:"vector_dimension"`
	PGVector        PGVectorConfig `json:"pgvector"`
	Qdrant          QdrantConfig   `json:"qdrant"`
}

type PGVectorConfig struct {
	IndexType       string `json:"index_type"`
	HNSWM           int    `json:"hnsw_m"`
	HNSWEFConstruct int    `json:"hnsw_ef_construction"`
	SearchEF        int    `json:"search_ef"`
}

type QdrantConfig struct {
	Addr   string `json:"addr"`
	APIKey string `json:"api_key"`
}

type SchedulerConfig struct {
	Enabled                       bool               `json:"enabled"`
	Endpoint                      string             `json:"endpoint"`
	HeuristicEndpoint             string             `json:"heuristic_endpoint"`
	ONNXEndpoint                  string             `json:"onnx_endpoint"`
	ONNXRolloutPercent            int                `json:"onnx_rollout_percent"`
	QualityMAPEAlertPercent       float64            `json:"quality_mape_alert_percent"`
	ErrorSpikeAlertRate           float64            `json:"error_spike_alert_rate"`
	Timeout                       string             `json:"timeout"`
	Strict                        bool               `json:"strict"`
	BreakerFailureThreshold       int                `json:"breaker_failure_threshold"`
	BreakerRecoveryTimeout        string             `json:"breaker_recovery_timeout"`
	QueueBackend                  string             `json:"queue_backend"`
	QueueSoftLimit                int                `json:"queue_soft_limit"`
	QueueHardLimit                int                `json:"queue_hard_limit"`
	QueuePopTimeout               string             `json:"queue_pop_timeout"`
	ExecutorConcurrency           int                `json:"executor_concurrency"`
	DefaultPriority               string             `json:"default_priority"`
	MaxPriority                   string             `json:"max_priority"`
	HighQuotaPerMinute            int                `json:"high_quota_per_minute"`
	ScoreUncertaintyPenaltyK      float64            `json:"score_uncertainty_penalty_k"`
	HeuristicConfigFile           string             `json:"heuristic_config_file"`
	FeedbackEnabled               bool               `json:"feedback_enabled"`
	Mode                          string             `json:"mode"`
	ONNXArtifactDir               string             `json:"onnx_artifact_dir"`
	SemanticNeighborsEnabled      bool               `json:"semantic_neighbors_enabled"`
	SemanticNeighborsMinCount     int                `json:"semantic_neighbors_min_count"`
	SemanticNeighborsTaskTimeout  string             `json:"semantic_neighbors_task_timeout"`
	SemanticNeighborsBatchTimeout string             `json:"semantic_neighbors_batch_timeout"`
	SLAPromotionEnabled           bool               `json:"sla_promotion_enabled"`
	SLAPromotionCandidateWindow   int                `json:"sla_promotion_candidate_window"`
	SLAPromotionRules             []SLAPromotionRule `json:"sla_promotion_rules"`
}

type SLAPromotionRule struct {
	PolicyID      string `json:"policy_id"`
	TenantID      string `json:"tenant_id"`
	TenantClass   string `json:"tenant_class"`
	ModelClass    string `json:"model_class"`
	RequestKind   string `json:"request_kind"`
	WaitThreshold string `json:"wait_threshold"`
}
