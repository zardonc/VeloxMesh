package health

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"veloxmesh/internal/hotstate"
)

type RedisStore struct {
	client hotstate.Client
	ttl    time.Duration

	// Local state for atomic counters since snapshot is a full struct replacement
	mu       sync.RWMutex
	localMap map[string]*RedisProviderState
}

type RedisProviderState struct {
	ID                   string        `json:"id"`
	Status               Status        `json:"status"`
	EWMALatency          time.Duration `json:"ewma_latency"`
	PendingRequests      int           `json:"pending_requests"`
	ConsecutiveFailures  int           `json:"consecutive_failures"`
	ConsecutiveSuccesses int           `json:"consecutive_successes"`
	TotalSuccesses       int           `json:"total_successes"`
	TotalFailures        int           `json:"total_failures"`
	LastError            string        `json:"last_error,omitempty"`
	LastUpdated          time.Time     `json:"last_updated"`
	FailureThreshold     int           `json:"failure_threshold"`
	SuccessThreshold     int           `json:"success_threshold"`

	LastProbeAt       time.Time     `json:"last_probe_at"`
	LastProbeSuccess  bool          `json:"last_probe_success"`
	LastProbeError    string        `json:"last_probe_error"`
	LastProbeDuration time.Duration `json:"last_probe_duration"`
}

func NewRedisStore(client hotstate.Client, ttlStr string) *RedisStore {
	ttl, _ := time.ParseDuration(ttlStr)
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &RedisStore{
		client:   client,
		ttl:      ttl,
		localMap: make(map[string]*RedisProviderState),
	}
}

func (s *RedisStore) EnsureProvider(id string, failureThreshold, successThreshold int) {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if successThreshold <= 0 {
		successThreshold = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.localMap[id]; !exists {
		s.localMap[id] = &RedisProviderState{
			ID:               id,
			Status:           StatusHealthy,
			FailureThreshold: failureThreshold,
			SuccessThreshold: successThreshold,
			LastUpdated:      time.Now(),
		}
		s.syncToRedis(context.Background(), id, s.localMap[id])
	}
}

func (s *RedisStore) BeginRequest(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, exists := s.localMap[id]
	if !exists {
		return
	}
	state.PendingRequests++
	state.LastUpdated = time.Now()
	s.syncToRedis(context.Background(), id, state)
}

func (s *RedisStore) EndRequest(id string, latency time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, exists := s.localMap[id]
	if !exists {
		return
	}
	if state.PendingRequests > 0 {
		state.PendingRequests--
	}

	if err != nil {
		state.ConsecutiveFailures++
		state.ConsecutiveSuccesses = 0
		state.TotalFailures++
		state.LastError = err.Error()
	} else {
		state.ConsecutiveSuccesses++
		state.TotalSuccesses++
		if state.ConsecutiveSuccesses >= state.SuccessThreshold {
			state.ConsecutiveFailures = 0
			state.ConsecutiveSuccesses = 0
			state.LastError = ""
		}
		if state.EWMALatency == 0 {
			state.EWMALatency = latency
		} else {
			state.EWMALatency = time.Duration(float64(latency)*0.2 + float64(state.EWMALatency)*0.8)
		}
	}
	state.LastUpdated = time.Now()
	s.syncToRedis(context.Background(), id, state)
}

func (s *RedisStore) RecordProbe(id string, success bool, latency time.Duration, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.localMap[id]
	if !exists {
		return
	}

	state.LastProbeAt = time.Now()
	state.LastProbeDuration = latency
	state.LastProbeSuccess = success
	state.LastProbeError = errMsg

	if !success {
		state.ConsecutiveFailures++
		state.ConsecutiveSuccesses = 0
		state.TotalFailures++
		if state.Status == StatusHealthy && state.ConsecutiveFailures >= state.FailureThreshold {
			state.Status = StatusUnhealthy
		}
	} else {
		state.ConsecutiveSuccesses++
		state.TotalSuccesses++
		if state.Status == StatusUnhealthy && state.ConsecutiveSuccesses >= state.SuccessThreshold {
			state.Status = StatusHealthy
			state.ConsecutiveFailures = 0
			state.ConsecutiveSuccesses = 0
		}
	}
	state.LastUpdated = time.Now()

	s.syncToRedis(context.Background(), id, state)

	// Save probe snapshot directly (RedisStore specifics)
	probeData := map[string]interface{}{
		"success": success,
		"latency": latency,
		"error":   errMsg,
		"time":    state.LastProbeAt,
	}
	data, _ := json.Marshal(probeData)
	_ = s.client.SetProbeSnapshot(context.Background(), id, data, s.ttl)
}

func (s *RedisStore) Snapshot(id string) ProviderSnapshot {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := s.client.GetHealthSnapshot(ctx, id)
	if err != nil {
		if err == hotstate.ErrCacheMiss {
			// Fallback to local
			s.mu.RLock()
			defer s.mu.RUnlock()
			state, exists := s.localMap[id]
			if !exists {
				return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
			}
			return s.buildSnapshot(state)
		}
		return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
	}

	var state RedisProviderState
	if err := json.Unmarshal(data, &state); err != nil {
		return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
	}

	// Sync local map
	s.mu.Lock()
	if existing, ok := s.localMap[id]; ok {
		*existing = state
	}
	s.mu.Unlock()

	return s.buildSnapshot(&state)
}

func (s *RedisStore) Snapshots() map[string]ProviderSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ProviderSnapshot, len(s.localMap))
	for id, state := range s.localMap {
		result[id] = s.buildSnapshot(state)
	}
	return result
}

func (s *RedisStore) buildSnapshot(state *RedisProviderState) ProviderSnapshot {
	status := StatusHealthy
	if state.ConsecutiveFailures >= state.FailureThreshold {
		status = StatusUnhealthy
	} else if state.ConsecutiveFailures > 0 {
		status = StatusDegraded
	}

	var lastErr error
	if state.LastError != "" {
		lastErr = fmt.Errorf("%s", state.LastError)
	}

	return ProviderSnapshot{
		ID:                  state.ID,
		Status:              status,
		EWMALatency:         state.EWMALatency,
		PendingRequests:     state.PendingRequests,
		ConsecutiveFailures: state.ConsecutiveFailures,
		TotalSuccesses:      state.TotalSuccesses,
		TotalFailures:       state.TotalFailures,
		LastError:           lastErr,
		LastUpdated:         state.LastUpdated,
		LastProbeAt:         state.LastProbeAt,
		LastProbeSuccess:    state.LastProbeSuccess,
		LastProbeError:      state.LastProbeError,
		LastProbeDuration:   state.LastProbeDuration,
	}
}

func (s *RedisStore) syncToRedis(ctx context.Context, id string, state *RedisProviderState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal health state: %w", err)
	}
	return s.client.SetHealthSnapshot(ctx, id, data, s.ttl)
}
