//go:build phase29preflight

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
)

// Failure cases: a delayed ordered write blocks a newer request; bypassing a
// legacy client's serialization loses ordering; either path loses counters.
const publicationSyncTimeout = 500 * time.Millisecond
const publicationResponseDelay = 200 * time.Millisecond
const publicationFastLimit = 40 * time.Millisecond

// Hide only the optional ordered capability; every operation still uses Redis.
type legacyPublicationClient struct{ hotstate.Client }

func TestLiveRedisSameProviderPublication(t *testing.T) {
	liveEnvironment(t)
	for _, ordered := range []bool{true, false} {
		t.Run(fmt.Sprintf("ordered=%t", ordered), func(t *testing.T) {
			liveSameProviderPublication(t, ordered)
		})
	}
}

func liveSameProviderPublication(t *testing.T, ordered bool) {
	t.Helper()
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0,
		fmt.Sprint("health-publication-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var writer hotstate.Client = client
	if !ordered {
		writer = legacyPublicationClient{Client: client}
	}
	store := health.NewRedisStoreWithOptions(writer, health.RedisStoreOptions{TTL: "1m", SyncTimeout: publicationSyncTimeout})
	store.EnsureProvider("same", 3, 1)
	proxy.delay.Store(int64(publicationResponseDelay))
	done := make(chan struct{})
	go func() { store.BeginRequest("same"); close(done) }()
	waitPublicationDelay(t, proxy)
	started := time.Now()
	store.BeginRequest("same")
	elapsed := time.Since(started)
	<-done
	data, err := client.GetHealthSnapshot(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	var remote health.RedisProviderState
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	shipLogJSON(t, map[string]any{"type": "same_provider_publication", "ordered": ordered,
		"second_publication_ms": float64(elapsed.Microseconds()) / 1000, "remote_pending": remote.PendingRequests,
		"local_pending": store.Snapshots()["same"].PendingRequests})
	if remote.PendingRequests != 2 || store.Snapshots()["same"].PendingRequests != 2 {
		t.Fatal("concurrent publication lost pending increments")
	}
	if ordered && elapsed >= publicationFastLimit {
		t.Fatalf("atomic ordered publication waited behind unrelated Redis response: %s", elapsed)
	}
	if !ordered && elapsed < publicationFastLimit {
		t.Fatalf("legacy publication bypassed required serialization: %s", elapsed)
	}
}

func waitPublicationDelay(t *testing.T, proxy *liveNetworkProxy) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for proxy.delay.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if proxy.delay.Load() != 0 {
		t.Fatal("real Redis response delay was not consumed")
	}
}
