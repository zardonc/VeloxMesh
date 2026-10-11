//go:build phase29preflight

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/observability"
)

func TestLiveCacheReplay(t *testing.T) {
	chain := newLiveChain(t)
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	liveDecodeChat(t, liveHTTP(t, chain, request))
	liveWaitOutcomes(t, chain.timing, "store/stored")
	write := liveStoredCacheWrite(t, chain)
	write.Vector = liveComponentVector(t)
	for range 2 {
		if err := chain.app.semanticCache.StoreWrite(context.Background(), write); err != nil {
			t.Fatal(err)
		}
	}
	count := liveCachePointCount(t, write)
	if count != 1 {
		t.Fatalf("cache replay created duplicate points: %d", count)
	}
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("replayed entry not readable")
	}
	liveDecodeChat(t, hit)
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "cache_replay", "points": count, "writes": 3, "billing": 1})
}

func TestLiveCacheRestartCleanup(t *testing.T) {
	env := liveEnvironment(t)
	chain := newLiveChainWithEnvironment(t, env)
	var cancelled atomic.Bool
	chain.timing.onStage = func(stage observability.StageMeasurement) {
		if stage.Name == "vector_insert" && cancelled.CompareAndSwap(false, true) {
			chain.app.semanticCache.Close()
		}
	}
	liveDecodeChat(t, liveHTTP(t, chain, liveFAQPayload(chain.model, "How long is the trial for plan 101?")))
	liveWaitOutcomes(t, chain.timing, "store/shutdown_cancelled")
	write := liveStoredCacheWrite(t, chain)
	if liveCachePointCount(t, write) != 1 {
		t.Fatal("no real indexed point before activation cancellation")
	}
	chain.app.Close()
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6334")
	t.Setenv("PHASE29_QDRANT_ADDR", proxy.listener.Addr().String())
	fresh, _ := liveApplicationWithDatabase(t, env, liveDatabase{dsn: chain.dsn, keyID: chain.token})
	recorder := installLiveTiming(t, fresh, time.Now())
	t.Cleanup(func() { recorder.dump(t) })
	proxy.disconnect()
	server := httptest.NewServer(fresh.Router)
	t.Cleanup(server.Close)
	restarted := chain
	restarted.app, restarted.url = fresh, server.URL
	restarted.timing = recorder
	liveWaitCleanupFailure(t, restarted)
	var pending int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries WHERE enabled = 0").Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("cleanup fault lost retry evidence: count=%d error=%v", pending, err)
	}
	proxy.broken.Store(false)
	liveWaitDiscardedRows(t, restarted)
	if liveCachePointCount(t, write) != 0 {
		t.Fatal("restart cleanup left indexed orphan")
	}
	liveAssertSettlement(t, restarted, 1)
	shipLogJSON(t, map[string]any{"type": "cache_restart_cleanup", "activation_cancelled": true, "retained_on_delete_error": true, "rows_after": 0, "points_after": 0, "additional_billing": 0})
}

func TestLiveCacheExpiryCleanup(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_TTL", "500ms")
	chain := newLiveChainWithEnvironment(t, env)
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	liveDecodeChat(t, liveHTTP(t, chain, request))
	liveWaitOutcomes(t, chain.timing, "store/stored")
	write := liveStoredCacheWrite(t, chain)
	time.Sleep(time.Second)
	response := liveHTTP(t, chain, request)
	if response.Header.Get("X-Cache-Hit") == "true" {
		t.Fatal("expired entry hit")
	}
	liveDecodeChat(t, response)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var rows int
		if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries").Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows == 0 {
			break
		}
		time.Sleep(liveInjectedNetworkDelay)
	}
	var remaining int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM semantic_cache_entries").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("expired rows not physically cleaned: rows=%d err=%v", remaining, err)
	}
	if liveCachePointCount(t, write) != 0 {
		t.Fatal("expired vector not cleaned")
	}
	liveAssertSettlement(t, chain, 2)
	shipLogJSON(t, map[string]any{"type": "cache_expiry_cleanup", "expired_hit": false, "rows_after": remaining, "points_after": 0, "billing": 2})
}

func liveStoredCacheWrite(t *testing.T, chain liveChain) cache.CacheWrite {
	var write cache.CacheWrite
	if err := chain.repo.DBForTest().QueryRow("SELECT id, scope, model, response FROM semantic_cache_entries ORDER BY created_at LIMIT 1").Scan(&write.ID, &write.Scope, &write.Model, &write.Response); err != nil {
		t.Fatal(err)
	}
	write.Text = "How long is the trial for plan 101?"
	return write
}

func liveCachePointCount(t *testing.T, write cache.CacheWrite) uint64 {
	client, err := qdrant.NewClient(&qdrant.Config{Host: "127.0.0.1", Port: 6334, APIKey: os.Getenv("QDRANT_API_KEY")})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	digest := sha256.Sum256([]byte(write.Scope + "\x00" + write.Model))
	ctx, cancel := context.WithTimeout(context.Background(), liveComponentDeadline)
	defer cancel()
	count, err := client.Count(ctx, &qdrant.CountPoints{CollectionName: fmt.Sprint("semantic_cache_", hex.EncodeToString(digest[:])), Filter: &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewMatch("id", write.ID)}}, Exact: qdrant.PtrOf(true)})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func liveWaitCleanupFailure(t *testing.T, chain liveChain) {
	deadline := time.Now().Add(30 * time.Second)
	for liveOutcomeCount(chain.timing, "cleanup/vector_error") == 0 && time.Now().Before(deadline) {
		time.Sleep(liveInjectedNetworkDelay)
	}
	if liveOutcomeCount(chain.timing, "cleanup/vector_error") == 0 {
		t.Fatal("real cleanup network fault not observed")
	}
}
