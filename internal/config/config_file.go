package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type fileConfig struct {
	MultiNodeEnabled *bool             `json:"multi_node_enabled"`
	NodeID           string            `json:"node_id"`
	RoutingStrategy  string            `json:"routing_strategy"`
	DefaultProvider  string            `json:"default_provider"`
	FallbackEnabled  *bool             `json:"fallback_enabled"`
	MaxAttempts      *int              `json:"max_attempts"`
	HealthCheck      HealthCheckConfig `json:"health_check"`
	Providers        []ProviderConfig  `json:"providers"`

	ControlState *controlStateFileConfig `json:"control_state"`
	Redis        *redisFileConfig        `json:"redis"`
	Cache        *cacheFileConfig        `json:"cache"`

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

	SemanticPipelineConfigFile string               `json:"semantic_pipeline_config_file"`
	Scheduler                  *schedulerFileConfig `json:"scheduler"`
	SchedulerConfigFile        string               `json:"scheduler_config_file"`
	CacheConfigFile            string               `json:"cache_config_file"`
}

type controlStateFileConfig struct {
	Backend          string `json:"backend"`
	DSN              string `json:"dsn"`
	MigrateOnStartup *bool  `json:"migrate_on_startup"`
	LocalSeedEnabled *bool  `json:"local_seed_enabled"`
	EncryptionKey    string `json:"encryption_key"`
	AdminAPIKey      string `json:"admin_api_key"`
	AuditRetention   string `json:"audit_retention"`
}

type redisFileConfig struct {
	Enabled        *bool  `json:"enabled"`
	Addr           string `json:"addr"`
	Password       string `json:"password"`
	DB             *int   `json:"db"`
	Namespace      string `json:"namespace"`
	HealthTTL      string `json:"health_ttl"`
	AuthCacheTTL   string `json:"auth_cache_ttl"`
	DegradeToLocal *bool  `json:"degrade_to_local"`
}

type cacheFileConfig struct {
	Enabled         *bool          `json:"enabled"`
	Provider        string         `json:"provider"`
	VectorStore     string         `json:"vector_store"`
	VectorDimension *int           `json:"vector_dimension"`
	PGVector        PGVectorConfig `json:"pgvector"`
	Qdrant          QdrantConfig   `json:"qdrant"`
}

type schedulerFileConfig struct {
	Enabled                         *bool               `json:"enabled"`
	Endpoint                        string              `json:"endpoint"`
	HeuristicEndpoint               string              `json:"heuristic_endpoint"`
	ONNXEndpoint                    string              `json:"onnx_endpoint"`
	ONNXRolloutPercent              *int                `json:"onnx_rollout_percent"`
	QualityMAPEAlertPercent         *float64            `json:"quality_mape_alert_percent"`
	ErrorSpikeAlertRate             *float64            `json:"error_spike_alert_rate"`
	QualitySampleWindow             *int                `json:"quality_sample_window"`
	Timeout                         string              `json:"timeout"`
	Strict                          *bool               `json:"strict"`
	BreakerFailureThreshold         *int                `json:"breaker_failure_threshold"`
	BreakerRecoveryTimeout          string              `json:"breaker_recovery_timeout"`
	ScorerMaxConcurrency            *int                `json:"scorer_max_concurrency"`
	ScorerSlowThreshold             string              `json:"scorer_slow_threshold"`
	QueueBackend                    string              `json:"queue_backend"`
	QueueSoftLimit                  *int                `json:"queue_soft_limit"`
	QueueHardLimit                  *int                `json:"queue_hard_limit"`
	QueuePopTimeout                 string              `json:"queue_pop_timeout"`
	ExecutorConcurrency             *int                `json:"executor_concurrency"`
	DefaultPriority                 string              `json:"default_priority"`
	MaxPriority                     string              `json:"max_priority"`
	HighQuotaPerMinute              *int                `json:"high_quota_per_minute"`
	ScoreUncertaintyPenaltyK        *float64            `json:"score_uncertainty_penalty_k"`
	HeuristicConfigFile             string              `json:"heuristic_config_file"`
	FeedbackEnabled                 *bool               `json:"feedback_enabled"`
	Mode                            string              `json:"mode"`
	ONNXArtifactDir                 string              `json:"onnx_artifact_dir"`
	SemanticNeighborsEnabled        *bool               `json:"semantic_neighbors_enabled"`
	SemanticNeighborsEmbeddingModel string              `json:"semantic_neighbors_embedding_model"`
	SemanticNeighborsMinCount       *int                `json:"semantic_neighbors_min_count"`
	SemanticNeighborsInputMaxChars  *int                `json:"semantic_neighbors_input_max_chars"`
	SemanticNeighborsTaskTimeout    string              `json:"semantic_neighbors_task_timeout"`
	SemanticNeighborsBatchTimeout   string              `json:"semantic_neighbors_batch_timeout"`
	SLAPromotionEnabled             *bool               `json:"sla_promotion_enabled"`
	SLAPromotionCandidateWindow     *int                `json:"sla_promotion_candidate_window"`
	SLAPromotionRules               *[]SLAPromotionRule `json:"sla_promotion_rules"`
}

