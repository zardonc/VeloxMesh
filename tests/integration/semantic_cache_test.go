package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/pipeline"
)

type mockEmbedAdapter struct {
	id string
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
func (m *memorySemanticCacheRepo) RecordHit(ctx context.Context, id string) error { return nil }
func (m *memorySemanticCacheRepo) Disable(ctx context.Context, id string) error   { return nil }

func TestSemanticCache_CacheHeaders(t *testing.T) {
	ctx := context.Background()
	_ = ctx
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)

	cfg := &config.Config{
		DevAPIKey: "dev-key",
	}

	p1 := &mockEmbedAdapter{id: "p1"}
	registry := providers.NewRegistry(cfg, []providers.ProviderAdapter{p1}, nil)
	route := routing.NewHealthAwareRouter(registry, store, "round-robin")

	cacheRepo := &memorySemanticCacheRepo{}
	semanticCacheSvc := cache.NewSemanticCacheService(cache.SemanticCacheConfig{
		Enabled:       true,
		Threshold:     0.9,
		MaxCandidates: 10,
		TTL:           1 * time.Hour,
	}, cacheRepo, nil, p1)

	gwSvc := gateway.NewService(route, admission.NewPassThroughController(), store, true, 2, nil, semanticCacheSvc, pipeline.DefaultRegistry(), nil, nil)

	appRouter := router.NewRouter(cfg, gwSvc, nil, nil, nil, nil, nil)

	reqBody, _ := json.Marshal(llm.ChatCompletionRequest{
		Model:    "emb",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})

	// First Request - Miss
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req1.Header.Set("Authorization", "Bearer dev-key")
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
	req2.Header.Set("Authorization", "Bearer dev-key")
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
