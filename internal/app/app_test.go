package app

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/scheduler"
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
	t.Setenv("SCHEDULER_ENABLED", "true")
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

func TestApp_SchedulerDisabledDoesNotCreateRunner(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("SCHEDULER_ENABLED", "false")

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if application.SchedulerRunner != nil {
		t.Fatalf("expected scheduler runner to be disabled")
	}
	if application.SchedulerQueueBackend != "disabled" {
		t.Fatalf("expected disabled scheduler backend, got %s", application.SchedulerQueueBackend)
	}
	models := application.RuntimeProviderManager.GetAvailableModels()
	if len(models) != 1 || models[0] != "gpt-4o-mini" {
		t.Fatalf("expected real static provider model catalog, got %v", models)
	}
	adapter, decision, err := application.RuntimeProviderManager.Select(context.Background(), &llm.LLMRequest{Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatalf("expected real provider selection: %v", err)
	}
	if _, ok := adapter.(*openai.Adapter); !ok {
		t.Fatalf("expected real OpenAI-compatible adapter, got %T", adapter)
	}
	if decision.ProviderID != "openai-primary" {
		t.Fatalf("expected openai-primary routing decision, got %#v", decision)
	}
	if !adapter.Capabilities().SupportsOperation(providers.OperationChatCompletions) {
		t.Fatalf("expected chat completions capability")
	}
}

func TestNewSchedulerQueueDefaultsToMemoryWhenRedisIsEnabled(t *testing.T) {
	cfg := &config.Config{
		RedisEnabled:   true,
		RedisAddr:      "127.0.0.1:1",
		RedisNamespace: "scheduler-test",
		Scheduler:      config.SchedulerConfig{QueueBackend: "auto"},
	}
	queue, backend := newSchedulerQueue(context.Background(), cfg, discardLogger())
	if backend != "memory" {
		t.Fatalf("expected memory backend, got backend=%s", backend)
	}
	if _, ok := queue.(*scheduler.MemoryQueue); !ok {
		t.Fatalf("expected memory queue, got %T", queue)
	}
}

func TestNewSchedulerQueueExplicitRedisIsNodeScoped(t *testing.T) {
	redisServer := miniredis.RunT(t)
	cfg := &config.Config{
		RedisEnabled:   true,
		RedisAddr:      redisServer.Addr(),
		RedisNamespace: "scheduler-test",
		NodeID:         "node-a",
		Scheduler:      config.SchedulerConfig{QueueBackend: "redis"},
	}
	queue, backend := newSchedulerQueue(context.Background(), cfg, discardLogger())
	if backend != "redis" {
		t.Fatalf("expected redis backend, got backend=%s", backend)
	}
	if _, ok := queue.(*scheduler.RedisQueue); !ok {
		t.Fatalf("expected redis queue, got %T", queue)
	}
	if got := schedulerRedisQueueName(cfg); got != "gateway-node-a" {
		t.Fatalf("queue name=%q, want gateway-node-a", got)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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

func TestApp_SemanticNeighborsMissingDependenciesDoNotBlockStartup(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "true")

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !application.Config.Scheduler.SemanticNeighborsEnabled {
		t.Fatalf("expected config to preserve semantic-neighbor opt-in")
	}
	if application.SchedulerSemanticNeighborsOn {
		t.Fatalf("expected semantic neighbors disabled without durable/vector/embed dependencies")
	}
}

func TestApp_SemanticNeighborsEnsureCollectionKeepsServiceEnabled(t *testing.T) {
	testenv.Load()
	qdrantAddr := os.Getenv("QDRANT_ADDR")
	if qdrantAddr == "" {
		t.Fatalf("QDRANT_ADDR is required for real semantic-neighbor startup test")
	}
	setSemanticNeighborAppEnv(t, qdrantAddr)

	application, err := New()
	if err != nil {
		t.Fatalf("expected semantic-neighbor startup with real qdrant: %v", err)
	}
	if !application.SchedulerSemanticNeighborsOn {
		t.Fatalf("expected semantic neighbors enabled after collection ensure")
	}
	if application.Config.Cache.VectorDimension != 3 {
		t.Fatalf("expected configured cache vector dimension 3, got %d", application.Config.Cache.VectorDimension)
	}
}

func TestApp_SemanticNeighborsPGVectorEnsureKeepsServiceEnabled(t *testing.T) {
	testenv.Load()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Fatalf("POSTGRES_TEST_DSN is required for real pgvector semantic-neighbor startup test")
	}
	dsn = isolatedAppPostgresDSN(t, dsn)
	seedLivePostgresStartupProvider(t, dsn, livePostgresTestEncryptionKey)
	setSemanticNeighborPGVectorAppEnv(t, dsn)

	application, err := New()
	if err != nil {
		t.Fatalf("expected semantic-neighbor startup with real pgvector: %v", err)
	}
	if !application.SchedulerSemanticNeighborsOn {
		t.Fatalf("expected semantic neighbors enabled after pgvector ensure")
	}
	if application.Config.Cache.VectorDimension != 3 {
		t.Fatalf("expected configured cache vector dimension 3, got %d", application.Config.Cache.VectorDimension)
	}
}

func TestApp_SemanticNeighborsEnsureFailureFailsOpen(t *testing.T) {
	setSemanticNeighborAppEnv(t, "127.0.0.1:1")

	application, err := New()
	if err != nil {
		t.Fatalf("expected app startup to fail open: %v", err)
	}
	if application.SchedulerSemanticNeighborsOn {
		t.Fatalf("expected semantic neighbors disabled after qdrant ensure failure")
	}
}

func TestApp_SchedulerHeuristicOnlyStartup(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("SCHEDULER_ENABLED", "true")
	t.Setenv("SCHEDULER_ENDPOINT", "127.0.0.1:1")

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if application.SchedulerRunner == nil {
		t.Fatalf("expected scheduler runner")
	}
}

func TestApp_SchedulerWeightedRolloutStartup(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("SCHEDULER_ENABLED", "true")
	t.Setenv("SCHEDULER_HEURISTIC_ENDPOINT", "127.0.0.1:1")
	t.Setenv("SCHEDULER_ONNX_ENDPOINT", "127.0.0.1:2")
	t.Setenv("SCHEDULER_ONNX_ROLLOUT_PERCENT", "10")

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if application.SchedulerRunner == nil {
		t.Fatalf("expected scheduler runner")
	}
}

func TestApp_SchedulerSLAPromotionWiring(t *testing.T) {
	configPath := t.TempDir() + "/config.json"
	data := `{
		"default_provider": "openai-primary",
		"providers": [{
			"id": "openai-primary",
			"type": "openai-compatible",
			"base_url": "https://api.openai.com/v1",
			"models": ["gpt-4o-mini"]
		}],
		"scheduler": {
			"enabled": true,
			"sla_promotion_enabled": true,
			"sla_promotion_candidate_window": 7,
			"sla_promotion_rules": [{
				"policy_id": "tier-gold-code",
				"tenant_id": "tenant-a",
				"model_class": "large",
				"request_kind": "code_gen",
				"wait_threshold": "2s"
			}]
		}
	}`
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_FILE", configPath)

	application, err := New()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	promoter := application.SchedulerRunner.Executor.Promoter
	if promoter == nil {
		t.Fatalf("expected SLA promoter")
	}
	if promoter.CandidateWindow != 7 || len(promoter.Rules) != 1 {
		t.Fatalf("unexpected promoter config: %#v", promoter)
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

func setSemanticNeighborAppEnv(t *testing.T, qdrantAddr string) {
	t.Helper()
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("DEFAULT_PROVIDER", "openai-primary")
	t.Setenv("OPENAI_PRIMARY_MODELS", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_BASE_URL", "https://api.openai.com/v1")
	t.Setenv("OPENAI_PRIMARY_DEFAULT_MODEL", "gpt-4o-mini")
	t.Setenv("OPENAI_PRIMARY_API_KEY", "test-key")
	t.Setenv("CONTROL_STATE_BACKEND", "sqlite")
	t.Setenv("CONTROL_STATE_DSN", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared")
	t.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	t.Setenv("CONTROL_STATE_LOCAL_SEED_ENABLED", "true")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", livePostgresTestEncryptionKey)
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "true")
	t.Setenv("SEMANTIC_CACHE_PROVIDER", "openai-primary")
	t.Setenv("SEMANTIC_CACHE_VECTOR_STORE", "qdrant")
	t.Setenv("SEMANTIC_CACHE_VECTOR_DIMENSION", "3")
	t.Setenv("QDRANT_ADDR", qdrantAddr)
}

func setSemanticNeighborPGVectorAppEnv(t *testing.T, dsn string) {
	t.Helper()
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
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "true")
	t.Setenv("SEMANTIC_CACHE_PROVIDER", "app-live-provider")
	t.Setenv("SEMANTIC_CACHE_VECTOR_STORE", "pgvector")
	t.Setenv("SEMANTIC_CACHE_VECTOR_DIMENSION", "3")
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
