//go:build phase29preflight

package app

import (
	"sync/atomic"
	"testing"

	"veloxmesh/internal/observability"
)

func TestLivePostProviderRedisDelay(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	t.Setenv("PHASE29_REDIS_ADDR", proxy.listener.Addr().String())
	chain := newLiveChainWithEnvironment(t, env)
	var injected atomic.Bool
	chain.timing.onStage = func(stage observability.StageMeasurement) {
		if stage.Name == "provider_complete" && injected.CompareAndSwap(false, true) {
			proxy.delay.Store(int64(liveInjectedNetworkDelay))
		}
	}
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	response := liveHTTP(t, chain, request)
	id := response.Header.Get("X-Request-ID")
	liveDecodeChat(t, response)
	gap := livePostProviderGap(t, chain.timing, id)
	const minimumRedisDelayMS = 160
	if !injected.Load() || proxy.delay.Load() != 0 || gap < minimumRedisDelayMS {
		t.Fatalf("real Redis delay not observed in post-provider gap: %.3fms", gap)
	}
	chain.app.semanticCache.Close()
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("Redis delay affected eventual cache persistence")
	}
	liveDecodeChat(t, hit)
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "redis_post_provider_delay", "injected_ms": liveInjectedNetworkDelay.Milliseconds(),
		"post_provider_gap_ms": gap, "forwarded": true, "restored_hit": true})
}

func livePostProviderGap(t *testing.T, recorder *liveTiming, id string) float64 {
	t.Helper()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	var provider, response *liveStage
	for _, stage := range recorder.stages {
		if stage.ID != id {
			continue
		}
		copy := stage
		if stage.Name == "provider_complete" {
			provider = &copy
		}
		if stage.Name == "response_rules" {
			response = &copy
		}
	}
	if provider == nil || response == nil {
		t.Fatal("missing post-provider stage boundaries")
	}
	return response.StartMS - provider.StartMS - provider.ElapsedMS
}
