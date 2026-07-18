package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLoadConfigReadsSMTPPasswordFromDockerSecretFile(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "smtp-password")
	if err := os.WriteFile(secretPath, []byte("docker-secret-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "mailer")
	t.Setenv("SMTP_PASSWORD", "lower-priority-password")
	t.Setenv("SMTP_PASSWORD_FILE", secretPath)
	t.Setenv("SMTP_FROM", "noreply@example.test")
	t.Setenv("SMTP_TLS_MODE", "starttls")

	config, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if config.SMTPPassword != "docker-secret-password" || config.SMTPPasswordFile != secretPath {
		t.Fatalf("SMTP Docker Secret was not loaded with file precedence")
	}
	if config.SMTPHost != "smtp.example.test" || config.SMTPTLSMode != "starttls" {
		t.Fatalf("SMTP connection metadata was not loaded: %+v", config)
	}
}

func TestLoadConfigRejectsPartialOrUnsafeSMTPConfiguration(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_TLS_MODE", "plain")
	_, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"))
	if err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("partial or unsafe SMTP config error = %v", err)
	}
}

func TestLoadConfigReadsAuthenticationSecurityPolicy(t *testing.T) {
	t.Setenv("DASHBOARD_DEMO_MODE", "false")
	t.Setenv("DASHBOARD_TEST_MODE", "true")
	t.Setenv("SESSION_TTL", "45m")
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("VERIFICATION_SEND_EMAIL_LIMIT", "2")
	t.Setenv("VERIFICATION_SEND_IP_LIMIT", "12")
	t.Setenv("VERIFICATION_VERIFY_EMAIL_LIMIT", "8")
	t.Setenv("VERIFICATION_VERIFY_IP_LIMIT", "30")
	t.Setenv("VERIFICATION_RATE_WINDOW", "20m")

	config, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if config.DemoMode || !config.TestMode || !config.SessionCookieSecure || config.SessionTTL != 45*time.Minute {
		t.Fatalf("session/test security policy was not loaded: %+v", config)
	}
	if config.VerificationSendEmailLimit != 2 || config.VerificationSendIPLimit != 12 || config.VerificationVerifyEmailLimit != 8 || config.VerificationVerifyIPLimit != 30 || config.VerificationRateWindow != 20*time.Minute {
		t.Fatalf("verification rate policy was not loaded: %+v", config)
	}
}

func TestLoadConfigReadsGatewayAdminEnvironmentWithoutExposingItToFrontend(t *testing.T) {
	t.Setenv("VELOXMESH_ADMIN_URL", "http://gateway.internal:8080")
	t.Setenv("VELOXMESH_DATA_URL", "http://gateway.internal:8080")
	t.Setenv("VELOXMESH_METRICS_URL", "http://gateway.internal:9090")
	t.Setenv("VELOXMESH_ADMIN_API_KEY", "server-only-key")
	t.Setenv("VELOXMESH_DATA_API_KEY", "server-only-data-key")
	t.Setenv("VELOXMESH_API_TIMEOUT", "17s")
	t.Setenv("DASHBOARD_DEMO_MODE", "true")

	config, err := loadConfig(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if config.GatewayAdminURL != "http://gateway.internal:8080" || config.GatewayDataURL != "http://gateway.internal:8080" || config.GatewayMetricsURL != "http://gateway.internal:9090" {
		t.Fatalf("gateway URLs were not loaded: %+v", config)
	}
	if config.GatewayAdminAPIKey != "server-only-key" || config.GatewayDataAPIKey != "server-only-data-key" || config.GatewayAPITimeout != 17*time.Second {
		t.Fatalf("gateway server-only configuration was not loaded")
	}
	if !config.DemoMode {
		t.Fatal("DASHBOARD_DEMO_MODE OS environment override was ignored")
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
