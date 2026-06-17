package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
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

	RoutingStrategy string // e.g. "round-robin", "least-latency"
	DefaultProvider string

	FallbackEnabled bool
	MaxAttempts     int

	HealthCheck HealthCheckConfig

	Providers []ProviderConfig
}

func LoadConfig() (*Config, error) {
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

	applyDefaults(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
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
}

func (c *Config) Validate() error {
	if c.RoutingStrategy != "round-robin" && c.RoutingStrategy != "least-latency" {
		return fmt.Errorf("invalid routing strategy")
	}

	if err := validateFallback(c); err != nil {
		return err
	}

	if len(c.Providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	seen := make(map[string]bool)
	defaultFound := false

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

func validateProvider(p *ProviderConfig) error {
	if p.ID == "" {
		return fmt.Errorf("empty provider id")
	}
	if p.Type != "openai-compatible" && p.Type != "anthropic" && p.Type != "gemini" {
		return fmt.Errorf("unsupported provider type for %s", p.ID)
	}
	if err := validateProviderBaseURL(p.ID, p.BaseURL); err != nil {
		return err
	}
	if err := validateProviderModels(p); err != nil {
		return err
	}
	if p.Timeout != "" {
		if err := validateDurationField(p.Timeout, fmt.Sprintf("provider %s timeout", p.ID)); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderBaseURL(id, baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("missing base URL for %s", id)
	}
	u, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL for %s", id)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base URL must use http or https for %s", id)
	}
	if u.Host == "" {
		return fmt.Errorf("base URL host cannot be empty for %s", id)
	}
	return nil
}

func validateProviderModels(p *ProviderConfig) error {
	if len(p.Models) == 0 {
		return fmt.Errorf("missing models for %s", p.ID)
	}
	if p.DefaultModel != "" {
		found := false
		for _, m := range p.Models {
			if m == p.DefaultModel {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("default model %q not found in models for %s", p.DefaultModel, p.ID)
		}
	}
	return nil
}

func validateFallback(c *Config) error {
	if c.MaxAttempts < 1 {
		return fmt.Errorf("fallback max_attempts must be >= 1")
	}
	if !c.FallbackEnabled && c.MaxAttempts > 1 {
		return fmt.Errorf("explicit multi-attempt fallback setting when fallback is disabled")
	}
	if c.FallbackEnabled && c.MaxAttempts > len(c.Providers) {
		return fmt.Errorf("fallback max_attempts greater than configured provider count")
	}
	return nil
}

func validateHealthCheckConfig(hc *HealthCheckConfig) error {
	if err := validateDurationField(hc.Interval, "health_check.interval"); err != nil {
		return err
	}
	if err := validateDurationField(hc.Timeout, "health_check.timeout"); err != nil {
		return err
	}
	if err := validateDurationField(hc.InitialDelay, "health_check.initial_delay"); err != nil {
		return err
	}
	if err := validateDurationField(hc.StaleAfter, "health_check.stale_after"); err != nil {
		return err
	}
	if hc.FailureThreshold < 1 {
		return fmt.Errorf("health_check.failure_threshold must be >= 1")
	}
	if hc.SuccessThreshold < 1 {
		return fmt.Errorf("health_check.success_threshold must be >= 1")
	}
	if hc.MaxConcurrency < 1 {
		return fmt.Errorf("health_check.max_concurrency must be >= 1")
	}
	return nil
}

func validateProviderHealthCheck(p *ProviderConfig) error {
	if p.HealthCheck.Interval != "" {
		if err := validateDurationField(p.HealthCheck.Interval, fmt.Sprintf("provider %s health_check.interval", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.Timeout != "" {
		if err := validateDurationField(p.HealthCheck.Timeout, fmt.Sprintf("provider %s health_check.timeout", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.InitialDelay != "" {
		if err := validateDurationField(p.HealthCheck.InitialDelay, fmt.Sprintf("provider %s health_check.initial_delay", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.FailureThreshold != 0 && p.HealthCheck.FailureThreshold < 1 {
		return fmt.Errorf("provider %s health_check.failure_threshold must be >= 1", p.ID)
	}
	if p.HealthCheck.SuccessThreshold != 0 && p.HealthCheck.SuccessThreshold < 1 {
		return fmt.Errorf("provider %s health_check.success_threshold must be >= 1", p.ID)
	}
	return nil
}

func validateDurationField(d, name string) error {
	dur, err := time.ParseDuration(d)
	if err != nil {
		return fmt.Errorf("invalid duration for %s", name)
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
