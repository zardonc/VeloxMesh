//go:build phase29preflight

package app

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"veloxmesh/internal/cache"
)

func TestLiveEmbeddingMemoBoundaries(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MEMO_CAPACITY", "2")
	t.Setenv("PHASE29_MEMO_TTL", "2s")
	chain := newLiveChainWithEnvironment(t, env)
	query := cache.CacheLookup{Scope: chain.token, Model: chain.model, Text: "How long is the trial for plan 101?"}
	otherScope := cache.CacheLookup{Scope: query.Scope + "-other-account", Model: query.Model, Text: query.Text}
	otherModel := cache.CacheLookup{Scope: query.Scope, Model: query.Model + "-other-model", Text: query.Text}
	liveMemoReadyScopes(t, chain, []cache.CacheLookup{query, otherScope, otherModel})
	first := liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-first", calls: 1})
	repeat := liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-repeat", calls: 0})
	if !slices.Equal(first.Vector, repeat.Vector) {
		t.Fatal("memo changed the real embedding")
	}
	// Intentionally mutate an outward-facing result to test ownership isolation.
	repeat.Vector[0]++
	third := liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-copy", calls: 0})
	if !slices.Equal(first.Vector, third.Vector) {
		t.Fatal("caller modified the cached vector")
	}
	liveMemoLookup(t, chain, liveMemoCheck{query: otherScope, id: "memo-scope", calls: 1})
	liveMemoLookup(t, chain, liveMemoCheck{query: otherModel, id: "memo-model", calls: 1})
	liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-evicted", calls: 1})
	time.Sleep(2100 * time.Millisecond)
	liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-expired", calls: 1})
	ctx, cancel := context.WithCancel(timingContext("memo-cancel"))
	cancel()
	if _, err := chain.app.semanticCache.LookupWithVector(ctx, query); err == nil {
		t.Fatal("cancelled lookup returned a memo vector without error")
	}
	if liveUsageCount(t, chain) != 0 {
		t.Fatal("embedding memo verification called the main model")
	}
	shipLogJSON(t, map[string]any{"type": "memo_boundaries", "copy": true, "scope_isolation": true,
		"model_isolation": true, "ttl": true, "capacity": true, "cancel": true, "main_model_calls": 0})
}

type liveMemoCheck struct {
	query cache.CacheLookup
	id    string
	calls int
}

// Persist real vectors without warming the memo or calling the chat model.
// Empty scopes intentionally bypass embedding after readiness hardening.
func liveMemoReadyScopes(t *testing.T, chain liveChain, queries []cache.CacheLookup) {
	t.Helper()
	vector := liveComponentVector(t)
	for _, query := range queries {
		write := cache.CacheWrite{ID: uuid.NewString(), Scope: query.Scope, Model: query.Model,
			Text: "How long is the trial for plan 101?", Vector: vector,
			Response: `[{"index":0,"message":{"role":"assistant","content":"Memo fixture candidate."},"finish_reason":"stop"}]`}
		if err := chain.app.semanticCache.StoreWrite(context.Background(), write); err != nil {
			t.Fatalf("prepare real memo candidate: %v", err)
		}
	}
}

func liveMemoLookup(t *testing.T, chain liveChain, check liveMemoCheck) cache.CacheLookupResult {
	t.Helper()
	result, err := chain.app.semanticCache.LookupWithVector(timingContext(check.id), check.query)
	if len(result.Vector) == 0 {
		t.Fatalf("real embedding unavailable: %v", err)
	}
	if calls := liveEmbeddingHTTPCount(chain.timing, check.id); calls != check.calls {
		t.Fatalf("%s: embedding calls=%d expected=%d", check.id, calls, check.calls)
	}
	return result
}

func liveEmbeddingHTTPCount(recorder *liveTiming, id string) int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	count := 0
	for _, stage := range recorder.stages {
		if stage.ID == id && stage.Name == "embedding_http" {
			count++
		}
	}
	return count
}

func TestLiveEmbeddingMemoFailureRecovery(t *testing.T) {
	env := liveEnvironment(t)
	proxy := liveDeadlineProxy(t, "embedding")
	t.Setenv("PHASE29_MEMO_CAPACITY", "2")
	t.Setenv("PHASE29_MEMO_TTL", "2s")
	chain := newLiveChainWithEnvironment(t, env)
	query := cache.CacheLookup{Scope: chain.token, Model: chain.model, Text: "What is the duration of plan 111?"}
	liveMemoReadyScopes(t, chain, []cache.CacheLookup{query})
	liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-before-disconnect", calls: 1})
	proxy.disconnect()
	liveMemoLookup(t, chain, liveMemoCheck{query: query, id: "memo-during-disconnect", calls: 0})
	newQuery := cache.CacheLookup{Scope: query.Scope, Model: query.Model, Text: "What is the duration of plan 112?"}
	if _, err := chain.app.semanticCache.LookupWithVector(timingContext("memo-fault"), newQuery); err == nil {
		t.Fatal("disconnected real embedding unexpectedly succeeded")
	}
	proxy.broken.Store(false)
	liveMemoLookup(t, chain, liveMemoCheck{query: newQuery, id: "memo-recovered", calls: 1})
	shipLogJSON(t, map[string]any{"type": "memo_fault_recovery", "error_visible": true, "failed_vector_not_cached": true})
}

func TestLiveEmbeddingMemoHitBilling(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MEMO_CAPACITY", "16")
	t.Setenv("PHASE29_MEMO_TTL", "1m")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token,
		model: chain.model, question: "How long is the trial for plan 101?", origin: time.Now()}
	seed := shipRequest(options)
	if !seed.OK || seed.Hit {
		t.Fatal("memo real seed failed")
	}
	chain.app.semanticCache.Close()
	options.origin = time.Now()
	samples := liveBurstRequests(t, options)
	liveScenarioResults(t, samples, "hit")
	for _, sample := range samples {
		if sample.AnswerHash != seed.AnswerHash || liveEmbeddingHTTPCount(chain.timing, sample.RequestID) != 0 {
			t.Fatal("memo hit changed answer or called embedding again")
		}
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "memo_hit_billing", "requests": len(samples), "embedding_calls": 1, "billed_requests": 1})
}

func TestLiveCacheLoadOnMemo(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MEMO_CAPACITY", "128")
	t.Setenv("PHASE29_MEMO_TTL", "1m")
	shipLoadEnvironment(t, "on", env)
}

func TestLiveCacheLoadOnWithoutMemo(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MEMO_CAPACITY", "0")
	t.Setenv("PHASE29_MEMO_TTL", "")
	shipLoadEnvironment(t, "on", env)
}
