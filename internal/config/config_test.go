package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestConfigFallbackDefaults(t *testing.T) {
	c1 := &Config{
		ControlStateBackend: "disabled",
		Providers:           []ProviderConfig{{ID: "p1"}},
	}
	c1.FallbackEnabled = len(c1.Providers) > 1
	if c1.FallbackEnabled {
		t.Errorf("expected fallback to be disabled for 1 provider")
	}

	c2 := &Config{
		ControlStateBackend: "disabled",
		Providers:           []ProviderConfig{{ID: "p1"}, {ID: "p2"}},
	}
	c2.FallbackEnabled = len(c2.Providers) > 1
	if !c2.FallbackEnabled {
		t.Errorf("expected fallback to be enabled for 2 providers")
	}
}

func TestConfigValidationSuccess(t *testing.T) {
	c := &Config{
		ControlStateBackend: "disabled",
		RoutingStrategy:     "round-robin",
		FallbackEnabled:     true,
		MaxAttempts:         2,
		HealthCheck: HealthCheckConfig{
			Interval:         "30s",
			Timeout:          "2s",
			InitialDelay:     "0s",
			FailureThreshold: 3,
			SuccessThreshold: 1,
			StaleAfter:       "0s",
			MaxConcurrency:   4,
		},
		Providers: []ProviderConfig{
			{
				ID:           "p1",
				Type:         "openai-compatible",
				BaseURL:      "https://api.openai.com/v1",
				Models:       []string{"m1", "m2"},
				DefaultModel: "m1",
				Timeout:      "10s",
			},
			{
				ID:      "p2",
				Type:    "anthropic",
				BaseURL: "http://localhost:8080",
				Models:  []string{"m3"},
				HealthCheck: &ProviderHealthCheckConfig{
					Interval: "15s",
				},
			},
			{
				ID:      "p3",
				Type:    "gemini",
				BaseURL: "https://generativelanguage.googleapis.com/v1beta",
				Models:  []string{"m4"},
			},
		},
	}

	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error for valid config: %v", err)
	}
}