func readFileConfig(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("failed to read config file %s: %v", path, err)
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("failed to parse config file %s: %v", path, err)
	}
	return cfg, nil
}

func applyFileConfig(cfg *Config, fileCfg fileConfig) {
	applyRootFileConfig(cfg, fileCfg)
	applyLegacyFileConfig(cfg, fileCfg)
	mergeSchedulerConfig(&cfg.Scheduler, fileCfg.Scheduler)
	mergeControlStateConfig(&cfg.ControlState, fileCfg.ControlState)
	mergeRedisConfig(&cfg.Redis, fileCfg.Redis)
	mergeCacheConfig(&cfg.Cache, fileCfg.Cache)
	syncLegacyConfigFields(cfg)
}

func applyRootFileConfig(cfg *Config, fileCfg fileConfig) {
	if fileCfg.FallbackEnabled != nil {
		cfg.FallbackEnabled = *fileCfg.FallbackEnabled
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
	if fileCfg.SemanticPipelineConfigFile != "" {
		cfg.SemanticPipelineConfigFile = fileCfg.SemanticPipelineConfigFile
	}
	cfg.SchedulerConfigFile = fileCfg.SchedulerConfigFile
	cfg.CacheConfigFile = fileCfg.CacheConfigFile
}

func applyLegacyFileConfig(cfg *Config, fileCfg fileConfig) {
	applyLegacyControlStateConfig(&cfg.ControlState, fileCfg)
	applyLegacyRedisConfig(&cfg.Redis, fileCfg)
	applyLegacyCacheConfig(&cfg.Cache, fileCfg)
}

func applyComponentConfigFiles(cfg *Config) error {
	if cfg.SchedulerConfigFile != "" {
		data, err := os.ReadFile(cfg.SchedulerConfigFile)
		if err != nil {
			return fmt.Errorf("failed to read scheduler_config_file %s: %v", cfg.SchedulerConfigFile, err)
		}
		var scheduler schedulerFileConfig
		if err := json.Unmarshal(data, &scheduler); err != nil {
			return fmt.Errorf("failed to parse scheduler_config_file %s: %v", cfg.SchedulerConfigFile, err)
		}
		mergeSchedulerConfig(&cfg.Scheduler, &scheduler)
	}
	if cfg.CacheConfigFile != "" {
		cacheCfg, err := readCacheConfigFile(cfg.CacheConfigFile)
		if err != nil {
			return err
		}
		mergeCacheConfig(&cfg.Cache, &cacheCfg)
	}
	syncLegacyConfigFields(cfg)
	return nil
}

func readCacheConfigFile(path string) (cacheFileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheFileConfig{}, fmt.Errorf("failed to read cache_config_file %s: %v", path, err)
	}
	var cfg cacheFileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cacheFileConfig{}, fmt.Errorf("failed to parse cache_config_file %s: %v", path, err)
	}
	return cfg, nil
}

func controlStateConfigFromEnv() ControlStateConfig {
	return ControlStateConfig{
		Backend:          getEnv("CONTROL_STATE_BACKEND", "disabled"),
		DSN:              getEnv("CONTROL_STATE_DSN", ""),
		MigrateOnStartup: getEnv("CONTROL_STATE_MIGRATE_ON_STARTUP", "false") == "true",
		LocalSeedEnabled: getEnv("CONTROL_STATE_LOCAL_SEED_ENABLED", "false") == "true",
		EncryptionKey:    getEnv("CONTROL_STATE_ENCRYPTION_KEY", ""),
		AdminAPIKey:      getEnv("ADMIN_API_KEY", ""),
		AuditRetention:   getEnv("AUDIT_RETENTION", "720h"),
	}
}

