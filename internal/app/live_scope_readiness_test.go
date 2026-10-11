//go:build phase29preflight

package app

import (
	"net/http"
	"testing"
	"time"
)

// Fresh scope must miss without a nonexistent vector query, persist a real seed,
// and then use the normal vector path with zero Usage on the hit.
func TestLiveScopeReadinessGateway(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", "on")
	t.Setenv("PHASE29_REUSE_MODE", "semantic")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token, model: chain.model, question: "How long is the trial for plan 101?", origin: time.Now()}
	seed := shipRequest(options)
	if !seed.OK || seed.Hit {
		t.Fatal("fresh seed request failed")
	}
	if liveOutcomeCount(chain.timing, "lookup/scope_not_ready") != 1 || liveOutcomeCount(chain.timing, "lookup/vector_error") != 0 {
		t.Fatal("fresh scope performed a nonexistent collection lookup")
	}
	liveWaitStoreCount(t, chain.timing, 1)
	hit := shipRequest(options)
	if !hit.OK || !hit.Hit || hit.AnswerHash != seed.AnswerHash {
		t.Fatal("activated scope did not reuse the persisted seed")
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "scope_readiness_gateway", "fresh_seed": seed, "activated_hit": hit, "missing_collection_faults": liveOutcomeCount(chain.timing, "lookup/vector_error")})
}
