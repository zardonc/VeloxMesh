package integration

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Set default environment variables for integration tests
	// This simulates the fallback logic that was previously hardcoded in config.go
	os.Setenv("CONFIG_FILE", "")
	os.Setenv("DEFAULT_PROVIDER", "openai-primary")
	os.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	os.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	os.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	os.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")

	os.Exit(m.Run())
}
