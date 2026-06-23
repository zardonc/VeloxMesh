package gateway

import (
	"sync"
	"time"
)

type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half-open"
)

type CircuitBreakerConfig struct {
	FailureThreshold int
	RecoveryTimeout  time.Duration
}

type ProviderBreaker struct {
	state          CircuitState
	failures       int
	openedAt       time.Time
	halfOpenTested bool
}

type CircuitBreaker struct {
	mu       sync.RWMutex
	breakers map[string]*ProviderBreaker
	config   CircuitBreakerConfig
	nowFunc  func() time.Time
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		breakers: make(map[string]*ProviderBreaker),
		config:   cfg,
		nowFunc:  time.Now,
	}
}

// UpdateConfig allows dynamically updating the circuit breaker configuration.
func (cb *CircuitBreaker) UpdateConfig(cfg CircuitBreakerConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.config = cfg
}

// Allow returns true if the provider's circuit is closed or half-open and ready to test.
func (cb *CircuitBreaker) Allow(providerID string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	pb, exists := cb.breakers[providerID]
	if !exists {
		return true // Closed by default
	}

	if pb.state == StateClosed {
		return true
	}

	if pb.state == StateOpen {
		if cb.nowFunc().Sub(pb.openedAt) >= cb.config.RecoveryTimeout {
			pb.state = StateHalfOpen
			pb.halfOpenTested = true
			return true
		}
		return false
	}

	if pb.state == StateHalfOpen {
		if pb.halfOpenTested {
			return false // only one request allowed to test half-open
		}
		pb.halfOpenTested = true
		return true
	}

	return true
}

// RecordResult updates the circuit breaker state based on the outcome of a request.
func (cb *CircuitBreaker) RecordResult(providerID string, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	pb, exists := cb.breakers[providerID]
	if !exists {
		if success {
			return
		}
		pb = &ProviderBreaker{state: StateClosed}
		cb.breakers[providerID] = pb
	}

	if success {
		pb.failures = 0
		pb.state = StateClosed
		pb.halfOpenTested = false
		return
	}

	if pb.state == StateHalfOpen {
		pb.state = StateOpen
		pb.openedAt = cb.nowFunc()
		pb.halfOpenTested = false
		return
	}

	pb.failures++
	if pb.failures >= cb.config.FailureThreshold {
		pb.state = StateOpen
		pb.openedAt = cb.nowFunc()
	}
}

// State returns the current state of a provider's circuit breaker.
func (cb *CircuitBreaker) State(providerID string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	pb, exists := cb.breakers[providerID]
	if !exists {
		return StateClosed
	}
	// For testing, we also check if an open state has expired
	if pb.state == StateOpen && cb.nowFunc().Sub(pb.openedAt) >= cb.config.RecoveryTimeout {
		return StateHalfOpen
	}
	return pb.state
}
