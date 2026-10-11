package cache

import (
	"context"
	"math"
	"testing"

	"veloxmesh/internal/llm"
)

func TestSemanticCacheMalformedVectorResultsDoNotScan(t *testing.T) {
	for name, result := range map[string]map[string]interface{}{
		"missing score":    {"id": "entry"},
		"nonfinite score":  {"id": "entry", "score": math.NaN()},
		"missing identity": {"score": float64(1)},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &mockRepo{}
			adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}}
			svc := NewSemanticCacheService(boundsConfig(), repo, &mockVectorAdapter{results: []map[string]interface{}{result}}, adapter)
			t.Cleanup(svc.Close)
			if entry, err := svc.Lookup(context.Background(), "scope", "model", "text"); entry != nil || err == nil {
				t.Fatal("malformed vector result must report bypass")
			}
			if repo.listCalls != 0 {
				t.Fatal("malformed result triggered repository scan")
			}
		})
	}
}
