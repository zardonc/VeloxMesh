package health

import (
	"sync"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

type ProviderSnapshot struct {
	ID                  string
	Status              Status
	EWMALatency         time.Duration
	PendingRequests     int
	ConsecutiveFailures int
	TotalSuccesses      int
	TotalFailures       int
	LastError           error
	LastUpdated         time.Time

	LastProbeAt       time.Time
	LastProbeSuccess  bool
	LastProbeError    string
	LastProbeDuration time.Duration
}

type ModelSnapshot struct {
	ProviderID     string
	Model          string
	TotalSuccesses int
	TotalFailures  int
	LastUpdated    time.Time
}

type Store interface {
	EnsureProvider(id string, failureThreshold, successThreshold int)
	BeginRequest(id string)
	EndRequest(id string, latency time.Duration, err error)
	RecordProbe(id string, success bool, latency time.Duration, errMsg string)
	Snapshot(id string) ProviderSnapshot
	Snapshots() map[string]ProviderSnapshot

	RecordModelOutcome(providerID, model string, success bool)
	ModelSnapshot(providerID, model string) ModelSnapshot
}

type inMemoryStore struct {
	mu        sync.RWMutex
	providers map[string]*providerState
	models    map[string]*modelState
}

type providerState struct {
	id                   string
	ewmaLatency          time.Duration
	pendingRequests      int
	consecutiveFailures  int
	consecutiveSuccesses int
	failureThreshold     int
	successThreshold     int
	totalSuccesses       int
	totalFailures        int
	lastError            error
	lastUpdated          time.Time

	lastProbeAt       time.Time
	lastProbeSuccess  bool
	lastProbeError    string
	lastProbeDuration time.Duration
}

type modelState struct {
	providerID     string
	model          string
	totalSuccesses int
	totalFailures  int
	lastUpdated    time.Time
}

func NewInMemoryStore() Store {
	return &inMemoryStore{
		providers: make(map[string]*providerState),
		models:    make(map[string]*modelState),
	}
}

func (s *inMemoryStore) EnsureProvider(id string, failureThreshold, successThreshold int) {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if successThreshold <= 0 {
		successThreshold = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.providers[id]; !exists {
		s.providers[id] = &providerState{
			id:               id,
			failureThreshold: failureThreshold,
			successThreshold: successThreshold,
			lastUpdated:      time.Now(),
		}
	} else {
		s.providers[id].failureThreshold = failureThreshold
		s.providers[id].successThreshold = successThreshold
	}
}

func (s *inMemoryStore) BeginRequest(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, exists := s.providers[id]; exists {
		state.pendingRequests++
		state.lastUpdated = time.Now()
	}
}

func (s *inMemoryStore) EndRequest(id string, latency time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, exists := s.providers[id]
	if !exists {
		return
	}

	if state.pendingRequests > 0 {
		state.pendingRequests--
	}
	state.lastUpdated = time.Now()

	if err != nil {
		state.consecutiveFailures++
		state.consecutiveSuccesses = 0
		state.totalFailures++
		state.lastError = err
	} else {
		state.consecutiveSuccesses++
		state.totalSuccesses++
		if state.consecutiveSuccesses >= state.successThreshold {
			state.consecutiveFailures = 0
			state.consecutiveSuccesses = 0
			state.lastError = nil
		}

		// Simple EWMA: 0.2 * new + 0.8 * old
		if state.ewmaLatency == 0 {
			state.ewmaLatency = latency
		} else {
			state.ewmaLatency = time.Duration(float64(latency)*0.2 + float64(state.ewmaLatency)*0.8)
		}
	}
}

func (s *inMemoryStore) RecordProbe(id string, success bool, latency time.Duration, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, exists := s.providers[id]
	if !exists {
		return
	}

	state.lastProbeAt = time.Now()
	state.lastProbeDuration = latency
	state.lastProbeSuccess = success
	state.lastProbeError = errMsg

	if !success {
		state.consecutiveFailures++
		state.consecutiveSuccesses = 0
		state.totalFailures++
	} else {
		state.consecutiveSuccesses++
		state.totalSuccesses++
		if state.consecutiveSuccesses >= state.successThreshold {
			state.consecutiveFailures = 0
			state.consecutiveSuccesses = 0
			state.lastError = nil
		}
	}
	state.lastUpdated = time.Now()
}

func (s *inMemoryStore) Snapshot(id string) ProviderSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, exists := s.providers[id]
	if !exists {
		return ProviderSnapshot{ID: id, Status: StatusUnhealthy} // Unknown provider considered unhealthy
	}

	return s.buildSnapshot(state)
}

func (s *inMemoryStore) Snapshots() map[string]ProviderSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ProviderSnapshot, len(s.providers))
	for id, state := range s.providers {
		result[id] = s.buildSnapshot(state)
	}
	return result
}

func (s *inMemoryStore) buildSnapshot(state *providerState) ProviderSnapshot {
	status := StatusHealthy
	if state.consecutiveFailures >= state.failureThreshold {
		status = StatusUnhealthy
	} else if state.consecutiveFailures > 0 {
		status = StatusDegraded
	}

	return ProviderSnapshot{
		ID:                  state.id,
		Status:              status,
		EWMALatency:         state.ewmaLatency,
		PendingRequests:     state.pendingRequests,
		ConsecutiveFailures: state.consecutiveFailures,
		TotalSuccesses:      state.totalSuccesses,
		TotalFailures:       state.totalFailures,
		LastError:           state.lastError,
		LastUpdated:         state.lastUpdated,
		LastProbeAt:         state.lastProbeAt,
		LastProbeSuccess:    state.lastProbeSuccess,
		LastProbeError:      state.lastProbeError,
		LastProbeDuration:   state.lastProbeDuration,
	}
}

func (s *inMemoryStore) RecordModelOutcome(providerID, model string, success bool) {
	key := providerID + ":" + model
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.models[key]
	if !exists {
		state = &modelState{
			providerID: providerID,
			model:      model,
		}
		s.models[key] = state
	}

	if success {
		state.totalSuccesses++
	} else {
		state.totalFailures++
	}
	state.lastUpdated = time.Now()
}

func (s *inMemoryStore) ModelSnapshot(providerID, model string) ModelSnapshot {
	key := providerID + ":" + model
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.models[key]
	if !exists {
		return ModelSnapshot{
			ProviderID: providerID,
			Model:      model,
		}
	}

	return ModelSnapshot{
		ProviderID:     state.providerID,
		Model:          state.model,
		TotalSuccesses: state.totalSuccesses,
		TotalFailures:  state.totalFailures,
		LastUpdated:    state.lastUpdated,
	}
}
