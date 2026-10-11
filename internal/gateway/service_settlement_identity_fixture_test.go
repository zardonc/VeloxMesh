package gateway_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/storage"
)

const (
	identityTraceID              = "reused-client-trace"
	identityProviderID           = "identity-provider"
	identityModel                = "gpt-4o"
	identityUserA                = "identity-user-a"
	identityUserB                = "identity-user-b"
	identitySystemPrompt         = "Answer this static FAQ."
	identityInitialBalance int64 = 100
	identityRate           int64 = 1000
	identityRequestCost    int64 = 2
	identityWriteWait            = 3 * time.Second
	identityQueueCapacity        = 4
)

type identityFixtureOptions struct {
	cache, missingUsage bool
	embedding           providers.EmbedAdapter
	vector              storage.VectorAdapter
	readTimeout         time.Duration
}

type identityFixture struct {
	repo    *sqlite.Repository
	db      *sql.DB
	service *gateway.Service
	cache   *cache.SemanticCacheService
	adapter *identityAdapter
	stages  *identityStageRecorder
}

type identityAdapter struct {
	mockAdapter
	missingUsage bool
	traceIDs     []string
}

func (a *identityAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	a.traceIDs = append(a.traceIDs, req.RequestID)
	identity := middleware.GetAuthIdentity(ctx)
	response := textResponse("answer for " + identity.ID)
	if a.missingUsage {
		response.Usage = nil
	}
	return response, nil
}

type identityStageRecorder struct {
	*observability.StubMetrics
	mu        sync.Mutex
	stages    []observability.StageMeasurement
	completed chan struct{}
}

func (r *identityStageRecorder) RecordStage(stage observability.StageMeasurement) {
	r.mu.Lock()
	r.stages = append(r.stages, stage)
	r.mu.Unlock()
	if stage.Name == "store_total" {
		r.completed <- struct{}{}
	}
}

func (r *identityStageRecorder) snapshot() []observability.StageMeasurement {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observability.StageMeasurement(nil), r.stages...)
}

func (r *identityStageRecorder) waitForWrite(t *testing.T) {
	t.Helper()
	timer := time.NewTimer(identityWriteWait)
	defer timer.Stop()
	select {
	case <-r.completed:
	case <-timer.C:
		t.Fatalf("cache write did not finish within %s; stages=%+v", identityWriteWait, r.snapshot())
	}
}

func newIdentityFixture(t *testing.T, options identityFixtureOptions) *identityFixture {
	t.Helper()
	previous := observability.DefaultMetrics
	stages := &identityStageRecorder{StubMetrics: observability.NewStubMetrics(), completed: make(chan struct{}, identityQueueCapacity)}
	observability.DefaultMetrics = stages
	t.Cleanup(func() { observability.DefaultMetrics = previous })
	repo, observer := openIdentitySQLite(t)
	seedIdentitySQLite(t, repo)
	adapter := &identityAdapter{mockAdapter: mockAdapter{id: identityProviderID, models: []string{identityModel}}, missingUsage: options.missingUsage}
	store := health.NewInMemoryStore()
	store.EnsureProvider(identityProviderID, 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	var semanticCache *cache.SemanticCacheService
	if options.cache {
		cacheConfig := identityCacheConfig()
		if options.readTimeout > 0 {
			cacheConfig.ReadTimeout, cacheConfig.ReadConcurrency = options.readTimeout, identityQueueCapacity
		}
		embedding := options.embedding
		if embedding == nil {
			embedding = &embeddingSpy{}
		}
		semanticCache = cache.NewSemanticCacheService(cacheConfig, repo.SemanticCache(), options.vector, embedding)
		t.Cleanup(semanticCache.Close)
	}
	service := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, repo, semanticCache, pipeline.DefaultRegistry(), nil, nil)
	return &identityFixture{repo: repo, db: observer, service: service, cache: semanticCache, adapter: adapter, stages: stages}
}

func openIdentitySQLite(t *testing.T) (*sqlite.Repository, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settlement-identity.db")
	repo, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	observer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := observer.Close(); err != nil {
			t.Error(err)
		}
	})
	return repo, observer
}

func seedIdentitySQLite(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.Providers().Create(ctx, &controlstate.ProviderMutation{ID: identityProviderID, Name: "Identity fixture", Type: "openai-compatible", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rate := &controlstate.ProviderModelRate{ProviderID: identityProviderID, Model: identityModel, InputCreditRate: identityRate, OutputCreditRate: identityRate}
	if err := repo.Rates().Save(ctx, rate); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{identityUserA, identityUserB} {
		key := &controlstate.APIKeyRecord{ID: id, Prefix: id, Hash: id, Name: id, Role: "user", Enabled: true, CreditBalance: identityInitialBalance}
		if err := repo.APIKeys().Create(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
}

func identityCacheConfig() cache.SemanticCacheConfig {
	return cache.SemanticCacheConfig{
		Enabled: true, Threshold: 0.99, MaxCandidates: identityQueueCapacity, TTL: time.Minute,
		VectorDimension: 1, EmbeddingModel: "text-embedding-3-small",
		WriteWorkers: 1, QueueCapacity: identityQueueCapacity, WriteTimeout: time.Second, ShutdownGrace: identityWriteWait,
		UseCases: []cache.SemanticCacheUseCase{{ReuseMode: "semantic", APIKeyIDs: []string{identityUserA, identityUserB},
			UseCaseID: "identity-faq", KnowledgeVersion: "v1", TargetModel: identityModel, SystemPrompt: identitySystemPrompt}},
	}
}

func identityRequest(question string) *llm.LLMRequest {
	temperature, maxTokens := 0.0, 256
	return &llm.LLMRequest{RequestID: identityTraceID, Model: identityModel, Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: identitySystemPrompt}, {Role: llm.RoleUser, Content: question}}}
}

func (f *identityFixture) call(t *testing.T, key, question string) *llm.LLMRequest {
	t.Helper()
	request := identityRequest(question)
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, &middleware.AuthIdentity{ID: key, Role: "user"})
	response, err := f.service.HandleChatCompletion(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || len(response.Choices) != 1 || response.Choices[0].Message.Content != "answer for "+key {
		t.Fatalf("unexpected response for %s: %+v", key, response)
	}
	if request.RequestID != identityTraceID {
		t.Errorf("request trace changed to %q", request.RequestID)
	}
	return request
}

type identityUsage struct {
	ID, Key, Status string
	Credits         sql.NullInt64
}

func (f *identityFixture) usages(t *testing.T) []identityUsage {
	t.Helper()
	rows, err := f.db.QueryContext(context.Background(), "SELECT id, api_key_id, status, credits_consumed FROM usage_records ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var records []identityUsage
	for rows.Next() {
		var record identityUsage
		if err := rows.Scan(&record.ID, &record.Key, &record.Status, &record.Credits); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func (f *identityFixture) assertBalance(t *testing.T, key string, expected int64) {
	t.Helper()
	record, err := f.repo.APIKeys().GetByHash(context.Background(), key)
	if err != nil || record == nil {
		t.Fatalf("read key %s: record=%+v error=%v", key, record, err)
	}
	if record.CreditBalance != expected {
		t.Errorf("%s balance=%d, want %d", key, record.CreditBalance, expected)
	}
}

func (f *identityFixture) candidates(t *testing.T, key string) []*controlstate.SemanticCacheEntry {
	t.Helper()
	scope, eligible := f.cache.Eligible(key, "user", identityRequest("question"))
	if !eligible {
		t.Fatal("fixture request must be cache eligible")
	}
	entries, err := f.repo.SemanticCache().ListCandidates(context.Background(), scope, identityModel)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}
