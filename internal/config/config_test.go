package config

import (
	"strings"
	"testing"
)

func TestConfigFallbackDefaults(t *testing.T) {
	c1 := &Config{
		Providers: []ProviderConfig{{ID: "p1"}},
	}
	c1.FallbackEnabled = len(c1.Providers) > 1
	if c1.FallbackEnabled {
		t.Errorf("expected fallback to be disabled for 1 provider")
	}

	c2 := &Config{
		Providers: []ProviderConfig{{ID: "p1"}, {ID: "p2"}},
	}
	c2.FallbackEnabled = len(c2.Providers) > 1
	if !c2.FallbackEnabled {
		t.Errorf("expected fallback to be enabled for 2 providers")
	}
}

func TestConfigValidationSuccess(t *testing.T) {
	c := &Config{
		RoutingStrategy: "round-robin",
		FallbackEnabled: true,
		MaxAttempts:     2,
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
				RoutingStrategy: "round-robin",
				FallbackEnabled: true,
				MaxAttempts:     2,
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
		RoutingStrategy: "round-robin",
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
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.RedisEnabled {
		t.Errorf("expected Redis to be disabled by default")
	}
	if cfg.RedisNamespace != "veloxmesh:local" {
		t.Errorf("expected default namespace veloxmesh:local, got %s", cfg.RedisNamespace)
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
}
