package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

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
	Models       []string                   `json:"models"`
	DefaultModel string                     `json:"default_model"`
	Timeout      string                     `json:"timeout"`
	Weight       int                        `json:"weight"`
	HealthCheck  *ProviderHealthCheckConfig `json:"health_check"`
}

type Config struct {
	GatewayDataAddr    string
	GatewayAdminAddr   string
	GatewayMetricsAddr string
	LogLevel           string
	DevAPIKey          string

	RoutingStrategy string // e.g. "round-robin", "least-latency"
	DefaultProvider string

	FallbackEnabled bool
	MaxAttempts     int

	HealthCheck HealthCheckConfig

	Providers []ProviderConfig
}

func LoadConfig() (*Config, error) {
	// defaults
	cfg := &Config{
		GatewayDataAddr:    getEnv("GATEWAY_DATA_ADDR", ":8080"),
		GatewayAdminAddr:   getEnv("GATEWAY_ADMIN_ADDR", ":8081"),
		GatewayMetricsAddr: getEnv("GATEWAY_METRICS_ADDR", ":9090"),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		DevAPIKey:          getEnv("DEV_API_KEY", "vx-dev"),
		RoutingStrategy:    getEnv("ROUTING_STRATEGY", "least-latency"),
	}

	configFile := getEnv("CONFIG_FILE", "")
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %v", err)
		}

		var fileCfg struct {
			RoutingStrategy string            `json:"routing_strategy"`
			DefaultProvider string            `json:"default_provider"`
			FallbackEnabled *bool             `json:"fallback_enabled"`
			MaxAttempts     *int              `json:"max_attempts"`
			HealthCheck     HealthCheckConfig `json:"health_check"`
			Providers       []ProviderConfig  `json:"providers"`
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

		if fileCfg.RoutingStrategy != "" {
			cfg.RoutingStrategy = fileCfg.RoutingStrategy
		}
		if fileCfg.DefaultProvider != "" {
			cfg.DefaultProvider = fileCfg.DefaultProvider
		}
		cfg.HealthCheck = fileCfg.HealthCheck
		cfg.Providers = fileCfg.Providers

		if !fallbackEnabledSet {
			cfg.FallbackEnabled = len(cfg.Providers) > 1
		}
	} else {
		// Fallback to backward-compatible env config
		providerID := getEnv("DEFAULT_PROVIDER", "openai-primary")
		cfg.DefaultProvider = providerID

		modelsRaw := getEnv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
		var models []string
		for _, m := range strings.Split(modelsRaw, ",") {
			m = strings.TrimSpace(m)
			if m != "" {
				models = append(models, m)
			}
		}
		if len(models) == 0 {
			models = []string{"gpt-4o-mini"}
		}

		cfg.Providers = []ProviderConfig{
			{
				ID:           providerID,
				Type:         "openai-compatible",
				BaseURL:      getEnv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1"),
				APIKey:       getEnv("OPENAI_PRIMARY_API_KEY", ""),
				Models:       models,
				DefaultModel: getEnv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini"),
				Timeout:      "30s",
			},
		}
	}

	// Apply health check defaults
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

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.RoutingStrategy != "round-robin" && c.RoutingStrategy != "least-latency" {
		return fmt.Errorf("invalid routing strategy")
	}

	if c.MaxAttempts == 0 {
		c.MaxAttempts = 2
	} else if c.MaxAttempts < 1 {
		c.MaxAttempts = 1
	} else if c.MaxAttempts > 5 {
		c.MaxAttempts = 5
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	seen := make(map[string]bool)
	defaultFound := false

	for _, p := range c.Providers {
		if p.ID == "" {
			return fmt.Errorf("empty provider id")
		}
		if seen[p.ID] {
			return fmt.Errorf("duplicate provider id: %s", p.ID)
		}
		seen[p.ID] = true

		if p.Type != "openai-compatible" && p.Type != "anthropic" && p.Type != "gemini" {
			return fmt.Errorf("unsupported provider type for %s", p.ID)
		}
		if p.BaseURL == "" {
			return fmt.Errorf("missing base URL for %s", p.ID)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("missing models for %s", p.ID)
		}
		if p.ID == c.DefaultProvider {
			defaultFound = true
		}
		if p.Timeout != "" {
			if _, err := time.ParseDuration(p.Timeout); err != nil {
				return fmt.Errorf("invalid timeout for %s: %v", p.ID, err)
			}
		}
	}

	if !defaultFound && c.DefaultProvider != "" {
		return fmt.Errorf("default provider not found")
	}

	if err := validateDuration(c.HealthCheck.Interval, "health_check.interval"); err != nil {
		return err
	}
	if err := validateDuration(c.HealthCheck.Timeout, "health_check.timeout"); err != nil {
		return err
	}
	if err := validateDuration(c.HealthCheck.InitialDelay, "health_check.initial_delay"); err != nil {
		return err
	}
	if err := validateDuration(c.HealthCheck.StaleAfter, "health_check.stale_after"); err != nil {
		return err
	}
	if c.HealthCheck.FailureThreshold < 1 {
		return fmt.Errorf("health_check.failure_threshold must be >= 1")
	}
	if c.HealthCheck.SuccessThreshold < 1 {
		return fmt.Errorf("health_check.success_threshold must be >= 1")
	}
	if c.HealthCheck.MaxConcurrency < 1 {
		return fmt.Errorf("health_check.max_concurrency must be >= 1")
	}

	for _, p := range c.Providers {
		if p.HealthCheck != nil {
			if p.HealthCheck.Interval != "" {
				if err := validateDuration(p.HealthCheck.Interval, fmt.Sprintf("provider %s health_check.interval", p.ID)); err != nil {
					return err
				}
			}
			if p.HealthCheck.Timeout != "" {
				if err := validateDuration(p.HealthCheck.Timeout, fmt.Sprintf("provider %s health_check.timeout", p.ID)); err != nil {
					return err
				}
			}
			if p.HealthCheck.InitialDelay != "" {
				if err := validateDuration(p.HealthCheck.InitialDelay, fmt.Sprintf("provider %s health_check.initial_delay", p.ID)); err != nil {
					return err
				}
			}
			if p.HealthCheck.FailureThreshold != 0 && p.HealthCheck.FailureThreshold < 1 {
				return fmt.Errorf("provider %s health_check.failure_threshold must be >= 1", p.ID)
			}
			if p.HealthCheck.SuccessThreshold != 0 && p.HealthCheck.SuccessThreshold < 1 {
				return fmt.Errorf("provider %s health_check.success_threshold must be >= 1", p.ID)
			}
		}
	}

	return nil
}

func validateDuration(d, name string) error {
	dur, err := time.ParseDuration(d)
	if err != nil {
		return fmt.Errorf("invalid duration for %s: %v", name, err)
	}
	if dur < 0 {
		return fmt.Errorf("duration for %s cannot be negative", name)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
