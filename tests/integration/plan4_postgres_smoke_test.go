package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func seedPlan4Provider(t *testing.T, dsn, key, providerKey, baseURL string) {
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
	model := "gpt-4o"
	mutation := &controlstate.ProviderMutation{
		ID:           "plan4-provider",
		Name:         "Plan 4 Provider",
		Type:         "openai-compatible",
		BaseURL:      baseURL,
		Enabled:      true,
		Models:       []string{model},
		DefaultModel: &model,
	}
	if _, err := repo.Providers().Create(ctx, mutation); err != nil {
		current, getErr := repo.Providers().Get(ctx, "plan4-provider")
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
	if err := repo.Providers().PutEncryptedSecret(ctx, "plan4-provider", encrypted.Ciphertext, encrypted.Nonce, encrypted.KeyID); err != nil {
		t.Fatalf("store provider secret: %v", err)
	}
	if err := repo.Routing().Save(ctx, &controlstate.RoutingConfig{
		ID:              "default",
		Strategy:        "least-latency",
		DefaultProvider: "plan4-provider",
		FallbackEnabled: false,
		MaxAttempts:     1,
	}); err != nil {
		t.Fatalf("save routing: %v", err)
	}
}
