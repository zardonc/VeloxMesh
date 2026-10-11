//go:build phase29preflight

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
)

// Failure cases: default deadlines disappear, replication errors are hidden,
// a slow reply blocks newer calls, or late model/probe writes replace new state.
const publicationDeadlineCeiling = 100 * time.Millisecond

func publicationFixture(t *testing.T) (*liveNetworkProxy, *hotstate.RedisClient) {
	t.Helper()
	liveEnvironment(t)
	proxy := newLiveNetworkProxy(t, "127.0.0.1:6379")
	client, err := hotstate.NewRedisClient(context.Background(), proxy.listener.Addr().String(), "", 0,
		fmt.Sprint("health-extended-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return proxy, client
}

func TestLiveRedisPublicationDefaultDeadline(t *testing.T) {
	proxy, client := publicationFixture(t)
	var logs bytes.Buffer
	store := health.NewRedisStoreWithOptions(client, health.RedisStoreOptions{
		TTL: "1m", Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	store.EnsureProvider("same", 3, 1)
	proxy.delay.Store(int64(publicationResponseDelay))
	done := make(chan time.Duration, 1)
	go func() {
		started := time.Now()
		store.BeginRequest("same")
		done <- time.Since(started)
	}()
	waitPublicationDelay(t, proxy)
	started := time.Now()
	store.BeginRequest("same")
	second, first := time.Since(started), <-done
	if first >= publicationDeadlineCeiling || second >= publicationFastLimit {
		t.Fatalf("default deadline or concurrent publication regressed: first=%s second=%s", first, second)
	}
	if !strings.Contains(logs.String(), "Redis health replication failed") {
		t.Fatal("timed-out publication was not reported")
	}
	store.EndRequest("same", time.Millisecond, nil)
	store.EndRequest("same", time.Millisecond, nil)
	data, err := client.GetHealthSnapshot(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	var remote health.RedisProviderState
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	local := store.Snapshots()["same"]
	if remote.PendingRequests != 0 || remote.TotalSuccesses != 2 || local.TotalSuccesses != 2 {
		t.Fatalf("recovery lost counters: remote=%+v local=%+v", remote, local)
	}
	shipLogJSON(t, map[string]any{"type": "publication_default_deadline", "first_ms": float64(first.Microseconds()) / 1000,
		"second_ms": float64(second.Microseconds()) / 1000, "remote_successes": remote.TotalSuccesses, "errors": logs.String()})
}

func TestLiveRedisModelLatePublication(t *testing.T) {
	proxy, client := publicationFixture(t)
	store := health.NewRedisStore(client, "1m")
	store.RecordModelOutcome("same", "model", true)
	proxy.requestDelay.Store(int64(publicationResponseDelay))
	store.RecordModelOutcome("same", "model", false)
	if proxy.requestDelay.Load() != 0 {
		t.Fatal("late model command was not injected")
	}
	store.RecordModelOutcome("same", "model", true)
	time.Sleep(publicationResponseDelay * 2)
	data, err := client.GetBytes(context.Background(), "model_snapshot:same:model")
	if err != nil {
		t.Fatal(err)
	}
	var remote health.RedisModelState
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	if remote.TotalSuccesses != 2 || remote.TotalFailures != 1 {
		t.Fatalf("late model snapshot replaced newer counters: %+v", remote)
	}
	shipLogJSON(t, map[string]any{"type": "model_late_publication", "successes": remote.TotalSuccesses, "failures": remote.TotalFailures})
}

func TestLiveRedisProbeConcurrentPublication(t *testing.T) {
	proxy, client := publicationFixture(t)
	store := health.NewRedisStore(client, "1m")
	store.EnsureProvider("same", 3, 1)
	store.RecordProbe("same", true, time.Millisecond, "")
	proxy.requestDelay.Store(int64(publicationResponseDelay))
	done := make(chan struct{})
	go func() { store.RecordProbe("same", false, time.Millisecond, "injected"); close(done) }()
	deadline := time.Now().Add(time.Second)
	for proxy.requestDelay.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if proxy.requestDelay.Load() != 0 {
		t.Fatal("late probe command was not injected")
	}
	store.RecordProbe("same", true, time.Millisecond, "")
	<-done
	time.Sleep(publicationResponseDelay * 2)
	data, err := client.GetProbeSnapshot(context.Background(), "same")
	if err != nil {
		t.Fatal(err)
	}
	var remote struct {
		Success bool      `json:"success"`
		Time    time.Time `json:"time"`
	}
	if err := json.Unmarshal(data, &remote); err != nil {
		t.Fatal(err)
	}
	local := store.Snapshots()["same"]
	if !remote.Success || !remote.Time.Equal(local.LastProbeAt) || local.TotalFailures != 1 || local.TotalSuccesses != 2 {
		t.Fatalf("probe ordering or counts changed: remote=%+v local=%+v", remote, local)
	}
	shipLogJSON(t, map[string]any{"type": "probe_concurrent_publication", "latest_success": remote.Success, "successes": local.TotalSuccesses, "failures": local.TotalFailures})
}
