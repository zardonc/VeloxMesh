//go:build phase29preflight

package app

// Failure cases: omitted/disabled mode caches answers; exact mode reuses a
// different question; memo authorizes an answer; changed scope finds old entries.
import (
	"net/http"
	"testing"
	"time"
)

func TestLiveExactCachePolicy(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_REUSE_MODE", "exact")
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, system: liveAcceptanceSystem, question: "What is the refund policy?", origin: time.Now()}
	seed := shipRequest(options)
	if !seed.OK || seed.Hit {
		t.Fatal("exact seed failed or hit an unrelated answer")
	}
	liveWaitStoreCount(t, chain.timing, 1)
	chain.app.semanticCache.Close()
	repeated := shipRequest(options)
	if !repeated.OK || !repeated.Hit || repeated.AnswerHash != seed.AnswerHash {
		t.Fatal("complete identical question did not reuse its exact answer")
	}
	for _, question := range []string{"Is a refund request on the eighth day permitted?", "Are refunds available for fourteen days?", "Is the free trial exactly seven days long?", "What is the refund policy? "} {
		request := options
		request.question = question
		sample := shipRequest(request)
		shipLogJSON(t, sample)
		if !sample.OK || sample.Hit {
			t.Fatal("different question reused an exact answer")
		}
	}
	if calls := livePolicyEmbeddingCalls(chain.timing); calls != 0 {
		t.Fatalf("exact cache invoked embedding: %d calls", calls)
	}
	liveAssertSettlement(t, chain, 5)
	shipLogJSON(t, map[string]any{"type": "cache_policy", "mode": "exact", "same_question_hit": true, "unsafe_hits": 0})
}

func livePolicyEmbeddingCalls(recorder *liveTiming) int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	calls := 0
	for _, stage := range recorder.stages {
		if stage.Name == "embedding_http" {
			calls++
		}
	}
	return calls
}

func TestLiveDisabledCachePolicy(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_REUSE_MODE", "disabled")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: liveWarmupIndex, origin: time.Now()}
	for range 2 {
		sample := shipRequest(options)
		if !sample.OK || sample.Hit {
			t.Fatal("disabled policy failed or reused an answer")
		}
	}
	liveAssertSettlement(t, chain, 2)
}

func TestLiveImplicitCachePolicy(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_REUSE_MODE", "implicit")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, index: liveWarmupIndex, origin: time.Now()}
	for range 2 {
		sample := shipRequest(options)
		if !sample.OK || sample.Hit {
			t.Fatal("omitted policy implicitly authorized answer reuse")
		}
	}
	liveAssertSettlement(t, chain, 2)
}
