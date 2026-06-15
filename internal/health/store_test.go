package health_test

import (
	"errors"
	"testing"
	"time"
	"veloxmesh/internal/health"
)

func TestInMemoryStore_EndRequest(t *testing.T) {
	store := health.NewInMemoryStore(3)
	store.EnsureProvider("p1")

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
