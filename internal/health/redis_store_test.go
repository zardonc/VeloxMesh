package health_test

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
)

func TestRedisStore(t *testing.T) {
	cache := hotstate.NewLocalHotState()
	store := health.NewRedisStore(cache, "1m")

	store.EnsureProvider("p1", 3, 1)

	state := store.Snapshot("p1")

	if state.Status != health.StatusHealthy {
		t.Errorf("expected healthy status")
	}

	store.BeginRequest("p1")
	state = store.Snapshot("p1")
	if state.PendingRequests != 1 {
		t.Errorf("expected 1 active request, got %d", state.PendingRequests)
	}

	store.EndRequest("p1", 0, nil)
	state = store.Snapshot("p1")
	if state.PendingRequests != 0 {
		t.Errorf("expected 0 active requests, got %d", state.PendingRequests)
	}

	store.RecordProbe("p1", true, 100*time.Millisecond, "")

	state = store.Snapshot("p1")
	if state.TotalSuccesses != 2 {
		t.Errorf("expected 2 successes, got %d", state.TotalSuccesses)
	}
	if state.LastProbeDuration != 100*time.Millisecond {
		t.Errorf("expected latency 100ms")
	}

	// Verify probe snapshot is stored
	data, err := cache.GetProbeSnapshot(context.Background(), "p1")
	if err != nil {
		t.Errorf("expected probe snapshot to be stored")
	}
	if data == nil {
		t.Errorf("expected data in probe snapshot")
	}
}

func TestRedisStore_ModelSnapshot(t *testing.T) {
	cache := hotstate.NewLocalHotState()
	store := health.NewRedisStore(cache, "1m")

	// Initial unknown state
	snap := store.ModelSnapshot("p1", "gpt-4")
	if snap.TotalSuccesses != 0 || snap.TotalFailures != 0 {
		t.Errorf("expected zero counters for unknown model")
	}

	// Record success
	store.RecordModelOutcome("p1", "gpt-4", true)
	snap = store.ModelSnapshot("p1", "gpt-4")
	if snap.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", snap.TotalSuccesses)
	}

	// Record failure
	store.RecordModelOutcome("p1", "gpt-4", false)
	snap = store.ModelSnapshot("p1", "gpt-4")
	if snap.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", snap.TotalFailures)
	}
}