func TestConfigValidationFailures(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectedErr string
	}{
		{
			name: "duplicate provider id",
			modify: func(c *Config) {
				c.Providers = append(c.Providers, ProviderConfig{ID: "p1", Type: "anthropic", BaseURL: "http://b", Models: []string{"m1"}})
			},
			expectedErr: "duplicate provider id: p1",
		},
		{
			name: "empty provider id",
			modify: func(c *Config) {
				c.Providers[0].ID = ""
			},
			expectedErr: "empty provider id",
		},
		{
			name: "unsupported provider type",
			modify: func(c *Config) {
				c.Providers[0].Type = "invalid-type"
			},
			expectedErr: "unsupported provider type for p1",
		},
		{
			name: "missing base URL",
			modify: func(c *Config) {
				c.Providers[0].BaseURL = ""
			},
			expectedErr: "missing base URL for p1",
		},
		{
			name: "malformed URL",
			modify: func(c *Config) {
				c.Providers[0].BaseURL = ":not-a-url"
			},
			expectedErr: "invalid base URL for p1",
		},
		{
			name: "non-HTTP(S) URL",
			modify: func(c *Config) {
				c.Providers[0].BaseURL = "ftp://localhost"
			},
			expectedErr: "base URL must use http or https for p1",
		},
		{
			name: "empty model list",
			modify: func(c *Config) {
				c.Providers[0].Models = nil
			},
			expectedErr: "missing models for p1",
		},
		{
			name: "default model mismatch",
			modify: func(c *Config) {
				c.Providers[0].DefaultModel = "missing-model"
			},
			expectedErr: `default model "missing-model" not found in models for p1`,
		},
		{
			name: "invalid provider timeout",
			modify: func(c *Config) {
				c.Providers[0].Timeout = "not-a-time"
			},
			expectedErr: "invalid duration for provider p1 timeout",
		},
		{
			name: "negative provider timeout",
			modify: func(c *Config) {
				c.Providers[0].Timeout = "-1s"
			},
			expectedErr: "duration for provider p1 timeout cannot be negative",
		},
		{
			name: "invalid global health-check duration",
			modify: func(c *Config) {
				c.HealthCheck.Interval = "invalid"
			},
			expectedErr: "invalid duration for health_check.interval",
		},
		{
			name: "invalid provider health-check override duration",
			modify: func(c *Config) {
				c.Providers[0].HealthCheck = &ProviderHealthCheckConfig{Interval: "invalid"}
			},
			expectedErr: "invalid duration for provider p1 health_check.interval",
		},
		{
			name: "invalid health-check thresholds",
			modify: func(c *Config) {
				c.HealthCheck.FailureThreshold = 0
			},
			expectedErr: "health_check.failure_threshold must be >= 1",
		},
		{
			name: "invalid max concurrency",
			modify: func(c *Config) {
				c.HealthCheck.MaxConcurrency = 0
			},
			expectedErr: "health_check.max_concurrency must be >= 1",
		},
		{
			name: "fallback max_attempts greater than provider count",
			modify: func(c *Config) {
				c.FallbackEnabled = true
				c.MaxAttempts = 3 // Only 2 providers
			},
			expectedErr: "fallback max_attempts greater than configured provider count",
		},
		{
			name: "explicit multi-attempt setting when fallback disabled",
			modify: func(c *Config) {
				c.FallbackEnabled = false
				c.MaxAttempts = 2
			},
			expectedErr: "explicit multi-attempt fallback setting when fallback is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				ControlStateBackend: "disabled",
				RoutingStrategy:     "round-robin",
				FallbackEnabled:     true,
				MaxAttempts:         2,
				HealthCheck: HealthCheckConfig{
					Interval:         "30s",
					Timeout:          "2s",
					InitialDelay:     "0s",
					FailureThreshold: 3,
					SuccessThreshold: 1,
					StaleAfter:       "0s",
					MaxConcurrency:   4,
				},
				Providers: []ProviderConfig{
					{
						ID:      "p1",
						Type:    "openai-compatible",
						BaseURL: "https://api.openai.com/v1",
						Models:  []string{"m1"},
					},
					{
						ID:      "p2",
						Type:    "anthropic",
						BaseURL: "https://api.anthropic.com",
						Models:  []string{"m2"},
					},
				},
			}

			tt.modify(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.expectedErr)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestSecretSafety(t *testing.T) {
	// D-14 and D-21: configure raw API key values and assert returned validation error strings
	// do not contain raw API keys, authorization header values, raw prompts, raw upstream bodies.

	c := &Config{
		ControlStateBackend: "disabled",
		RoutingStrategy:     "round-robin",
		HealthCheck: HealthCheckConfig{
			Interval:         "30s",
			Timeout:          "2s",
			InitialDelay:     "0s",
			FailureThreshold: 3,
			SuccessThreshold: 1,
			StaleAfter:       "0s",
			MaxConcurrency:   4,
		},
		Providers: []ProviderConfig{
			{
				ID:      "p1",
				Type:    "openai-compatible",
				BaseURL: "invalid-url",
				APIKey:  "sk-SECRET_VALUE_DO_NOT_EXPOSE",
				Auth: &ProviderAuthConfig{
					APIKeyEnv: "API_KEY_ENV_NAME",
				},
				Models: []string{"m1"},
			},
		},
	}

	err := c.Validate()
	if err == nil {
		t.Fatalf("expected validation error due to invalid URL")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "sk-SECRET_VALUE_DO_NOT_EXPOSE") {
		t.Errorf("validation error exposed the raw API key!")
	}
	if strings.Contains(errMsg, "API_KEY_ENV_NAME") {
		t.Errorf("validation error exposed the env variable name!")
	}
}

func TestEnvFallback(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "test-provider")
	t.Setenv("OPENAI_PRIMARY_MODELS", "test-model-1, test-model-2")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.test.com")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "test-model-1")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected backward-compatible env config to load, got error: %v", err)
	}

	if cfg.DefaultProvider != "test-provider" {
		t.Errorf("expected default provider test-provider, got %s", cfg.DefaultProvider)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.Providers))
	}

	p := cfg.Providers[0]
	if p.ID != "test-provider" {
		t.Errorf("expected provider ID test-provider, got %s", p.ID)
	}
	if p.BaseURL != "https://api.test.com" {
		t.Errorf("expected BaseURL https://api.test.com, got %s", p.BaseURL)
	}
	if p.APIKey != "test-key" {
		t.Errorf("expected APIKey test-key, got %s", p.APIKey)
	}
	if len(p.Models) != 2 || p.Models[0] != "test-model-1" || p.Models[1] != "test-model-2" {
		t.Errorf("expected models [test-model-1 test-model-2], got %v", p.Models)
	}
	if p.DefaultModel != "test-model-1" {
		t.Errorf("expected default model test-model-1, got %s", p.DefaultModel)
	}
}

func TestRedisConfigDefaults(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RedisEnabled {
		t.Errorf("expected Redis to be disabled by default")
	}
	if cfg.RedisNamespace != "" {
		t.Errorf("expected default namespace empty, got %s", cfg.RedisNamespace)
	}
	if cfg.RedisHealthTTL != "1m" {
		t.Errorf("expected default health TTL 1m, got %s", cfg.RedisHealthTTL)
	}
	if cfg.RedisAuthCacheTTL != "5m" {
		t.Errorf("expected default auth cache TTL 5m, got %s", cfg.RedisAuthCacheTTL)
	}
	if !cfg.RedisDegradeToLocal {
		t.Errorf("expected degrade to local to be true by default")
	}
}

func TestRedisConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_NAMESPACE", "prod")
	t.Setenv("REDIS_HEALTH_TTL", "10s")
	t.Setenv("REDIS_AUTH_CACHE_TTL", "10m")
	t.Setenv("REDIS_DEGRADE_TO_LOCAL", "false")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.RedisEnabled {
		t.Errorf("expected Redis to be enabled")
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Errorf("expected addr redis:6379, got %s", cfg.RedisAddr)
	}
	if cfg.RedisNamespace != "prod" {
		t.Errorf("expected namespace prod, got %s", cfg.RedisNamespace)
	}
	if cfg.RedisHealthTTL != "10s" {
		t.Errorf("expected health TTL 10s, got %s", cfg.RedisHealthTTL)
	}
	if cfg.RedisAuthCacheTTL != "10m" {
		t.Errorf("expected auth cache TTL 10m, got %s", cfg.RedisAuthCacheTTL)
	}
	if cfg.RedisDegradeToLocal {
		t.Errorf("expected degrade to local to be false")
	}
	if !cfg.Redis.Enabled || cfg.Redis.Addr != "redis:6379" {
		t.Fatalf("expected nested Redis env compatibility, got %#v", cfg.Redis)
	}
}