func redisConfigFromEnv() RedisConfig {
	return RedisConfig{
		Enabled:        getEnv("REDIS_ENABLED", "false") == "true",
		Addr:           getEnv("REDIS_ADDR", ""),
		Password:       getEnv("REDIS_PASSWORD", ""),
		Namespace:      getEnv("REDIS_NAMESPACE", ""),
		HealthTTL:      getEnv("REDIS_HEALTH_TTL", "1m"),
		AuthCacheTTL:   getEnv("REDIS_AUTH_CACHE_TTL", "5m"),
		DegradeToLocal: getEnv("REDIS_DEGRADE_TO_LOCAL", "true") == "true",
	}
}

func cacheConfigFromEnv() CacheConfig {
	return CacheConfig{
		Enabled:         getEnv("SEMANTIC_CACHE_ENABLED", "false") == "true",
		Provider:        getEnv("SEMANTIC_CACHE_PROVIDER", ""),
		VectorStore:     getEnv("SEMANTIC_CACHE_VECTOR_STORE", ""),
		VectorDimension: getEnvInt("SEMANTIC_CACHE_VECTOR_DIMENSION", defaultSemanticCacheVectorDimension),
		PGVector: PGVectorConfig{
			IndexType:       getEnv("PGVECTOR_INDEX_TYPE", defaultPGVectorIndexType),
			HNSWM:           getEnvInt("PGVECTOR_HNSW_M", defaultPGVectorHNSWM),
			HNSWEFConstruct: getEnvInt("PGVECTOR_HNSW_EF_CONSTRUCTION", defaultPGVectorHNSWEFConstruction),
			SearchEF:        getEnvInt("PGVECTOR_SEARCH_EF", defaultPGVectorSearchEF),
		},
		Qdrant: QdrantConfig{
			Addr:   getEnv("QDRANT_ADDR", ""),
			APIKey: getEnv("QDRANT_API_KEY", ""),
		},
	}
}

func applyLegacyControlStateConfig(dst *ControlStateConfig, src fileConfig) {
	if src.ControlStateBackend != "" {
		dst.Backend = src.ControlStateBackend
	}
	if src.ControlStateDSN != "" {
		dst.DSN = src.ControlStateDSN
	}
	if src.ControlStateMigrateOnStartup != nil {
		dst.MigrateOnStartup = *src.ControlStateMigrateOnStartup
	}
	if src.ControlStateLocalSeedEnabled != nil {
		dst.LocalSeedEnabled = *src.ControlStateLocalSeedEnabled
	}
	if src.ControlStateEncryptionKey != "" {
		dst.EncryptionKey = src.ControlStateEncryptionKey
	}
	if src.AdminAPIKey != "" {
		dst.AdminAPIKey = src.AdminAPIKey
	}
	if src.AuditRetention != "" {
		dst.AuditRetention = src.AuditRetention
	}
}

func applyLegacyRedisConfig(dst *RedisConfig, src fileConfig) {
	if src.RedisEnabled != nil {
		dst.Enabled = *src.RedisEnabled
	}
	if src.RedisAddr != "" {
		dst.Addr = src.RedisAddr
	}
	if src.RedisPassword != "" {
		dst.Password = src.RedisPassword
	}
	if src.RedisDB != nil {
		dst.DB = *src.RedisDB
	}
	if src.RedisNamespace != "" {
		dst.Namespace = src.RedisNamespace
	}
	if src.RedisHealthTTL != "" {
		dst.HealthTTL = src.RedisHealthTTL
	}
	if src.RedisAuthCacheTTL != "" {
		dst.AuthCacheTTL = src.RedisAuthCacheTTL
	}
	if src.RedisDegradeToLocal != nil {
		dst.DegradeToLocal = *src.RedisDegradeToLocal
	}
}

