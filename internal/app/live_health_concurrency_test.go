//go:build phase29preflight

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
)

func TestLiveRedisProviderIsolation(t *testing.T) {
	liveEnvironment(t)
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0, fmt.Sprint("health-isolation-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := health.NewRedisStore(client, "1m")
	store.EnsureProvider("slow", 3, 1)
	store.EnsureProvider("fast", 3, 1)
	proxy.delay.Store(int64(liveInjectedNetworkDelay))
	done := make(chan struct{})
	go func() { store.BeginRequest("slow"); close(done) }()
	deadline := time.Now().Add(time.Second)
	for proxy.delay.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if proxy.delay.Load() != 0 {
		t.Fatal("real response delay not consumed")
	}
	started := time.Now()
	store.BeginRequest("fast")
	elapsed := time.Since(started)
	<-done
	shipLogJSON(t, map[string]any{"type": "redis_provider_isolation", "unrelated_provider_ms": float64(elapsed.Microseconds()) / 1000})
	if elapsed >= 100*time.Millisecond {
		t.Fatalf("one Redis delay blocked unrelated provider: %s", elapsed)
	}
	if store.Snapshots()["slow"].PendingRequests != 1 || store.Snapshots()["fast"].PendingRequests != 1 {
		t.Fatal("local counters lost during replication fault")
	}
}

func TestLiveRedisConcurrentSnapshots(t *testing.T) {
	liveEnvironment(t)
	client, err := hotstate.NewRedisClient(context.Background(), liveRedisAddress(), "", 0, fmt.Sprint("health-order-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := health.NewRedisStore(client, "1m")
	store.EnsureProvider("same", 3, 1)
	const concurrency, perWorker = 8, 25
	var workers sync.WaitGroup
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range perWorker {
				store.BeginRequest("same")
				store.EndRequest("same", time.Millisecond, nil)
				store.RecordModelOutcome("same", "real-health-test", true)
			}
		}()
	}
	workers.Wait()
	data, err := client.GetHealthSnapshot(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	var remote health.RedisProviderState
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	model := store.ModelSnapshot("same", "real-health-test")
	local := store.Snapshots()["same"]
	if remote.PendingRequests != 0 || remote.TotalSuccesses != concurrency*perWorker || local.TotalSuccesses != remote.TotalSuccesses || model.TotalSuccesses != remote.TotalSuccesses {
		t.Fatalf("health snapshots lost/reordered counters: remote=%+v local=%+v model=%+v", remote, local, model)
	}
	shipLogJSON(t, map[string]any{"type": "redis_snapshot_order", "concurrency": concurrency, "successful": remote.TotalSuccesses, "pending": remote.PendingRequests, "model_successful": model.TotalSuccesses})
}

func TestLiveRedisStalePublication(t *testing.T) {
	liveEnvironment(t)
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0, fmt.Sprint("health-stale-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := health.NewRedisStore(client, "1m")
	store.EnsureProvider("same", 3, 1)
	proxy.requestDelay.Store(int64(liveInjectedNetworkDelay))
	done := make(chan struct{})
	go func() { store.BeginRequest("same"); close(done) }()
	<-done
	store.EndRequest("same", time.Millisecond, nil)
	time.Sleep(liveInjectedNetworkDelay * 2)
	data, err := client.GetHealthSnapshot(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	var remote health.RedisProviderState
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	shipLogJSON(t, map[string]any{"type": "redis_stale_publication", "remote_pending": remote.PendingRequests, "remote_successes": remote.TotalSuccesses})
	if remote.PendingRequests != 0 || remote.TotalSuccesses != 1 {
		t.Fatalf("timed-out old command overwrote newer snapshot: pending=%d successes=%d", remote.PendingRequests, remote.TotalSuccesses)
	}
}