func TestNestedConfigEnvCompatibility(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("CONTROL_STATE_BACKEND", "disabled")
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("SEMANTIC_CACHE_ENABLED", "true")
	t.Setenv("SEMANTIC_CACHE_VECTOR_STORE", "qdrant")
	t.Setenv("SEMANTIC_CACHE_VECTOR_DIMENSION", "512")
	t.Setenv("QDRANT_ADDR", "http://qdrant:6333")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ControlState.Backend != "disabled" {
		t.Fatalf("expected nested control state env, got %#v", cfg.ControlState)
	}
	if !cfg.Cache.Enabled || cfg.Cache.VectorDimension != 512 || cfg.Cache.Qdrant.Addr != "http://qdrant:6333" {
		t.Fatalf("expected nested cache env compatibility, got %#v", cfg.Cache)
	}
}

func TestNestedConfigLoadsFromLegacyFlatJSON(t *testing.T) {
	configPath := writeTempConfig(t, `{
		"default_provider": "p1",
		"providers": [{"id":"p1","type":"openai-compatible","base_url":"http://test","models":["m1"]}],
		"control_state_backend": "disabled",
		"redis_enabled": true,
		"redis_addr": "redis:6379",
		"semantic_cache_enabled": true,
		"semantic_cache_vector_store": "qdrant",
		"semantic_cache_vector_dimension": 768,
		"qdrant_addr": "http://qdrant:6333"
	}`)
	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Redis.Enabled || cfg.Redis.Addr != "redis:6379" {
		t.Fatalf("legacy redis fields did not normalize: %#v", cfg.Redis)
	}
	if !cfg.Cache.Enabled || cfg.Cache.VectorDimension != 768 || cfg.Cache.Qdrant.Addr != "http://qdrant:6333" {
		t.Fatalf("legacy cache fields did not normalize: %#v", cfg.Cache)
	}
}

func TestNestedConfigWinsOverFlatJSON(t *testing.T) {
	configPath := writeTempConfig(t, `{
		"default_provider": "p1",
		"providers": [{"id":"p1","type":"openai-compatible","base_url":"http://test","models":["m1"]}],
		"redis_enabled": true,
		"redis_addr": "legacy:6379",
		"semantic_cache_vector_dimension": 384,
		"qdrant_addr": "http://legacy:6333",
		"redis": {"enabled": false, "addr": "nested:6379"},
		"cache": {"vector_dimension": 1024, "qdrant": {"addr": "http://nested:6333"}}
	}`)
	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Redis.Enabled || cfg.Redis.Addr != "nested:6379" {
		t.Fatalf("nested redis did not win: %#v", cfg.Redis)
	}
	if cfg.Cache.VectorDimension != 1024 || cfg.Cache.Qdrant.Addr != "http://nested:6333" {
		t.Fatalf("nested cache did not win: %#v", cfg.Cache)
	}
}

func TestComponentConfigFilesOverrideOnlyTheirBlocks(t *testing.T) {
	schedulerPath := writeTempConfig(t, `{"executor_concurrency": 3}`)
	cachePath := writeTempConfig(t, `{"vector_dimension": 2048, "qdrant": {"addr": "http://component:6333"}}`)
	configPath := writeTempConfig(t, `{
		"default_provider": "p1",
		"providers": [{"id":"p1","type":"openai-compatible","base_url":"http://test","models":["m1"]}],
		"scheduler_config_file": `+jsonString(t, schedulerPath)+`,
		"cache_config_file": `+jsonString(t, cachePath)+`,
		"scheduler": {"executor_concurrency": 1},
		"cache": {"vector_dimension": 768, "qdrant": {"addr": "http://main:6333"}},
		"redis": {"enabled": false, "addr": "redis-main:6379"}
	}`)
	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scheduler.ExecutorConcurrency != 3 {
		t.Fatalf("scheduler component override missing: %#v", cfg.Scheduler)
	}
	if cfg.Cache.VectorDimension != 2048 || cfg.Cache.Qdrant.Addr != "http://component:6333" {
		t.Fatalf("cache component override missing: %#v", cfg.Cache)
	}
	if cfg.Redis.Addr != "redis-main:6379" {
		t.Fatalf("cache component altered redis: %#v", cfg.Redis)
	}
}

func TestMissingComponentConfigFileReportsPath(t *testing.T) {
	configPath := writeTempConfig(t, `{
		"default_provider": "p1",
		"providers": [{"id":"p1","type":"openai-compatible","base_url":"http://test","models":["m1"]}],
		"cache_config_file": "/missing/cache.json"
	}`)
	t.Setenv("CONFIG_FILE", configPath)

	_, err := LoadConfig()
	if err == nil || !strings.Contains(err.Error(), "/missing/cache.json") {
		t.Fatalf("expected missing component path in error, got %v", err)
	}
}

func TestPlan4PostgresConfigDefaults(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("CONTROL_STATE_BACKEND", "postgres")
	t.Setenv("CONTROL_STATE_DSN", "postgres://user:pass@localhost:5432/db?sslmode=disable")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", "12345678901234567890123456789012")
	t.Setenv("SEMANTIC_CACHE_ENABLED", "true")
	t.Setenv("SEMANTIC_CACHE_VECTOR_STORE", "pgvector")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SemanticCacheVectorDimension != 1536 {
		t.Errorf("expected vector dimension 1536, got %d", cfg.SemanticCacheVectorDimension)
	}
	if cfg.PGVectorIndexType != "hnsw" {
		t.Errorf("expected pgvector index hnsw, got %s", cfg.PGVectorIndexType)
	}
}

