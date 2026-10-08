package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
)

func TestSemanticCacheLabeledFAQAndIsolation(t *testing.T) {
	// Human-labeled static FAQ; vectors are an offline oracle, not real-model evidence.
	adapter := &mockEmbedAdapter{embeddings: map[string][]float32{
		"What are support hours?": {1, 0}, "When is support open?": {.99, .01},
		"What are maintenance hours?": {0, 1}, "Are support hours different on holidays?": {.3, .95},
	}}
	repo := &mockRepo{hits: make(map[string]int)}
	cfg := boundsConfig()
	svc := NewSemanticCacheService(cfg, repo, nil, adapter)
	t.Cleanup(svc.Close)
	ctx := context.Background()
	if err := svc.Store(ctx, "entry", "key1-faq-v1", "model1", "What are support hours?", `[{"message":{"role":"assistant","content":"09:00-17:00 UTC weekdays"}}]`, nil); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		question, scope, model string
		hit                    bool
	}{
		{"When is support open?", "key1-faq-v1", "model1", true},
		{"What are maintenance hours?", "key1-faq-v1", "model1", false},
		{"Are support hours different on holidays?", "key1-faq-v1", "model1", false},
		{"What are support hours?", "key2-faq-v1", "model1", false},
		{"What are support hours?", "key1-faq-v2", "model1", false},
		{"What are support hours?", "key1-faq-v1", "model2", false},
	} {
		entry, err := svc.Lookup(ctx, sample.scope, sample.model, sample.question)
		if err != nil || (entry != nil) != sample.hit {
			t.Fatalf("labeled FAQ hit=%v expected=%v err=%v", entry != nil, sample.hit, err)
		}
	}
	repo.entries[0].ExpiresAt = time.Now().Add(-time.Second)
	if entry, err := svc.Lookup(ctx, "key1-faq-v1", "model1", "What are support hours?"); entry != nil || err != nil {
		t.Fatal("expired answer matched")
	}
}

type errorRepository struct{ mockRepo }

func (r *errorRepository) ListCandidates(context.Context, string, string) ([]*controlstate.SemanticCacheEntry, error) {
	return nil, errors.New("repository unavailable")
}
func (r *errorRepository) Store(context.Context, *controlstate.SemanticCacheEntry) error {
	return errors.New("repository unavailable")
}

func TestSemanticCacheRepositoryAndEmbeddingFailures(t *testing.T) {
	for _, adapter := range []*boundedEmbed{
		{err: errors.New("upstream unavailable")},
		{},
		{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}},
	} {
		svc := NewSemanticCacheService(boundsConfig(), &errorRepository{}, nil, adapter)
		if entry, err := svc.Lookup(context.Background(), "scope", "model", "text"); entry != nil || err == nil {
			t.Fatal("dependency failure must report bypass")
		}
		if err := svc.Store(context.Background(), "id", "scope", "model", "text", validTestAnswer, nil); err == nil {
			t.Fatal("dependency failure must report write drop")
		}
		svc.Close()
	}
}
