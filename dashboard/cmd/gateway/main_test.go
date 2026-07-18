package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigPrefersRedisEnvironmentOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env2.local")
	if err := os.WriteFile(path, []byte("REDIS_ADDR=file-redis:6379\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REDIS_ADDR", "e2e-redis:26379")

	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.RedisAddr != "e2e-redis:26379" {
		t.Fatalf("expected environment REDIS_ADDR override, got %q", config.RedisAddr)
	}
}

func TestLoadConfigUsesEnvironmentWhenEnvFileIsMissing(t *testing.T) {
	t.Setenv("REDIS_ADDR", "isolated-redis:26379")
	t.Setenv("ADMIN_BOOTSTRAP_USERNAME", "e2e_admin")

	config, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if config.RedisAddr != "isolated-redis:26379" {
		t.Fatalf("expected environment REDIS_ADDR without an env file, got %q", config.RedisAddr)
	}
	if config.BootstrapAdminUsername != "e2e_admin" {
		t.Fatalf("expected environment bootstrap Admin without an env file, got %q", config.BootstrapAdminUsername)
	}
}
