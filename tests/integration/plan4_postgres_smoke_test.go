package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/app"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/llm"
)

func TestPlan4PostgresSmoke(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	key := os.Getenv("PLAN4_CONTROL_STATE_ENCRYPTION_KEY")
	providerKey := os.Getenv("PLAN4_PROVIDER_API_KEY")
	devKey := os.Getenv("PLAN4_DEV_API_KEY")
	if dsn == "" || key == "" || providerKey == "" || devKey == "" {
		t.Skip("Skipping Plan 4 smoke; POSTGRES_TEST_DSN, PLAN4_CONTROL_STATE_ENCRYPTION_KEY, PLAN4_PROVIDER_API_KEY, and PLAN4_DEV_API_KEY are required")
	}
	dsn = isolatedPlan4PostgresDSN(t, dsn, "plan4_postgres_smoke_test")

	fakeProvider := setupFakeProvider(t, "plan4-provider", 0, http.StatusOK)
	defer fakeProvider.Close()
	seedPlan4Provider(t, dsn, key, providerKey, fakeProvider.URL)

	t.Setenv("CONFIG_FILE", "")
	t.Setenv("CONTROL_STATE_BACKEND", "postgres")
	t.Setenv("CONTROL_STATE_DSN", dsn)
	t.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", key)
	t.Setenv("DEV_API_KEY", devKey)
	t.Setenv("DEFAULT_PROVIDER", "plan4-provider")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", fakeProvider.URL)
	t.Setenv("OPENAI_PRIMARY_API_KEY", providerKey)
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o")

	application, err := app.New()
	if err != nil {
		t.Fatalf("plan4 app startup: %v", err)
	}

	body, _ := json.Marshal(llm.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+devKey)
	rec := httptest.NewRecorder()
	application.Router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected chat 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPlan4PostgresSansPrimaryRealProviderSmoke(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	devKey := os.Getenv("DEV_API_KEY")
	baseURL := os.Getenv("SANS_BASE_URL")
	providerKey := os.Getenv("SANS_PRIMARY_API_KEY")
	modelsRaw := os.Getenv("SANS_PRIMARY_MODELS")
	model := os.Getenv("SANS_PRIMARY_DEFAULT_MODEL")
	if dsn == "" || devKey == "" || baseURL == "" || providerKey == "" || modelsRaw == "" || model == "" {
		t.Skip("Skipping real Plan 4 smoke; POSTGRES_TEST_DSN, DEV_API_KEY, and SANS_* provider vars are required")
	}
	models := splitModels(modelsRaw)
	if len(models) < 2 {
		t.Fatalf("expected sans-primary to expose multiple models, got %d", len(models))
	}
	dsn = isolatedPlan4PostgresDSN(t, dsn, "plan4_sans_primary_real_provider_test")
	key := "12345678901234567890123456789012"
	seedPlan4ProviderConfig(t, dsn, key, providerKey, baseURL, "sans-primary", models, model)

	t.Setenv("CONFIG_FILE", "")
	t.Setenv("CONTROL_STATE_BACKEND", "postgres")
	t.Setenv("CONTROL_STATE_DSN", dsn)
	t.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", key)
	t.Setenv("DEV_API_KEY", devKey)
	t.Setenv("DEFAULT_PROVIDER", "sans-primary")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", baseURL)
	t.Setenv("OPENAI_PRIMARY_API_KEY", providerKey)
	t.Setenv("OPENAI_PRIMARY_MODELS", modelsRaw)
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", model)

	application, err := app.New()
	if err != nil {
		t.Fatalf("plan4 app startup with sans-primary: %v", err)
	}
	serverURL, shutdown := startPlan4HTTPServer(t, application.Router)
	defer shutdown()

	maxTokens := 16
	body, _ := json.Marshal(llm.ChatCompletionRequest{
		Model:     model,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "Reply with one short sentence."}},
		MaxTokens: &maxTokens,
	})
	req, err := http.NewRequest(http.MethodPost, serverURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+devKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("real provider request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected real provider chat 200, got %d", resp.StatusCode)
	}
}

func seedPlan4Provider(t *testing.T, dsn, key, providerKey, baseURL string) {
	seedPlan4ProviderConfig(t, dsn, key, providerKey, baseURL, "plan4-provider", []string{"gpt-4o"}, "gpt-4o")
}

func seedPlan4ProviderConfig(t *testing.T, dsn, key, providerKey, baseURL, providerID string, models []string, model string) {
	t.Helper()
	ctx := context.Background()
	repo, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	mutation := &controlstate.ProviderMutation{
		ID:           providerID,
		Name:         providerID,
		Type:         "openai-compatible",
		BaseURL:      baseURL,
		Enabled:      true,
		Models:       models,
		DefaultModel: &model,
	}
	if _, err := repo.Providers().Create(ctx, mutation); err != nil {
		current, getErr := repo.Providers().Get(ctx, providerID)
		if getErr != nil {
			t.Fatalf("create provider: %v", err)
		}
		mutation.Revision = &current.Revision
		if _, err := repo.Providers().Update(ctx, mutation); err != nil {
			t.Fatalf("update provider: %v", err)
		}
	}
	cipher, err := controlstate.NewAESGCMSecretCipher([]byte(key), "v1")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	encrypted, err := cipher.EncryptProviderSecret([]byte(providerKey))
	if err != nil {
		t.Fatalf("encrypt provider secret: %v", err)
	}
	if err := repo.Providers().PutEncryptedSecret(ctx, providerID, encrypted.Ciphertext, encrypted.Nonce, encrypted.KeyID); err != nil {
		t.Fatalf("store provider secret: %v", err)
	}
	if err := repo.Routing().Save(ctx, &controlstate.RoutingConfig{
		ID:              "default",
		Strategy:        "least-latency",
		DefaultProvider: providerID,
		FallbackEnabled: false,
		MaxAttempts:     1,
	}); err != nil {
		t.Fatalf("save routing: %v", err)
	}
}

func splitModels(raw string) []string {
	parts := strings.Split(raw, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		if model := strings.TrimSpace(part); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func isolatedPlan4PostgresDSN(t *testing.T, dsn, schema string) string {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres for schema setup: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quotePlan4Ident(schema)); err != nil {
		t.Fatalf("create postgres test schema: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func quotePlan4Ident(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func startPlan4HTTPServer(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}
	return "http://" + listener.Addr().String(), shutdown
}
