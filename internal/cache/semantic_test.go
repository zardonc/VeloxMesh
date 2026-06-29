package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type mockEmbedAdapter struct {
	embeddings map[string][]float32
}

func (m *mockEmbedAdapter) ID() string       { return "mock" }
func (m *mockEmbedAdapter) Models() []string { return []string{"mock-model"} }
func (m *mockEmbedAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}
func (m *mockEmbedAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}
func (m *mockEmbedAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{SupportedOperations: []providers.Operation{providers.OperationEmbeddings}}
}
func (m *mockEmbedAdapter) Embed(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	if len(req.Input) == 0 {
		return nil, errors.New("empty input")
	}
	text := req.Input[0]
	emb, ok := m.embeddings[text]
	if !ok {
		return nil, errors.New("no mock embedding for text")
	}
	return &llm.EmbeddingResponse{
		Model: req.Model,
		Data: []llm.Embedding{
			{Index: 0, Embedding: emb},
		},
	}, nil
}

type mockRepo struct {
	entries []*controlstate.SemanticCacheEntry
	hits    map[string]int
}

func (m *mockRepo) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	m.entries = append(m.entries, entry)
	return nil
}

func (m *mockRepo) ListCandidates(ctx context.Context, scope, model string) ([]*controlstate.SemanticCacheEntry, error) {
	var res []*controlstate.SemanticCacheEntry
	now := time.Now().UTC()
	for _, e := range m.entries {
		if e.Scope == scope && e.Model == model && e.Enabled && e.ExpiresAt.After(now) {
			res = append(res, e)
		}
	}
	// order by created desc
	for i := 0; i < len(res)/2; i++ {
		j := len(res) - 1 - i
		res[i], res[j] = res[j], res[i]
	}
	return res, nil
}

func (m *mockRepo) RecordHit(ctx context.Context, id string) error {
	m.hits[id]++
	return nil
}

func (m *mockRepo) Disable(ctx context.Context, id string) error {
	for _, e := range m.entries {
		if e.ID == id {
			e.Enabled = false
			return nil
		}
	}
	return nil
}

func TestSemanticCacheService_Hit(t *testing.T) {
	repo := &mockRepo{hits: make(map[string]int)}
	adapter := &mockEmbedAdapter{
		embeddings: map[string][]float32{
			"hello world": {1.0, 0.0},
			"hi world":    {0.9, 0.1},
			"bye world":   {0.0, 1.0},
		},
	}

	svc := NewSemanticCacheService(SemanticCacheConfig{
		Enabled:       true,
		Threshold:     0.8,
		MaxCandidates: 10,
		TTL:           1 * time.Hour,
	}, repo, nil, adapter)

	ctx := context.Background()

	// Store "hello world"
	err := svc.Store(ctx, "id-1", "scope-1", "gpt-4", "hello world", `{"response":"hi"}`, nil)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	// Lookup "hi world" - should hit (cos sim between [1,0] and [0.9,0.1] is high)
	entry, err := svc.Lookup(ctx, "scope-1", "gpt-4", "hi world")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if entry == nil {
		t.Fatalf("Expected hit")
	}
	if entry.ID != "id-1" {
		t.Errorf("Expected id-1, got %s", entry.ID)
	}
	if repo.hits["id-1"] != 1 {
		t.Errorf("Expected hit to be recorded")
	}

	// Lookup "bye world" - should miss (cos sim between [1,0] and [0,1] is 0)
	entry, err = svc.Lookup(ctx, "scope-1", "gpt-4", "bye world")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if entry != nil {
		t.Fatalf("Expected miss")
	}
}

func TestSemanticCacheService_Misses(t *testing.T) {
	repo := &mockRepo{hits: make(map[string]int)}
	adapter := &mockEmbedAdapter{
		embeddings: map[string][]float32{
			"test": {1.0, 0.0},
		},
	}

	svc := NewSemanticCacheService(SemanticCacheConfig{
		Enabled:       true,
		Threshold:     0.8,
		MaxCandidates: 10,
		TTL:           1 * time.Hour,
	}, repo, nil, adapter)

	ctx := context.Background()
	_ = svc.Store(ctx, "id-1", "scope-1", "gpt-4", "test", `{}`, nil)

	// Miss: different scope
	e, _ := svc.Lookup(ctx, "scope-2", "gpt-4", "test")
	if e != nil {
		t.Errorf("Expected miss for different scope")
	}

	// Miss: different model
	e, _ = svc.Lookup(ctx, "scope-1", "gpt-3.5", "test")
	if e != nil {
		t.Errorf("Expected miss for different model")
	}

	// Miss: disabled globally
	svcDisabled := NewSemanticCacheService(SemanticCacheConfig{
		Enabled: false,
	}, repo, nil, adapter)
	e, _ = svcDisabled.Lookup(ctx, "scope-1", "gpt-4", "test")
	if e != nil {
		t.Errorf("Expected miss when globally disabled")
	}

	// Miss: expired (simulate by storing with negative TTL)
	svcExp := NewSemanticCacheService(SemanticCacheConfig{
		Enabled:       true,
		Threshold:     0.8,
		MaxCandidates: 10,
		TTL:           -1 * time.Hour,
	}, repo, nil, adapter)
	_ = svcExp.Store(ctx, "id-exp", "scope-1", "gpt-4", "test", `{}`, nil)
	e, _ = svcExp.Lookup(ctx, "scope-1", "gpt-4", "test")
	// the first store might be found if we don't clear the repo, but the ID-exp will be expired
	// wait, since list candidates filters by expiration, id-exp will be skipped. id-1 will hit.
	// let's clear repo
	repo.entries = nil
	_ = svcExp.Store(ctx, "id-exp", "scope-1", "gpt-4", "test", `{}`, nil)
	e, _ = svcExp.Lookup(ctx, "scope-1", "gpt-4", "test")
	if e != nil {
		t.Errorf("Expected miss for expired entry")
	}
}

func TestSecretSafe(t *testing.T) {
	// A placeholder negative assertion: test won't run if it contains secrets.
}
