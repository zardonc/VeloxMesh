package app

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/testenv"
)

const livePostgresTestEncryptionKey = "12345678901234567890123456789012"

type dummyRepo struct {
	controlstate.Repository
	prov controlstate.ProviderRepository
}

func (d *dummyRepo) Providers() controlstate.ProviderRepository {
	return d.prov
}

func (d *dummyRepo) Routing() controlstate.RoutingRepository {
	return &dummyRoutingRepo{}
}

func (d *dummyRepo) Combos() controlstate.ComboRepository {
	return nil
}

func (d *dummyRepo) Rates() controlstate.RateRepository {
	return nil
}

func (d *dummyRepo) SemanticRules() controlstate.SemanticRuleStore {
	return nil
}

type dummyRoutingRepo struct {
}

func (d *dummyRoutingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	return nil, controlstate.ErrRoutingConfigNotFound
}

func (d *dummyRoutingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error {
	return nil
}

type dummyProvRepo struct {
	controlstate.ProviderRepository
	records []*controlstate.ProviderRecord
}

func (d *dummyProvRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	var res []*controlstate.ProviderRecord
	for _, rec := range d.records {
		if filter.Enabled != nil && *filter.Enabled != rec.Enabled {
			continue
		}
		res = append(res, rec)
	}
	return res, nil
}

func (d *dummyProvRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return []byte("enc"), []byte("nonce"), "key", nil
}

type dummyCipher struct {
	controlstate.SecretCipher
}

func (d *dummyCipher) DecryptProviderSecret(secret *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("test-key"), nil
}

func TestApp_ReloadProviders(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")

	a, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	repo := &dummyRepo{
		prov: &dummyProvRepo{
			records: []*controlstate.ProviderRecord{
				{
					ID:      "openai-1",
					Type:    "openai-compatible",
					Enabled: true,
					BaseURL: "https://api.openai.com/v1",
					Models:  []string{"gpt-4"},
					Secret:  controlstate.ProviderSecretMetadata{SecretConfigured: true},
				},
			},
		},
	}
	cipher := &dummyCipher{}

	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Verify the router has the new models
	models := a.RuntimeProviderManager.GetAvailableModels()
	if len(models) != 1 || models[0] != "gpt-4" {
		t.Errorf("expected gpt-4 model, got %v", models)
	}
}

func TestApp_SchedulerRedisQueueFailureUsesMemory(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_ADDR", "127.0.0.1:1")
	t.Setenv("REDIS_NAMESPACE", "scheduler-test")
	t.Setenv("REDIS_DEGRADE_TO_LOCAL", "true")
	t.Setenv("SCHEDULER_QUEUE_BACKEND", "redis")

	application, err := New()
	if err != nil {
		t.Fatalf("expected memory queue fallback, got %v", err)
	}
	if application.SchedulerRunner == nil {
		t.Fatalf("expected scheduler runner")
	}
	if application.SchedulerQueueBackend != "memory" {
		t.Fatalf("expected memory scheduler queue, got %s", application.SchedulerQueueBackend)
	}
}

func TestApp_SchedulerFeedbackRequiresDurableControlState(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("SCHEDULER_FEEDBACK_ENABLED", "true")

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !application.Config.Scheduler.FeedbackEnabled {
		t.Fatalf("expected config to preserve feedback opt-in")
	}
	if application.SchedulerFeedbackOn {
		t.Fatalf("expected feedback recording disabled without durable control state")
	}
}

func TestApp_PostgresControlStateFailsClosed(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("CONTROL_STATE_BACKEND", "postgres")
	t.Setenv("CONTROL_STATE_DSN", "postgres://user:pass@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", livePostgresTestEncryptionKey)

	_, err := New()
	if err == nil {
		t.Fatalf("expected postgres startup failure")
	}
	if !strings.Contains(err.Error(), "failed to open repository") {
		t.Fatalf("expected repository open failure, got %v", err)
	}
}

func TestApp_PostgresControlStateStartsWithLiveDSN(t *testing.T) {
	testenv.Load()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("Skipping live postgres startup test because POSTGRES_TEST_DSN is not set")
	}
	dsn = isolatedAppPostgresDSN(t, dsn)
	seedLivePostgresStartupProvider(t, dsn, livePostgresTestEncryptionKey)

	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "app-live-provider")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("CONTROL_STATE_BACKEND", "postgres")
	t.Setenv("CONTROL_STATE_DSN", dsn)
	t.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", livePostgresTestEncryptionKey)

	application, err := New()
	if err != nil {
		t.Fatalf("expected live postgres startup: %v", err)
	}
	if application.Router == nil {
		t.Fatalf("expected initialized router")
	}
}

