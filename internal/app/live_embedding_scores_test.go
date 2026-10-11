//go:build phase29preflight

package app

import (
	"context"
	"math"
	"os"
	"testing"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/openai"
)

func TestLiveEmbeddingSemanticScores(t *testing.T) {
	liveEnvironment(t)
	adapter := openai.NewAdapter("semantic-score", os.Getenv("PHASE29_EMBEDDING_BASE_URL"), os.Getenv("PHASE29_EMBEDDING_API_KEY"), os.Getenv("PHASE29_MODEL"))
	seed := liveScoreVector(t, adapter, "How long is the free trial?")
	for _, positive := range []bool{true, false} {
		for index, question := range liveSemanticQuestions(positive) {
			vector := liveScoreVector(t, adapter, question)
			if len(vector) != len(seed) {
				t.Fatal("real semantic vector dimension changed")
			}
			shipLogJSON(t, map[string]any{"type": "embedding_semantic_score", "positive": positive, "index": index,
				"cosine": liveVectorCosine(seed, vector), "dimension": len(vector), "embedding_model": os.Getenv("PHASE29_MODEL")})
		}
	}
}

func liveScoreVector(t *testing.T, adapter *openai.Adapter, question string) []float32 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	response, err := adapter.Embed(ctx, &llm.EmbeddingRequest{Model: os.Getenv("PHASE29_MODEL"), Input: []string{question}})
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || len(response.Data) != 1 {
		t.Fatal("invalid real embedding")
	}
	return response.Data[0].Embedding
}

func liveVectorCosine(first, second []float32) float64 {
	var dot, firstNorm, secondNorm float64
	for index, value := range first {
		a, b := float64(value), float64(second[index])
		dot, firstNorm, secondNorm = dot+a*b, firstNorm+a*a, secondNorm+b*b
	}
	return dot / math.Sqrt(firstNorm*secondNorm)
}
