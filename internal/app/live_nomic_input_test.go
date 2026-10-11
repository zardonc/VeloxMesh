//go:build phase29preflight

package app

import (
	"os"
	"testing"

	"veloxmesh/internal/providers/openai"
)

// Compare candidate representations only. This does not change cache policy,
// production thresholds, or the user's inference-service configuration.
func TestLiveNomicPrefixComparison(t *testing.T) {
	env := liveEnvironment(t)
	model := env["PHASE29_SECOND_EMBEDDING_MODEL"]
	if model == "" {
		t.Fatal("explicit real second embedding model required")
	}
	t.Setenv("PHASE29_MODEL", model)
	adapter := openai.NewAdapter("input-diagnostic", os.Getenv("PHASE29_EMBEDDING_BASE_URL"), os.Getenv("PHASE29_EMBEDDING_API_KEY"), model)
	for _, prefix := range []string{"", "clustering: ", "search_query: "} {
		seed := liveScoreVector(t, adapter, prefix+"How long is the free trial?")
		for _, positive := range []bool{true, false} {
			for index, question := range liveSemanticQuestions(positive) {
				vector := liveScoreVector(t, adapter, prefix+question)
				if len(vector) != len(seed) {
					t.Fatal("candidate representation changed dimensions")
				}
				shipLogJSON(t, map[string]any{"type": "input_score", "prefix": prefix, "positive": positive,
					"index": index, "cosine": liveVectorCosine(seed, vector), "dimension": len(vector), "embedding_model": model})
			}
		}
	}
}