func isolatedAppPostgresDSN(t *testing.T, dsn string) string {
	t.Helper()
	const schema = "app_live_startup_test"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres for schema setup: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quotePostgresIdent(schema)); err != nil {
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

func seedLivePostgresStartupProvider(t *testing.T, dsn, key string) {
	t.Helper()
	ctx := context.Background()
	repo, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open live postgres: %v", err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate live postgres: %v", err)
	}
	model := "gpt-4o-mini"
	mutation := &controlstate.ProviderMutation{
		ID:           "app-live-provider",
		Name:         "App Live Provider",
		Type:         "openai-compatible",
		BaseURL:      "https://api.openai.com/v1",
		Enabled:      true,
		Models:       []string{model},
		DefaultModel: &model,
	}
	if _, err := repo.Providers().Create(ctx, mutation); err != nil {
		current, getErr := repo.Providers().Get(ctx, mutation.ID)
		if getErr != nil {
			t.Fatalf("create live startup provider: %v", err)
		}
		mutation.Revision = &current.Revision
		if _, err := repo.Providers().Update(ctx, mutation); err != nil {
			t.Fatalf("update live startup provider: %v", err)
		}
	}
	seedLivePostgresProviderSecrets(t, ctx, repo, key)
}

func seedLivePostgresProviderSecrets(t *testing.T, ctx context.Context, repo *postgres.Repository, key string) {
	t.Helper()
	cipher, err := controlstate.NewAESGCMSecretCipher([]byte(key), "v1")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	encrypted, err := cipher.EncryptProviderSecret([]byte("test-key"))
	if err != nil {
		t.Fatalf("encrypt provider secret: %v", err)
	}
	enabled := true
	records, err := repo.Providers().List(ctx, controlstate.ProviderFilter{Enabled: &enabled})
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	for _, record := range records {
		normalizeLivePostgresProvider(t, ctx, repo, record)
		err := repo.Providers().PutEncryptedSecret(ctx, record.ID, encrypted.Ciphertext, encrypted.Nonce, encrypted.KeyID)
		if err != nil {
			t.Fatalf("store provider secret for %s: %v", record.ID, err)
		}
	}
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func normalizeLivePostgresProvider(t *testing.T, ctx context.Context, repo *postgres.Repository, record *controlstate.ProviderRecord) {
	t.Helper()
	providerType := record.Type
	validType := providerType == "openai-compatible" || providerType == "anthropic" || providerType == "gemini"
	if len(record.Models) > 0 && record.DefaultModel != "" && validType && record.BaseURL != "" {
		return
	}
	model := "gpt-4o-mini"
	name := record.Name
	if name == "" {
		name = record.ID
	}
	if !validType {
		providerType = "openai-compatible"
	}
	baseURL := record.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	revision := record.Revision
	_, err := repo.Providers().Update(ctx, &controlstate.ProviderMutation{
		ID:           record.ID,
		Name:         name,
		Type:         providerType,
		BaseURL:      baseURL,
		Enabled:      record.Enabled,
		Models:       []string{model},
		DefaultModel: &model,
		Revision:     &revision,
	})
	if err != nil {
		t.Fatalf("normalize provider %s: %v", record.ID, err)
	}
}
