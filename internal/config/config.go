package config

import (
	"os"
)

type Config struct {
	GatewayDataAddr   string
	GatewayAdminAddr  string
	GatewayMetricsAddr string
	LogLevel          string
	DevAPIKey         string
	DefaultProvider   string
	PrimaryBaseURL    string
	PrimaryAPIKey     string
	PrimaryModels     string
	PrimaryDefaultModel string
}

func LoadConfig() *Config {
	return &Config{
		GatewayDataAddr:   getEnv("GATEWAY_DATA_ADDR", ":8080"),
		GatewayAdminAddr:  getEnv("GATEWAY_ADMIN_ADDR", ":8081"),
		GatewayMetricsAddr: getEnv("GATEWAY_METRICS_ADDR", ":9090"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		DevAPIKey:         getEnv("DEV_API_KEY", "vx-dev"),
		DefaultProvider:   getEnv("DEFAULT_PROVIDER", "openai-primary"),
		PrimaryBaseURL:    getEnv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1"),
		PrimaryAPIKey:     getEnv("OPENAI_PRIMARY_API_KEY", ""),
		PrimaryModels:     getEnv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini"),
		PrimaryDefaultModel: getEnv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini"),
	}
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
