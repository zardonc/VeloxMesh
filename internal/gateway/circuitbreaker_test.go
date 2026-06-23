package gateway

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	now := time.Now()
	cb := &CircuitBreaker{
		breakers: make(map[string]*ProviderBreaker),
		config: CircuitBreakerConfig{
			FailureThreshold: 3,
			RecoveryTimeout:  time.Minute,
		},
		nowFunc: func() time.Time { return now },
	}

	providerID := "test-provider"

	// Initial state: Closed, Allowed
	if !cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to allow request initially")
	}
	if state := cb.State(providerID); state != StateClosed {
		t.Errorf("Expected initial state %v, got %v", StateClosed, state)
	}

	// 1 failure: Closed, Allowed
	cb.RecordResult(providerID, false)
	if !cb.Allow(providerID) {
		t.Errorf("Expected to allow request after 1 failure")
	}

	// 2 failures: Closed, Allowed
	cb.RecordResult(providerID, false)
	if !cb.Allow(providerID) {
		t.Errorf("Expected to allow request after 2 failures")
	}

	// 3 failures: Open, Not Allowed
	cb.RecordResult(providerID, false)
	if cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to block request after %d failures", cb.config.FailureThreshold)
	}
	if state := cb.State(providerID); state != StateOpen {
		t.Errorf("Expected state %v, got %v", StateOpen, state)
	}

	// Time advances, but not enough: Open, Not Allowed
	now = now.Add(time.Second * 30)
	if cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to block request before recovery timeout")
	}

	// Time advances past recovery timeout: HalfOpen, Allowed for one request
	now = now.Add(time.Second * 31)
	if !cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to allow one request after recovery timeout (half-open)")
	}
	if state := cb.State(providerID); state != StateHalfOpen {
		t.Errorf("Expected state %v, got %v", StateHalfOpen, state)
	}

	// Second request while half-open: Not Allowed
	if cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to block second request while half-open")
	}

	// Record success: Closed, Allowed
	cb.RecordResult(providerID, true)
	if !cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to allow request after half-open success")
	}
	if state := cb.State(providerID); state != StateClosed {
		t.Errorf("Expected state %v, got %v", StateClosed, state)
	}

	// Now fail again 3 times to reopen
	cb.RecordResult(providerID, false)
	cb.RecordResult(providerID, false)
	cb.RecordResult(providerID, false)

	// Time advances: HalfOpen
	now = now.Add(time.Minute)

	// Allow once
	if !cb.Allow(providerID) {
		t.Errorf("Expected to allow one request in half-open state")
	}

	// Fail in half-open: Open immediately
	cb.RecordResult(providerID, false)
	if cb.Allow(providerID) {
		t.Errorf("Expected circuit breaker to block request after half-open failure")
	}
	if state := cb.State(providerID); state != StateOpen {
		t.Errorf("Expected state %v, got %v", StateOpen, state)
	}
}
