package health_test

import (
	"errors"
	"testing"
	"time"
	"veloxmesh/internal/health"
)

func TestInMemoryStore_EndRequest(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)

	// Begin requests
	store.BeginRequest("p1")
	store.BeginRequest("p1")

	snap := store.Snapshot("p1")
	if snap.PendingRequests != 2 {
		t.Errorf("expected 2 pending requests, got %d", snap.PendingRequests)
	}

	// End request with success
	store.EndRequest("p1", 100*time.Millisecond, nil)
	snap = store.Snapshot("p1")
	if snap.PendingRequests != 1 {
		t.Errorf("expected 1 pending request, got %d", snap.PendingRequests)
	}
	if snap.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", snap.TotalSuccesses)
	}
	if snap.EWMALatency != 100*time.Millisecond {
		t.Errorf("expected EWMA 100ms, got %v", snap.EWMALatency)
	}
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected healthy, got %v", snap.Status)
	}

	// End request with error
	errTest := errors.New("timeout")
	store.EndRequest("p1", 0, errTest)
	snap = store.Snapshot("p1")
	if snap.PendingRequests != 0 {
		t.Errorf("expected 0 pending request, got %d", snap.PendingRequests)
	}
	if snap.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", snap.TotalFailures)
	}
	if snap.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", snap.ConsecutiveFailures)
	}
	if snap.LastError != errTest {
		t.Errorf("expected last error %v, got %v", errTest, snap.LastError)
	}
	if snap.Status != health.StatusDegraded {
		t.Errorf("expected degraded, got %v", snap.Status)
	}

	// End two more requests with error to trigger unhealthy
	store.EndRequest("p1", 0, errTest)
	store.EndRequest("p1", 0, errTest)
	snap = store.Snapshot("p1")
	if snap.Status != health.StatusUnhealthy {
		t.Errorf("expected unhealthy, got %v", snap.Status)
	}

	// Recover with success
	store.EndRequest("p1", 50*time.Millisecond, nil)
	snap = store.Snapshot("p1")
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected healthy after success, got %v", snap.Status)
	}
	if snap.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures after recovery, got %d", snap.ConsecutiveFailures)
	}
}

func TestInMemoryStore_RecordProbe(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)

	// Initial healthy probe
	store.RecordProbe("p1", true, 50*time.Millisecond, "")
	snap := store.Snapshot("p1")
	if !snap.LastProbeSuccess {
		t.Errorf("expected probe to be successful")
	}
	if snap.LastProbeDuration != 50*time.Millisecond {
		t.Errorf("expected probe duration 50ms, got %v", snap.LastProbeDuration)
	}
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected healthy, got %v", snap.Status)
	}

	// Failed probe
	store.RecordProbe("p1", false, 100*time.Millisecond, "connection refused")
	snap = store.Snapshot("p1")
	if snap.LastProbeSuccess {
		t.Errorf("expected probe to be failed")
	}
	if snap.LastProbeError != "connection refused" {
		t.Errorf("expected connection refused error, got %s", snap.LastProbeError)
	}
	if snap.Status != health.StatusDegraded {
		t.Errorf("expected degraded, got %v", snap.Status)
	}
	if snap.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", snap.ConsecutiveFailures)
	}

	// Two more failed probes to trigger unhealthy
	store.RecordProbe("p1", false, 10*time.Millisecond, "timeout")
	store.RecordProbe("p1", false, 10*time.Millisecond, "timeout")
	snap = store.Snapshot("p1")
	if snap.Status != health.StatusUnhealthy {
		t.Errorf("expected unhealthy, got %v", snap.Status)
	}

	// Successful probe to recover
	store.RecordProbe("p1", true, 40*time.Millisecond, "")
	snap = store.Snapshot("p1")
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected healthy, got %v", snap.Status)
	}
	if snap.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", snap.ConsecutiveFailures)
	}
	if snap.LastProbeError != "" {
		t.Errorf("expected empty probe error on success, got %s", snap.LastProbeError)
	}
}

func TestInMemoryStore_CustomThresholds(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p2", 2, 3)

	// Test custom failure threshold (2)
	store.RecordProbe("p2", false, 10*time.Millisecond, "err1")
	snap := store.Snapshot("p2")
	if snap.Status != health.StatusDegraded {
		t.Errorf("expected degraded after 1 fail, got %v", snap.Status)
	}

	store.RecordProbe("p2", false, 10*time.Millisecond, "err2")
	snap = store.Snapshot("p2")
	if snap.Status != health.StatusUnhealthy {
		t.Errorf("expected unhealthy after 2 fails, got %v", snap.Status)
	}

	// Test custom success threshold (3)
	// 1st success
	store.RecordProbe("p2", true, 10*time.Millisecond, "")
	snap = store.Snapshot("p2")
	if snap.Status != health.StatusUnhealthy {
		t.Errorf("expected still unhealthy after 1 success, got %v", snap.Status)
	}

	// 2nd success
	store.RecordProbe("p2", true, 10*time.Millisecond, "")
	snap = store.Snapshot("p2")
	if snap.Status != health.StatusUnhealthy {
		t.Errorf("expected still unhealthy after 2 successes, got %v", snap.Status)
	}

	// 3rd success - should recover
	store.RecordProbe("p2", true, 10*time.Millisecond, "")
	snap = store.Snapshot("p2")
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected healthy after 3 successes, got %v", snap.Status)
	}
	if snap.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", snap.ConsecutiveFailures)
	}
}

func TestInMemoryStore_ModelSnapshot(t *testing.T) {
	store := health.NewInMemoryStore()

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
