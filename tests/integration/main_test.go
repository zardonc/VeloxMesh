package integration

import (
	"os"
	"testing"

	"veloxmesh/internal/testenv"
)

func TestMain(m *testing.M) {
	testenv.Load()
	// Set default environment variables for integration tests
	// This simulates the fallback logic that was previously hardcoded in config.go
	setDefaultEnv("CONFIG_FILE", "")
	setDefaultEnv("DEFAULT_PROVIDER", "openai-primary")
	setDefaultEnv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	setDefaultEnv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	setDefaultEnv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	setDefaultEnv("OPENAI_PRIMARY_API_KEY", "test-key")
	setDefaultEnv("DEV_API_KEY", "test-dev-key")

	os.Exit(m.Run())
}

func setDefaultEnv(key, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}
