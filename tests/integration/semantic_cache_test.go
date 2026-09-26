package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	controlsqlite "veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type mockEmbedAdapter struct {
	id    string
	calls int
}

func (m *mockEmbedAdapter) ID() string       { return m.id }
func (m *mockEmbedAdapter) Models() []string { return []string{"emb"} }
func (m *mockEmbedAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeOpenAICompatible,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions, providers.OperationEmbeddings},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
	}
}
func (m *mockEmbedAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return &llm.LLMResponse{
		Provider: m.id,
		Model:    req.Model,
		Choices: []llm.Choice{{
			Index:   0,
			Message: llm.Message{Role: llm.RoleAssistant, Content: "Cached response"},
		}},
	}, nil
}
func (m *mockEmbedAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{}
}
func (m *mockEmbedAdapter) Embed(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	m.calls++
	return &llm.EmbeddingResponse{
		Data: []llm.Embedding{{Index: 0, Embedding: []float32{1.0, 0.0, 0.0}}},
	}, nil
}

type memorySemanticCacheRepo struct {
	entries []*controlstate.SemanticCacheEntry
}

func (m *memorySemanticCacheRepo) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}
func (m *memorySemanticCacheRepo) ListCandidates(ctx context.Context, scope, model string) ([]*controlstate.SemanticCacheEntry, error) {
	return m.entries, nil
}
func (m *memorySemanticCacheRepo) GetCandidate(ctx context.Context, id, scope, model string) (*controlstate.SemanticCacheEntry, error) {
	for _, entry := range m.entries {
		if entry.ID == id && entry.Scope == scope && entry.Model == model && entry.Enabled && entry.ExpiresAt.After(time.Now().UTC()) {
			return entry, nil
		}
	}
	return nil, nil
}
func (m *memorySemanticCacheRepo) RecordHit(ctx context.Context, id string) error { return nil }
func (m *memorySemanticCacheRepo) Disable(ctx context.Context, id string) error   { return nil }

func TestSemanticCache_CacheHeaders(t *testing.T) {
	ctx := context.Background()
	repo, err := controlsqlite.Open(fmt.Sprintf("file:semantic-cache-%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	token := fmt.Sprintf("phase29-token-%d", time.Now().UnixNano())
	keyHash := sha256.Sum256([]byte(token))
	keyID := fmt.Sprintf("phase29-key-%d", time.Now().UnixNano())
	if err := repo.APIKeys().Create(ctx, &controlstate.APIKeyRecord{
		ID: keyID, Hash: hex.EncodeToString(keyHash[:]), Name: "phase29 cache test", Role: "user", Enabled: true, CreditBalance: 1,
	}); err != nil {
		t.Fatalf("create test API key: %v", err)
	}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)

	cfg := &config.Config{}

	p1 := &mockEmbedAdapter{id: "p1"}
	registry := providers.NewRegistry(cfg, []providers.ProviderAdapter{p1}, nil)
	route := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)

	cacheRepo := &memorySemanticCacheRepo{}
	semanticCacheSvc := cache.NewSemanticCacheService(cache.SemanticCacheConfig{
		Enabled: true, Threshold: 0.9, MaxCandidates: 10, TTL: time.Hour,
		EmbeddingModel: "emb", VectorDimension: 3,
		UseCases: []cache.SemanticCacheUseCase{{
			APIKeyIDs: []string{keyID}, UseCaseID: "phase29-static-faq", KnowledgeVersion: "faq-v1", TargetModel: "emb", SystemPrompt: "Static FAQ only",
		}},
	}, cacheRepo, nil, p1)

	gwSvc := gateway.NewService(route, admission.NewPassThroughController(), store, true, 2, nil, semanticCacheSvc, pipeline.DefaultRegistry(), nil, nil)

	appRouter := router.NewRouter(cfg, gwSvc, nil, nil, nil, nil, nil, repo, nil, nil)

	temperature, maxTokens := 0.0, 256
	reqBody, _ := json.Marshal(llm.ChatCompletionRequest{
		Model: "emb", Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: "Static FAQ only"}, {Role: llm.RoleUser, Content: "Hello"}},
	})

	// First Request - Miss
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req1.Header.Set("Authorization", "Bearer "+token)
	rec1 := httptest.NewRecorder()
	appRouter.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("req1 expected 200, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-Cache-Hit") != "false" {
		t.Errorf("req1 expected X-Cache-Hit: false")
	}

	// Second Request - Hit
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	appRouter.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("req2 expected 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-Cache-Hit") != "true" {
		t.Errorf("req2 expected X-Cache-Hit: true")
	}
	if rec2.Header().Get("X-Cache-Level") != "semantic" {
		t.Errorf("req2 expected X-Cache-Level: semantic")
	}
}

func TestSemanticCache_DevKeyBypassesCache(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	cfg := &config.Config{DevAPIKey: "dev-key"}
	p1 := &mockEmbedAdapter{id: "p1"}
	registry := providers.NewRegistry(cfg, []providers.ProviderAdapter{p1}, nil)
	route := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	semanticCacheSvc := cache.NewSemanticCacheService(cache.SemanticCacheConfig{
		Enabled: true, Threshold: 0.9, MaxCandidates: 10, TTL: time.Hour,
	}, &memorySemanticCacheRepo{}, nil, p1)
	gwSvc := gateway.NewService(route, admission.NewPassThroughController(), store, true, 2, nil, semanticCacheSvc, pipeline.DefaultRegistry(), nil, nil)
	appRouter := router.NewRouter(cfg, gwSvc, nil, nil, nil, nil, nil, nil, nil, nil)
	body, _ := json.Marshal(llm.ChatCompletionRequest{Model: "emb", Messages: []llm.Message{{Role: llm.RoleUser, Content: "FAQ"}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer dev-key")
	rec := httptest.NewRecorder()

	appRouter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", rec.Code, http.StatusOK)
	}
	if p1.calls != 0 {
		t.Fatalf("embedding calls=%d, want 0 for development key", p1.calls)
	}
}