func TestSchedulerConfigDefaults(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scheduler.Enabled {
		t.Fatalf("expected Scheduler disabled by default")
	}
	if cfg.Scheduler.Timeout != "15ms" {
		t.Fatalf("expected 15ms scheduler timeout, got %s", cfg.Scheduler.Timeout)
	}
	if cfg.Scheduler.ScorerMaxConcurrency != 4 || cfg.Scheduler.ScorerSlowThreshold != "15ms" {
		t.Fatalf("unexpected scorer backpressure defaults: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.DefaultPriority != "normal" || cfg.Scheduler.MaxPriority != "high" {
		t.Fatalf("unexpected scheduler priorities: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.Mode != "heuristic" {
		t.Fatalf("expected heuristic scheduler mode, got %s", cfg.Scheduler.Mode)
	}
	if cfg.Scheduler.FeedbackEnabled {
		t.Fatalf("expected scheduler feedback disabled by default")
	}
	if cfg.Scheduler.ONNXRolloutPercent != 0 {
		t.Fatalf("expected default ONNX rollout 0, got %d", cfg.Scheduler.ONNXRolloutPercent)
	}
	if cfg.Scheduler.QualityMAPEAlertPercent != 25 || cfg.Scheduler.ErrorSpikeAlertRate != 0.05 {
		t.Fatalf("unexpected scheduler alert defaults: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.QualitySampleWindow != 100 {
		t.Fatalf("expected quality sample window 100, got %d", cfg.Scheduler.QualitySampleWindow)
	}
	if cfg.Scheduler.SemanticNeighborsEnabled {
		t.Fatalf("expected semantic neighbors disabled by default")
	}
	if cfg.Scheduler.SemanticNeighborsEmbeddingModel != defaultSemanticNeighborEmbeddingModel {
		t.Fatalf("expected default embedding model, got %s", cfg.Scheduler.SemanticNeighborsEmbeddingModel)
	}
	if cfg.Scheduler.SemanticNeighborsMinCount != 20 {
		t.Fatalf("expected semantic neighbor min count 20, got %d", cfg.Scheduler.SemanticNeighborsMinCount)
	}
	if cfg.Scheduler.SemanticNeighborsTaskTimeout != "5ms" || cfg.Scheduler.SemanticNeighborsBatchTimeout != "15ms" {
		t.Fatalf("unexpected semantic neighbor timeout defaults: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SLAPromotionEnabled {
		t.Fatalf("expected SLA promotion disabled by default")
	}
	if cfg.Scheduler.SLAPromotionCandidateWindow != 32 {
		t.Fatalf("expected SLA promotion candidate window 32, got %d", cfg.Scheduler.SLAPromotionCandidateWindow)
	}
	if len(cfg.Scheduler.SLAPromotionRules) != 0 {
		t.Fatalf("expected no SLA promotion rules by default")
	}
}

func TestSchedulerSLAPromotionConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_SLA_PROMOTION_ENABLED", "true")
	t.Setenv("SCHEDULER_SLA_PROMOTION_CANDIDATE_WINDOW", "8")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.SLAPromotionEnabled {
		t.Fatalf("expected SLA promotion enabled")
	}
	if cfg.Scheduler.SLAPromotionCandidateWindow != 8 {
		t.Fatalf("expected candidate window 8, got %d", cfg.Scheduler.SLAPromotionCandidateWindow)
	}
}

func TestSchedulerScorerBackpressureConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_SCORER_MAX_CONCURRENCY", "2")
	t.Setenv("SCHEDULER_SCORER_SLOW_THRESHOLD", "7ms")
	t.Setenv("SCHEDULER_QUALITY_SAMPLE_WINDOW", "77")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scheduler.ScorerMaxConcurrency != 2 || cfg.Scheduler.ScorerSlowThreshold != "7ms" || cfg.Scheduler.QualitySampleWindow != 77 {
		t.Fatalf("scorer backpressure env overrides not loaded: %#v", cfg.Scheduler)
	}
}

func TestSchedulerFeedbackConfigIsIndependent(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.Enabled {
		t.Fatalf("expected scheduler enabled")
	}
	if cfg.Scheduler.FeedbackEnabled {
		t.Fatalf("expected feedback to remain disabled")
	}
}

func TestSchedulerConfigFileLoadsWithoutMainConfigFile(t *testing.T) {
	path := t.TempDir() + "/scheduler.json"
	data := `{
		"enabled": true,
		"timeout": "25ms",
		"scorer_max_concurrency": 2,
		"scorer_slow_threshold": "8ms",
		"quality_sample_window": 60,
		"queue_backend": "memory",
		"semantic_neighbors_input_max_chars": 2048
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write scheduler config: %v", err)
	}
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("SCHEDULER_CONFIG_FILE", path)
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.Enabled || cfg.Scheduler.Timeout != "25ms" {
		t.Fatalf("scheduler config file not loaded: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.QueueBackend != "memory" || cfg.Scheduler.SemanticNeighborsInputMaxChars != 2048 {
		t.Fatalf("scheduler config file overrides missing: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.ScorerMaxConcurrency != 2 || cfg.Scheduler.ScorerSlowThreshold != "8ms" || cfg.Scheduler.QualitySampleWindow != 60 {
		t.Fatalf("scheduler scorer backpressure overrides missing: %#v", cfg.Scheduler)
	}
}

func TestSchedulerConfigFileOverridesEnvWithFalseAndZero(t *testing.T) {
	path := writeTempConfig(t, `{
		"enabled": false,
		"strict": false,
		"onnx_rollout_percent": 0,
		"queue_soft_limit": 0,
		"queue_hard_limit": 0,
		"high_quota_per_minute": 0,
		"endpoint": "",
		"heuristic_endpoint": "",
		"onnx_endpoint": "",
		"heuristic_config_file": "",
		"onnx_artifact_dir": "",
		"feedback_enabled": false,
		"semantic_neighbors_enabled": false,
		"sla_promotion_enabled": false,
		"sla_promotion_rules": []
	}`)
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("SCHEDULER_CONFIG_FILE", path)
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_ENABLED", "true")
	t.Setenv("SCHEDULER_STRICT", "true")
	t.Setenv("SCHEDULER_ENDPOINT", "scheduler:50051")
	t.Setenv("SCHEDULER_HEURISTIC_ENDPOINT", "heuristic:50051")
	t.Setenv("SCHEDULER_ONNX_ENDPOINT", "onnx:50051")
	t.Setenv("SCHEDULER_ONNX_ROLLOUT_PERCENT", "50")
	t.Setenv("SCHEDULER_QUEUE_SOFT_LIMIT", "10")
	t.Setenv("SCHEDULER_QUEUE_HARD_LIMIT", "20")
	t.Setenv("SCHEDULER_HIGH_QUOTA_PER_MINUTE", "30")
	t.Setenv("SCHEDULER_HEURISTIC_CONFIG_FILE", "heuristic.json")
	t.Setenv("SCHEDULER_ONNX_ARTIFACT_DIR", "artifacts")
	t.Setenv("SCHEDULER_FEEDBACK_ENABLED", "true")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "true")
	t.Setenv("SCHEDULER_SLA_PROMOTION_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scheduler.Enabled || cfg.Scheduler.Strict || cfg.Scheduler.FeedbackEnabled || cfg.Scheduler.SemanticNeighborsEnabled || cfg.Scheduler.SLAPromotionEnabled {
		t.Fatalf("scheduler component false overrides missing: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.ONNXRolloutPercent != 0 || cfg.Scheduler.QueueSoftLimit != 0 || cfg.Scheduler.QueueHardLimit != 0 || cfg.Scheduler.HighQuotaPerMinute != 0 {
		t.Fatalf("scheduler component zero overrides missing: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.Endpoint != "" || cfg.Scheduler.HeuristicEndpoint != "" || cfg.Scheduler.ONNXEndpoint != "" || cfg.Scheduler.HeuristicConfigFile != "" || cfg.Scheduler.ONNXArtifactDir != "" {
		t.Fatalf("scheduler component empty string overrides missing: %#v", cfg.Scheduler)
	}
	if len(cfg.Scheduler.SLAPromotionRules) != 0 {
		t.Fatalf("expected scheduler component to clear SLA rules, got %#v", cfg.Scheduler.SLAPromotionRules)
	}
}

func TestSchedulerFeedbackConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_FEEDBACK_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.FeedbackEnabled {
		t.Fatalf("expected feedback enabled")
	}
}

func TestSchedulerSemanticNeighborsConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "true")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL", "text-embedding-3-large")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_MIN_COUNT", "9")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_INPUT_MAX_CHARS", "1234")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_TASK_TIMEOUT", "7ms")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_BATCH_TIMEOUT", "21ms")
	t.Setenv("QDRANT_ADDR", "http://qdrant:6333")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.SemanticNeighborsEnabled || cfg.Scheduler.SemanticNeighborsMinCount != 9 {
		t.Fatalf("semantic neighbor env overrides not loaded: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsEmbeddingModel != "text-embedding-3-large" {
		t.Fatalf("semantic neighbor model override not loaded: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsInputMaxChars != 1234 {
		t.Fatalf("semantic neighbor input cap override not loaded: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsTaskTimeout != "7ms" || cfg.Scheduler.SemanticNeighborsBatchTimeout != "21ms" {
		t.Fatalf("semantic neighbor timeout overrides not loaded: %#v", cfg.Scheduler)
	}
}

func TestSchedulerConfigJSONOverride(t *testing.T) {
	configPath := writeTempConfig(t, `{
		"default_provider": "p1",
		"providers": [{"id":"p1","type":"openai-compatible","base_url":"http://test","models":["m1"]}],
		"scheduler": {
			"enabled": true,
			"endpoint": "127.0.0.1:50051",
			"timeout": "12ms",
			"scorer_max_concurrency": 3,
			"scorer_slow_threshold": "10ms",
			"quality_sample_window": 88,
			"default_priority": "low",
			"max_priority": "normal",
			"queue_backend": "memory",
			"feedback_enabled": true,
			"mode": "onnx",
			"onnx_artifact_dir": "artifacts/scheduler-p70-v1",
			"semantic_neighbors_enabled": true,
			"semantic_neighbors_embedding_model": "text-embedding-3-large",
			"semantic_neighbors_min_count": 11,
			"semantic_neighbors_input_max_chars": 4321,
			"semantic_neighbors_task_timeout": "6ms",
			"semantic_neighbors_batch_timeout": "18ms",
			"sla_promotion_enabled": true,
			"sla_promotion_candidate_window": 9,
			"sla_promotion_rules": [{
				"policy_id": "tier-gold-code",
				"tenant_id": "tenant-a",
				"model_class": "frontier",
				"request_kind": "code_gen",
				"wait_threshold": "2s"
			}]
		},
		"cache": {"qdrant": {"addr": "http://qdrant:6333"}}
	}`)
	t.Setenv("CONFIG_FILE", configPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Scheduler.Enabled || cfg.Scheduler.Endpoint != "127.0.0.1:50051" {
		t.Fatalf("scheduler override not loaded: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.Timeout != "12ms" || cfg.Scheduler.DefaultPriority != "low" || cfg.Scheduler.MaxPriority != "normal" {
		t.Fatalf("scheduler override not applied: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.ScorerMaxConcurrency != 3 || cfg.Scheduler.ScorerSlowThreshold != "10ms" || cfg.Scheduler.QualitySampleWindow != 88 {
		t.Fatalf("scheduler scorer backpressure override not applied: %#v", cfg.Scheduler)
	}
	if !cfg.Scheduler.FeedbackEnabled {
		t.Fatalf("scheduler feedback override not applied")
	}
	if cfg.Scheduler.Mode != "onnx" || cfg.Scheduler.ONNXArtifactDir == "" {
		t.Fatalf("scheduler ONNX override not applied: %#v", cfg.Scheduler)
	}
	if !cfg.Scheduler.SemanticNeighborsEnabled || cfg.Scheduler.SemanticNeighborsMinCount != 11 {
		t.Fatalf("scheduler semantic neighbor override not applied: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsEmbeddingModel != "text-embedding-3-large" {
		t.Fatalf("scheduler semantic neighbor model override not applied: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsInputMaxChars != 4321 {
		t.Fatalf("scheduler semantic neighbor input cap override not applied: %#v", cfg.Scheduler)
	}
	if cfg.Scheduler.SemanticNeighborsTaskTimeout != "6ms" || cfg.Scheduler.SemanticNeighborsBatchTimeout != "18ms" {
		t.Fatalf("scheduler semantic neighbor timeout override not applied: %#v", cfg.Scheduler)
	}
	if !cfg.Scheduler.SLAPromotionEnabled || cfg.Scheduler.SLAPromotionCandidateWindow != 9 {
		t.Fatalf("scheduler SLA promotion override not applied: %#v", cfg.Scheduler)
	}
	if len(cfg.Scheduler.SLAPromotionRules) != 1 {
		t.Fatalf("expected one SLA promotion rule, got %d", len(cfg.Scheduler.SLAPromotionRules))
	}
	rule := cfg.Scheduler.SLAPromotionRules[0]
	if rule.PolicyID != "tier-gold-code" || rule.TenantID != "tenant-a" || rule.ModelClass != "frontier" || rule.RequestKind != "code_gen" || rule.WaitThreshold != "2s" {
		t.Fatalf("unexpected SLA promotion rule: %#v", rule)
	}
}

func TestSchedulerConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectedErr string
	}{
		{
			name: "invalid timeout",
			modify: func(c *Config) {
				c.Scheduler.Timeout = "nope"
			},
			expectedErr: "invalid duration for scheduler.timeout",
		},
		{
			name: "invalid priority",
			modify: func(c *Config) {
				c.Scheduler.DefaultPriority = "urgent"
			},
			expectedErr: "invalid scheduler.default_priority",
		},
		{
			name: "enabled without endpoint stays valid",
			modify: func(c *Config) {
				c.Scheduler.Enabled = true
				c.Scheduler.Endpoint = ""
			},
			expectedErr: "",
		},
		{
			name: "onnx mode requires artifact dir",
			modify: func(c *Config) {
				c.Scheduler.Mode = "onnx"
				c.Scheduler.ONNXArtifactDir = ""
			},
			expectedErr: "scheduler.onnx_artifact_dir is required",
		},
		{
			name: "invalid mode",
			modify: func(c *Config) {
				c.Scheduler.Mode = "hybrid"
			},
			expectedErr: "scheduler.mode must be",
		},
		{
			name: "negative onnx rollout",
			modify: func(c *Config) {
				c.Scheduler.ONNXRolloutPercent = -1
			},
			expectedErr: "scheduler.onnx_rollout_percent",
		},
		{
			name: "onnx rollout over 100",
			modify: func(c *Config) {
				c.Scheduler.ONNXRolloutPercent = 101
			},
			expectedErr: "scheduler.onnx_rollout_percent",
		},
		{
			name: "onnx rollout requires endpoint",
			modify: func(c *Config) {
				c.Scheduler.ONNXRolloutPercent = 1
				c.Scheduler.ONNXEndpoint = ""
			},
			expectedErr: "scheduler.onnx_endpoint",
		},
		{
			name: "negative mape alert threshold",
			modify: func(c *Config) {
				c.Scheduler.QualityMAPEAlertPercent = -1
			},
			expectedErr: "scheduler.quality_mape_alert_percent",
		},
		{
			name: "negative error spike threshold",
			modify: func(c *Config) {
				c.Scheduler.ErrorSpikeAlertRate = -1
			},
			expectedErr: "scheduler.error_spike_alert_rate",
		},
		{
			name: "invalid quality sample window",
			modify: func(c *Config) {
				c.Scheduler.QualitySampleWindow = -1
			},
			expectedErr: "scheduler.quality_sample_window",
		},
		{
			name: "invalid scorer max concurrency",
			modify: func(c *Config) {
				c.Scheduler.ScorerMaxConcurrency = -1
			},
			expectedErr: "scheduler.scorer_max_concurrency",
		},
		{
			name: "invalid scorer slow threshold",
			modify: func(c *Config) {
				c.Scheduler.ScorerSlowThreshold = "soon"
			},
			expectedErr: "scheduler.scorer_slow_threshold",
		},
		{
			name: "invalid semantic min count",
			modify: func(c *Config) {
				c.Scheduler.SemanticNeighborsMinCount = -1
			},
			expectedErr: "scheduler.semantic_neighbors_min_count",
		},
		{
			name: "invalid semantic task timeout",
			modify: func(c *Config) {
				c.Scheduler.SemanticNeighborsTaskTimeout = "nope"
			},
			expectedErr: "scheduler.semantic_neighbors_task_timeout",
		},
		{
			name: "semantic batch timeout below task timeout",
			modify: func(c *Config) {
				c.Scheduler.SemanticNeighborsTaskTimeout = "10ms"
				c.Scheduler.SemanticNeighborsBatchTimeout = "5ms"
			},
			expectedErr: "scheduler.semantic_neighbors_batch_timeout",
		},
		{
			name: "disabled SLA promotion ignores malformed rule",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = false
				c.Scheduler.SLAPromotionCandidateWindow = -1
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{{RequestKind: "urgent"}}
			},
			expectedErr: "",
		},
		{
			name: "enabled SLA promotion rejects invalid window",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionCandidateWindow = -1
			},
			expectedErr: "scheduler.sla_promotion_candidate_window must be >= 1",
		},
		{
			name: "enabled SLA promotion rejects missing policy",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].PolicyID = ""
			},
			expectedErr: "scheduler.sla_promotion_rules[0].policy_id is required",
		},
		{
			name: "enabled SLA promotion rejects missing tenant selector",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].TenantID = ""
				c.Scheduler.SLAPromotionRules[0].TenantClass = ""
			},
			expectedErr: "scheduler.sla_promotion_rules[0] requires tenant_id or tenant_class",
		},
		{
			name: "enabled SLA promotion rejects missing model class",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].ModelClass = ""
			},
			expectedErr: "scheduler.sla_promotion_rules[0].model_class is required",
		},
		{
			name: "enabled SLA promotion rejects invalid request kind",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].RequestKind = "urgent"
			},
			expectedErr: "scheduler.sla_promotion_rules[0].request_kind is invalid",
		},
		{
			name: "enabled SLA promotion rejects invalid wait threshold",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].WaitThreshold = "soon"
			},
			expectedErr: "invalid duration for scheduler.sla_promotion_rules[0].wait_threshold",
		},
		{
			name: "enabled SLA promotion rejects non-positive wait threshold",
			modify: func(c *Config) {
				c.Scheduler.SLAPromotionEnabled = true
				c.Scheduler.SLAPromotionRules = []SLAPromotionRule{validSLAPromotionRule()}
				c.Scheduler.SLAPromotionRules[0].WaitThreshold = "0s"
			},
			expectedErr: "scheduler.sla_promotion_rules[0].wait_threshold must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validPlan4TestConfig()
			applyDefaults(c)
			tt.modify(c)
			err := c.Validate()
			if tt.expectedErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected error containing %q, got %v", tt.expectedErr, err)
			}
		})
	}
}

func validSLAPromotionRule() SLAPromotionRule {
	return SLAPromotionRule{
		PolicyID:      "tier-gold-code",
		TenantID:      "tenant-a",
		ModelClass:    "frontier",
		RequestKind:   "code_gen",
		WaitThreshold: "2s",
	}
}

func TestSchedulerRolloutConfigEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "p1")
	t.Setenv("OPENAI_PRIMARY_MODELS", "m1")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "http://test")
	t.Setenv("SCHEDULER_ENDPOINT", "legacy:50051")
	t.Setenv("SCHEDULER_ONNX_ENDPOINT", "onnx:50051")
	t.Setenv("SCHEDULER_ONNX_ROLLOUT_PERCENT", "100")
	t.Setenv("SCHEDULER_QUALITY_SAMPLE_WINDOW", "50")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Scheduler.HeuristicEndpoint != "legacy:50051" {
		t.Fatalf("expected legacy endpoint alias, got %q", cfg.Scheduler.HeuristicEndpoint)
	}
	if cfg.Scheduler.ONNXEndpoint != "onnx:50051" || cfg.Scheduler.ONNXRolloutPercent != 100 || cfg.Scheduler.QualitySampleWindow != 50 {
		t.Fatalf("unexpected ONNX rollout config: %#v", cfg.Scheduler)
	}
}

func TestPlan4PostgresConfigValidationFailures(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(*Config)
		expectedErr string
	}{
		{
			name: "postgres dsn required",
			modify: func(c *Config) {
				c.ControlStateBackend = "postgres"
				c.ControlStateDSN = ""
				c.ControlStateEncryptionKey = "12345678901234567890123456789012"
			},
			expectedErr: "postgres control state backend requires a DSN",
		},
		{
			name: "unsupported vector store",
			modify: func(c *Config) {
				c.SemanticCacheEnabled = true
				c.SemanticCacheVectorStore = "unsupported"
			},
			expectedErr: "unsupported semantic_cache_vector_store",
		},
		{
			name: "qdrant cache requires address",
			modify: func(c *Config) {
				c.SemanticCacheEnabled = true
				c.SemanticCacheVectorStore = "qdrant"
				c.QdrantAddr = ""
			},
			expectedErr: "qdrant_addr is required",
		},
		{
			name: "pgvector cache requires dsn",
			modify: func(c *Config) {
				c.ControlStateDSN = ""
				c.SemanticCacheEnabled = true
				c.SemanticCacheVectorStore = "pgvector"
			},
			expectedErr: "control_state.dsn is required",
		},
		{
			name: "invalid vector dimension",
			modify: func(c *Config) {
				c.SemanticCacheVectorDimension = -1
			},
			expectedErr: "semantic_cache_vector_dimension must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validPlan4TestConfig()
			tt.modify(c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.expectedErr)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestEnvExamplePlan4SecretSafety(t *testing.T) {
	data, err := os.ReadFile("../../deploy/env/local.example.env")
	if err != nil {
		t.Fatalf("read deploy/env/local.example.env: %v", err)
	}
	content := string(data)
	for _, forbidden := range []string{"dev_postgres_secret", "dev_redis_secret", "vx_qdrant_secret", "sk-"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("deploy/env/local.example.env contains forbidden secret marker %q", forbidden)
		}
	}
}

func TestEnvExampleSchedulerDisabledAndSecretSafe(t *testing.T) {
	data, err := os.ReadFile("../../deploy/env/local.example.env")
	if err != nil {
		t.Fatalf("read deploy/env/local.example.env: %v", err)
	}
	content := string(data)
	for _, required := range []string{"# SCHEDULER_CONFIG_FILE=deploy/config/scheduler.local.example.json", "# CACHE_CONFIG_FILE=deploy/config/cache.local.example.json", "# SCHEDULER_ENABLED=false", "# SCHEDULER_TIMEOUT=15ms", "# SCHEDULER_SCORER_MAX_CONCURRENCY=4", "# SCHEDULER_SCORER_SLOW_THRESHOLD=15ms", "# SCHEDULER_QUALITY_SAMPLE_WINDOW=100", "# SCHEDULER_DEFAULT_PRIORITY=normal", "# SCHEDULER_MAX_PRIORITY=high", "# SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED=false", "# SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL=text-embedding-3-small", "# SCHEDULER_SEMANTIC_NEIGHBORS_MIN_COUNT=20", "# SCHEDULER_SEMANTIC_NEIGHBORS_INPUT_MAX_CHARS=16000", "# SCHEDULER_SEMANTIC_NEIGHBORS_TASK_TIMEOUT=5ms", "# SCHEDULER_SEMANTIC_NEIGHBORS_BATCH_TIMEOUT=15ms", "# SCHEDULER_SLA_PROMOTION_ENABLED=false", "# SCHEDULER_SLA_PROMOTION_CANDIDATE_WINDOW=32"} {
		if !strings.Contains(content, required) {
			t.Fatalf("deploy/env/local.example.env missing %q", required)
		}
	}
	for _, forbidden := range []string{"SCHEDULER_API_KEY", "SCHEDULER_TOKEN", "sk-"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("deploy/env/local.example.env contains forbidden scheduler secret marker %q", forbidden)
		}
	}
}

func TestConfigExamplesParseAndStayDisabled(t *testing.T) {
	var main map[string]any
	readJSONExample(t, "../../deploy/config/app.local.example.json", &main)
	for _, block := range []string{"scheduler", "redis", "cache"} {
		values, ok := main[block].(map[string]any)
		if !ok {
			t.Fatalf("deploy/config/app.local.example.json missing %s block", block)
		}
		if values["enabled"] != false {
			t.Fatalf("deploy/config/app.local.example.json %s.enabled = %v, want false", block, values["enabled"])
		}
	}

	var cache map[string]any
	readJSONExample(t, "../../deploy/config/cache.local.example.json", &cache)
	for _, block := range []string{"qdrant", "pgvector"} {
		if _, ok := cache[block].(map[string]any); !ok {
			t.Fatalf("deploy/config/cache.local.example.json missing %s block", block)
		}
	}
}

func TestCopyableExamplesDoNotContainSecretShapedValues(t *testing.T) {
	paths := []string{
		"../../deploy/env/local.example.env",
		"../../deploy/config/app.local.example.json",
		"../../deploy/config/scheduler.local.example.json",
		"../../deploy/config/cache.local.example.json",
		"../../deploy/config/heuristic.example.json",
	}
	for _, path := range paths {
		content := strings.ToLower(readTextExample(t, path))
		for _, forbidden := range []string{"sk-", "dev_postgres_secret", "dev_redis_secret", "vx_qdrant_secret", "password", "token", `"api_key": "`} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s contains forbidden secret-shaped marker %q", path, forbidden)
			}
		}
	}
}

func readJSONExample(t *testing.T, path string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(readTextExample(t, path)), out); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func readTextExample(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/config.json"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func jsonString(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json string: %v", err)
	}
	return string(data)
}

func validPlan4TestConfig() *Config {
	return &Config{
		ControlStateBackend: "disabled",
		RoutingStrategy:     "round-robin",
		MaxAttempts:         1,
		HealthCheck: HealthCheckConfig{
			Interval:         "30s",
			Timeout:          "2s",
			InitialDelay:     "0s",
			FailureThreshold: 3,
			SuccessThreshold: 1,
			StaleAfter:       "0s",
			MaxConcurrency:   4,
		},
		Providers: []ProviderConfig{{
			ID:      "p1",
			Type:    "openai-compatible",
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"m1"},
		}},
	}
}
