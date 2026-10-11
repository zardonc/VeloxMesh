//go:build phase29preflight

package app

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/observability"
)

func TestLiveSQLiteConnectionPragmas(t *testing.T) {
	liveEnvironment(t)
	repo, _, _ := liveRepository(t)
	const connectionCount = 8
	connections := make([]*sql.Conn, 0, connectionCount)
	for range connectionCount {
		connection, err := repo.DBForTest().Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
		t.Cleanup(func() { _ = connection.Close() })
	}
	for index, connection := range connections {
		var foreignKeys, busyTimeout, synchronous int
		for query, target := range map[string]*int{"PRAGMA foreign_keys": &foreignKeys, "PRAGMA busy_timeout": &busyTimeout, "PRAGMA synchronous": &synchronous} {
			if err := connection.QueryRowContext(context.Background(), query).Scan(target); err != nil {
				t.Fatal(err)
			}
		}
		shipLogJSON(t, map[string]any{"type": "sqlite_connection_pragmas", "connection": index, "foreign_keys": foreignKeys, "busy_timeout_ms": busyTimeout, "synchronous": synchronous})
		if foreignKeys != 1 || busyTimeout != 5000 || synchronous != 1 {
			t.Errorf("unconfigured pooled connection %d: foreign_keys=%d busy_timeout=%d synchronous=%d", index, foreignKeys, busyTimeout, synchronous)
		}
	}
}

func TestLiveCachePendingVisibility(t *testing.T) {
	chain := newLiveChain(t)
	var visibility atomic.Int64
	visibility.Store(-1)
	chain.timing.onStage = func(stage observability.StageMeasurement) {
		if stage.Name != "repo_write" {
			return
		}
		var enabled int
		// This fixture writes one seed; trace IDs are not storage primary keys.
		err := chain.repo.DBForTest().QueryRow("SELECT CASE WHEN COUNT(*) = 1 THEN MIN(enabled) ELSE -1 END FROM semantic_cache_entries").Scan(&enabled)
		if err == nil {
			visibility.Store(int64(enabled))
		}
	}
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	liveDecodeChat(t, liveHTTP(t, chain, request))
	liveWaitOutcomes(t, chain.timing, "store/stored")
	if visibility.Load() != 0 {
		t.Fatalf("entry became readable before vector commit: enabled=%d", visibility.Load())
	}
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("confirmed vector write did not activate entry")
	}
	liveDecodeChat(t, hit)
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "pending_visibility", "pending_enabled": visibility.Load(), "activated_hit": true})
}

func TestLiveRedisDelayBound(t *testing.T) {
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
	liveDecodeChat(t, liveHTTP(t, chain, liveFAQPayload(chain.model, "How long is the trial for plan 101?")))
	chain.timing.mu.Lock()
	stages := append([]liveStage(nil), chain.timing.stages...)
	chain.timing.mu.Unlock()
	var id string
	for _, stage := range stages {
		if stage.Name == "provider_complete" {
			id = stage.ID
		}
	}
	gap := livePostProviderGap(t, chain.timing, id)
	const maxBoundedGapMS = 150
	shipLogJSON(t, map[string]any{"type": "redis_delay_bound", "injected_ms": liveInjectedNetworkDelay.Milliseconds(), "post_provider_gap_ms": gap})
	if !injected.Load() || gap >= maxBoundedGapMS {
		t.Fatalf("Redis health wait exceeded bound: %.3fms", gap)
	}
	liveAssertSettlement(t, chain, 1)
	chain.app.semanticCache.Close()
	time.Sleep(livePollInterval)
}
