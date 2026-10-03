//go:build phase29preflight

package app

import (
	"os"
	"strconv"
	"testing"

	"veloxmesh/internal/providers/openai"
)

func TestLiveEmbeddingDimensionMismatch(t *testing.T) {
	env := liveEnvironment(t)
	adapter := openai.NewAdapter("dimension-check", os.Getenv("PHASE29_EMBEDDING_BASE_URL"), os.Getenv("PHASE29_EMBEDDING_API_KEY"), os.Getenv("PHASE29_MODEL"))
	probe := liveEmbeddingRequest(adapter, liveWarmupIndex)
	if !probe.OK || probe.Dimension < 2 {
		t.Fatalf("invalid real dimension probe: %+v", probe)
	}
	t.Setenv("PHASE29_VECTOR_DIMENSION", strconv.Itoa(probe.Dimension-1))
	chain := newLiveChainWithEnvironment(t, env)
	response := liveHTTP(t, chain, liveFAQPayload(chain.model, "How long is the trial for plan 101?"))
	if response.Header.Get("X-Cache-Hit") == "true" {
		t.Fatal("incompatible real embedding produced a cache hit")
	}
	liveDecodeChat(t, response)
	chain.app.semanticCache.Close()
	var rows int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || liveOutcomeCount(chain.timing, "lookup/invalid_embedding") != 1 || liveOutcomeCount(chain.timing, "store/invalid_embedding") != 1 {
		t.Fatalf("invalid real dimension not rejected: rows=%d", rows)
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "real_dimension_mismatch", "actual_dimension": probe.Dimension,
		"configured_dimension": probe.Dimension - 1, "persisted_rows": rows, "forwarded": true})
}
