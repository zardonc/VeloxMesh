//go:build phase29preflight

package app

import (
	"context"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/storage"
)

const liveInjectedNetworkDelay = 200 * time.Millisecond
const liveRecoveryWait = 3 * time.Second

func TestLiveEmbeddingNetworkDeadline(t *testing.T) {
	liveNetworkDeadline(t, "embedding")
}

func TestLiveSearchNetworkDeadline(t *testing.T) {
	liveNetworkDeadline(t, "search")
}

func liveNetworkDeadline(t *testing.T, kind string) {
	env := liveEnvironment(t)
	var proxy *liveNetworkProxy
	if kind == "embedding" {
		upstream, err := url.Parse(os.Getenv("PHASE29_EMBEDDING_BASE_URL"))
		if err != nil || upstream.Host == "" {
			t.Fatal("explicit real embedding endpoint required")
		}
		proxy = newLiveNetworkProxy(t, upstream.Host)
		t.Setenv("PHASE29_EMBEDDING_BASE_URL", "http://"+proxy.listener.Addr().String()+upstream.Path)
	} else {
		proxy = newLiveNetworkProxy(t, "127.0.0.1:6334")
		t.Setenv("PHASE29_QDRANT_ADDR", proxy.listener.Addr().String())
	}
	chain := newLiveChainWithEnvironment(t, env)
	proxy.delay.Store(int64(liveInjectedNetworkDelay))
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	response := liveHTTP(t, chain, request)
	id := response.Header.Get("X-Request-ID")
	if response.Header.Get("X-Cache-Hit") == "true" {
		t.Fatal("network fault incorrectly hit")
	}
	first := liveDecodeChat(t, response)
	liveWaitOutcomes(t, chain.timing, "store/stored")
	counts := liveStageCounts(chain.timing, id)
	expectedStoreEmbeddings := 0
	if kind == "embedding" {
		expectedStoreEmbeddings = 1
	}
	liveAssertDeadlineStages(t, counts, expectedStoreEmbeddings)
	if liveOutcomeCount(chain.timing, "lookup/timeout") != 1 {
		t.Fatal("read deadline did not report one timeout")
	}
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("network restored but persisted response did not hit")
	}
	second := liveDecodeChat(t, hit)
	if first.Choices[0].Message.Content != second.Choices[0].Message.Content {
		t.Fatal("recovery changed cached answer")
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "network_fault", "kind": kind, "counts": counts, "delay_ms": liveInjectedNetworkDelay.Milliseconds(), "restored_hit": true})
}

func liveAssertDeadlineStages(t *testing.T, counts map[string]int, expected int) {
	if counts["provider_complete"] != 1 || counts["store_embedding"] != expected {
		t.Fatalf("real deadline stages=%v expected_store_embedding=%d", counts, expected)
	}
}

func liveFAQPayload(model, question string) llm.ChatCompletionRequest {
	temperature, limit := 0.0, 256
	return llm.ChatCompletionRequest{Model: model, Temperature: &temperature, MaxTokens: &limit,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveFAQSystem}, {Role: llm.RoleUser, Content: question}}}
}

func liveStageCounts(recorder *liveTiming, id string) map[string]int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	counts := map[string]int{}
	for _, stage := range recorder.stages {
		if stage.ID == id {
			counts[stage.Name]++
		}
	}
	return counts
}

func liveOutcomeCount(recorder *liveTiming, name string) int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	count := 0
	for _, sample := range recorder.samples {
		if sample.Type == "outcome" && sample.Name == name {
			count++
		}
	}
	return count
}

func liveWaitOutcomes(t *testing.T, recorder *liveTiming, name string) {
	t.Helper()
	deadline := time.Now().Add(liveRecoveryWait)
	for liveOutcomeCount(recorder, name) == 0 && time.Now().Before(deadline) {
		time.Sleep(livePollInterval)
	}
	if liveOutcomeCount(recorder, name) == 0 {
		t.Fatalf("missing real cache outcome %s", name)
	}
}

func TestLiveVectorPartialWriteRecovery(t *testing.T) {
	env := liveEnvironment(t)
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6334")
	t.Setenv("PHASE29_QDRANT_ADDR", proxy.listener.Addr().String())
	chain := newLiveChainWithEnvironment(t, env)
	var disconnected atomic.Bool
	chain.timing.onStage = func(stage observability.StageMeasurement) {
		if stage.Name == "repo_write" && disconnected.CompareAndSwap(false, true) {
			proxy.disconnect()
		}
	}
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	liveDecodeChat(t, liveHTTP(t, chain, request))
	liveWaitOutcomes(t, chain.timing, "store/vector_error")
	var before int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries").Scan(&before); err != nil || before != 1 {
		t.Fatalf("partial persistence count=%d error=%v", before, err)
	}
	proxy.broken.Store(false)
	adapter, err := storage.NewQdrantVectorAdapter(proxy.listener.Addr().String(), os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveRecoveryWait)
	defer cancel()
	if err := adapter.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	miss := liveHTTP(t, chain, request)
	if miss.Header.Get("X-Cache-Hit") == "true" {
		t.Fatal("unindexed partial entry produced a false hit")
	}
	liveDecodeChat(t, miss)
	upstreamRequests := 2 + liveRetryRecoveredWrite(t, chain, request)
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("write did not recover")
	}
	liveDecodeChat(t, hit)
	liveAssertSettlement(t, chain, upstreamRequests)
	var after int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries").Scan(&after); err != nil {
		t.Fatal(err)
	}
	shipLogJSON(t, map[string]any{"type": "partial_write", "unindexed_rows_before": before, "rows_after": after,
		"false_hit": false, "recovered": true, "upstream_requests": upstreamRequests})
}

func liveRetryRecoveredWrite(t *testing.T, chain liveChain, request llm.ChatCompletionRequest) int {
	deadline, started, retries := time.Now().Add(liveRecoveryWait), time.Now(), 0
	for liveOutcomeCount(chain.timing, "store/stored") == 0 && time.Now().Before(deadline) {
		time.Sleep(liveInjectedNetworkDelay)
		if liveOutcomeCount(chain.timing, "store/stored") > 0 {
			break
		}
		liveDecodeChat(t, liveHTTP(t, chain, request))
		retries++
	}
	if liveOutcomeCount(chain.timing, "store/stored") == 0 {
		t.Fatal("real dependency did not recover within bounded window")
	}
	shipLogJSON(t, map[string]any{"type": "network_recovery", "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000, "additional_requests": retries})
	return retries
}