func applyLegacyCacheConfig(dst *CacheConfig, src fileConfig) {
	if src.SemanticCacheEnabled != nil {
		dst.Enabled = *src.SemanticCacheEnabled
	}
	if src.SemanticCacheProvider != "" {
		dst.Provider = src.SemanticCacheProvider
	}
	if src.SemanticCacheVectorStore != "" {
		dst.VectorStore = src.SemanticCacheVectorStore
	}
	if src.SemanticCacheVectorDimension != nil {
		dst.VectorDimension = *src.SemanticCacheVectorDimension
	}
	if src.PGVectorIndexType != "" {
		dst.PGVector.IndexType = src.PGVectorIndexType
	}
	if src.PGVectorHNSWM != nil {
		dst.PGVector.HNSWM = *src.PGVectorHNSWM
	}
	if src.PGVectorHNSWEFConstruction != nil {
		dst.PGVector.HNSWEFConstruct = *src.PGVectorHNSWEFConstruction
	}
	if src.PGVectorSearchEF != nil {
		dst.PGVector.SearchEF = *src.PGVectorSearchEF
	}
	if src.QdrantAddr != "" {
		dst.Qdrant.Addr = src.QdrantAddr
	}
	if src.QdrantAPIKey != "" {
		dst.Qdrant.APIKey = src.QdrantAPIKey
	}
}

func mergeControlStateConfig(dst *ControlStateConfig, src *controlStateFileConfig) {
	if src == nil {
		return
	}
	if src.Backend != "" {
		dst.Backend = src.Backend
	}
	if src.DSN != "" {
		dst.DSN = src.DSN
	}
	if src.MigrateOnStartup != nil {
		dst.MigrateOnStartup = *src.MigrateOnStartup
	}
	if src.LocalSeedEnabled != nil {
		dst.LocalSeedEnabled = *src.LocalSeedEnabled
	}
	if src.EncryptionKey != "" {
		dst.EncryptionKey = src.EncryptionKey
	}
	if src.AdminAPIKey != "" {
		dst.AdminAPIKey = src.AdminAPIKey
	}
	if src.AuditRetention != "" {
		dst.AuditRetention = src.AuditRetention
	}
}

func mergeRedisConfig(dst *RedisConfig, src *redisFileConfig) {
	if src == nil {
		return
	}
	if src.Enabled != nil {
		dst.Enabled = *src.Enabled
	}
	if src.Addr != "" {
		dst.Addr = src.Addr
	}
	if src.Password != "" {
		dst.Password = src.Password
	}
	if src.DB != nil {
		dst.DB = *src.DB
	}
	if src.Namespace != "" {
		dst.Namespace = src.Namespace
	}
	if src.HealthTTL != "" {
		dst.HealthTTL = src.HealthTTL
	}
	if src.AuthCacheTTL != "" {
		dst.AuthCacheTTL = src.AuthCacheTTL
	}
	if src.DegradeToLocal != nil {
		dst.DegradeToLocal = *src.DegradeToLocal
	}
}

func mergeCacheConfig(dst *CacheConfig, src *cacheFileConfig) {
	if src == nil {
		return
	}
	if src.Enabled != nil {
		dst.Enabled = *src.Enabled
	}
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.VectorStore != "" {
		dst.VectorStore = src.VectorStore
	}
	if src.VectorDimension != nil {
		dst.VectorDimension = *src.VectorDimension
	}
	mergePGVectorConfig(&dst.PGVector, src.PGVector)
	mergeQdrantConfig(&dst.Qdrant, src.Qdrant)
}

func mergePGVectorConfig(dst *PGVectorConfig, src PGVectorConfig) {
	if src.IndexType != "" {
		dst.IndexType = src.IndexType
	}
	if src.HNSWM != 0 {
		dst.HNSWM = src.HNSWM
	}
	if src.HNSWEFConstruct != 0 {
		dst.HNSWEFConstruct = src.HNSWEFConstruct
	}
	if src.SearchEF != 0 {
		dst.SearchEF = src.SearchEF
	}
}

func mergeQdrantConfig(dst *QdrantConfig, src QdrantConfig) {
	if src.Addr != "" {
		dst.Addr = src.Addr
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
}

func normalizeConfigBlocks(c *Config) {
	if c.ControlState.Backend == "" && c.ControlStateBackend != "" {
		c.ControlState.Backend = c.ControlStateBackend
	}
	if c.ControlStateMigrateOnStartup {
		c.ControlState.MigrateOnStartup = true
	}
	if c.ControlStateLocalSeedEnabled {
		c.ControlState.LocalSeedEnabled = true
	}
	if c.ControlState.DSN == "" {
		c.ControlState.DSN = c.ControlStateDSN
	}
	if c.ControlState.EncryptionKey == "" {
		c.ControlState.EncryptionKey = c.ControlStateEncryptionKey
	}
	if c.ControlState.AuditRetention == "" {
		c.ControlState.AuditRetention = c.AuditRetention
	}
	if c.Redis.Addr == "" {
		c.Redis.Addr = c.RedisAddr
	}
	if c.RedisEnabled {
		c.Redis.Enabled = true
	}
	if c.Redis.DB == 0 {
		c.Redis.DB = c.RedisDB
	}
	if c.Redis.Password == "" {
		c.Redis.Password = c.RedisPassword
	}
	if c.Redis.Namespace == "" {
		c.Redis.Namespace = c.RedisNamespace
	}
	if c.Cache.VectorStore == "" {
		c.Cache.VectorStore = c.SemanticCacheVectorStore
	}
	if c.SemanticCacheEnabled {
		c.Cache.Enabled = true
	}
	if c.Cache.VectorDimension == 0 {
		c.Cache.VectorDimension = c.SemanticCacheVectorDimension
	}
	if c.Cache.Provider == "" {
		c.Cache.Provider = c.SemanticCacheProvider
	}
	if c.Cache.PGVector.IndexType == "" {
		c.Cache.PGVector.IndexType = c.PGVectorIndexType
	}
	if c.Cache.PGVector.HNSWM == 0 {
		c.Cache.PGVector.HNSWM = c.PGVectorHNSWM
	}
	if c.Cache.PGVector.HNSWEFConstruct == 0 {
		c.Cache.PGVector.HNSWEFConstruct = c.PGVectorHNSWEFConstruction
	}
	if c.Cache.PGVector.SearchEF == 0 {
		c.Cache.PGVector.SearchEF = c.PGVectorSearchEF
	}
	if c.Cache.Qdrant.Addr == "" {
		c.Cache.Qdrant.Addr = c.QdrantAddr
	}
	if c.Cache.Qdrant.APIKey == "" {
		c.Cache.Qdrant.APIKey = c.QdrantAPIKey
	}
	syncLegacyConfigFields(c)
}

func syncLegacyConfigFields(c *Config) {
	c.ControlStateBackend = c.ControlState.Backend
	c.ControlStateDSN = c.ControlState.DSN
	c.ControlStateMigrateOnStartup = c.ControlState.MigrateOnStartup
	c.ControlStateLocalSeedEnabled = c.ControlState.LocalSeedEnabled
	c.ControlStateEncryptionKey = c.ControlState.EncryptionKey
	c.AdminAPIKey = c.ControlState.AdminAPIKey
	c.AuditRetention = c.ControlState.AuditRetention
	c.RedisEnabled = c.Redis.Enabled
	c.RedisAddr = c.Redis.Addr
	c.RedisPassword = c.Redis.Password
	c.RedisDB = c.Redis.DB
	c.RedisNamespace = c.Redis.Namespace
	c.RedisHealthTTL = c.Redis.HealthTTL
	c.RedisAuthCacheTTL = c.Redis.AuthCacheTTL
	c.RedisDegradeToLocal = c.Redis.DegradeToLocal
	c.SemanticCacheEnabled = c.Cache.Enabled
	c.SemanticCacheProvider = c.Cache.Provider
	c.SemanticCacheVectorStore = c.Cache.VectorStore
	c.SemanticCacheVectorDimension = c.Cache.VectorDimension
	c.PGVectorIndexType = c.Cache.PGVector.IndexType
	c.PGVectorHNSWM = c.Cache.PGVector.HNSWM
	c.PGVectorHNSWEFConstruction = c.Cache.PGVector.HNSWEFConstruct
	c.PGVectorSearchEF = c.Cache.PGVector.SearchEF
	c.QdrantAddr = c.Cache.Qdrant.Addr
	c.QdrantAPIKey = c.Cache.Qdrant.APIKey
}
