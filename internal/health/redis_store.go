package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"veloxmesh/internal/hotstate"
)

const defaultHealthSyncTimeout = 50 * time.Millisecond

type RedisStoreOptions struct {
	TTL         string
	SyncTimeout time.Duration
	Logger      *slog.Logger
}
type RedisStore struct {
	client           hotstate.Client
	ttl, syncTimeout time.Duration
	logger           *slog.Logger
	mu               sync.RWMutex
	localMap         map[string]*RedisProviderState
	localModels      map[string]*RedisModelState
	gates            map[string]chan struct{}
}
type RedisModelState struct {
	ProviderID     string    `json:"provider_id"`
	Model          string    `json:"model"`
	TotalSuccesses int       `json:"total_successes"`
	TotalFailures  int       `json:"total_failures"`
	LastUpdated    time.Time `json:"last_updated"`
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
	LastProbeAt          time.Time     `json:"last_probe_at"`
	LastProbeSuccess     bool          `json:"last_probe_success"`
	LastProbeError       string        `json:"last_probe_error"`
	LastProbeDuration    time.Duration `json:"last_probe_duration"`
}

func NewRedisStore(client hotstate.Client, ttl string) *RedisStore {
	return NewRedisStoreWithOptions(client, RedisStoreOptions{TTL: ttl})
}
func NewRedisStoreWithOptions(client hotstate.Client, options RedisStoreOptions) *RedisStore {
	ttl, _ := time.ParseDuration(options.TTL)
	if ttl <= 0 {
		ttl = time.Minute
	}
	if options.SyncTimeout <= 0 {
		options.SyncTimeout = defaultHealthSyncTimeout
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &RedisStore{client: client, ttl: ttl, syncTimeout: options.SyncTimeout, logger: options.Logger,
		localMap: make(map[string]*RedisProviderState), localModels: make(map[string]*RedisModelState), gates: make(map[string]chan struct{})}
}
func (s *RedisStore) EnsureProvider(id string, failureThreshold, successThreshold int) {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if successThreshold <= 0 {
		successThreshold = 1
	}
	s.mu.Lock()
	_, exists := s.localMap[id]
	if !exists {
		s.localMap[id] = &RedisProviderState{ID: id, Status: StatusHealthy, FailureThreshold: failureThreshold, SuccessThreshold: successThreshold, LastUpdated: time.Now()}
	}
	s.mu.Unlock()
	if !exists {
		s.publishProvider(id)
	}
}
func (s *RedisStore) updateProvider(id string, update func(RedisProviderState) RedisProviderState) {
	s.mu.Lock()
	state, exists := s.localMap[id]
	if exists {
		updated := update(*state)
		updated.LastUpdated = time.Now()
		s.localMap[id] = &updated
	}
	s.mu.Unlock()
	if exists {
		s.publishProvider(id)
	}
}
func (s *RedisStore) BeginRequest(id string) {
	s.updateProvider(id, func(state RedisProviderState) RedisProviderState { state.PendingRequests++; return state })
}
func (s *RedisStore) EndRequest(id string, latency time.Duration, err error) {
	s.updateProvider(id, func(state RedisProviderState) RedisProviderState {
		if state.PendingRequests > 0 {
			state.PendingRequests--
		}
		return redisOutcome(state, latency, err)
	})
}
func redisOutcome(state RedisProviderState, latency time.Duration, err error) RedisProviderState {
	if err != nil {
		state.ConsecutiveFailures++
		state.ConsecutiveSuccesses = 0
		state.TotalFailures++
		state.LastError = err.Error()
		return state
	}
	state.ConsecutiveSuccesses++
	state.TotalSuccesses++
	if state.ConsecutiveSuccesses >= state.SuccessThreshold {
		state.ConsecutiveFailures, state.ConsecutiveSuccesses, state.LastError = 0, 0, ""
	}
	if state.EWMALatency == 0 {
		state.EWMALatency = latency
	} else {
		state.EWMALatency = time.Duration(float64(latency)*0.2 + float64(state.EWMALatency)*0.8)
	}
	return state
}
func (s *RedisStore) RecordProbe(id string, success bool, latency time.Duration, errMsg string) {
	s.updateProvider(id, func(state RedisProviderState) RedisProviderState {
		var err error
		if !success {
			err = fmt.Errorf("%s", errMsg)
		}
		updated := redisOutcome(state, latency, err)
		updated.LastProbeAt, updated.LastProbeDuration = time.Now(), latency
		updated.LastProbeSuccess, updated.LastProbeError = success, errMsg
		return updated
	})
	s.publish("probe:"+id, func(ctx context.Context) error {
		state := s.providerCopy(id)
		if state == nil {
			return nil
		}
		data, err := json.Marshal(map[string]interface{}{"success": state.LastProbeSuccess, "latency": state.LastProbeDuration, "error": state.LastProbeError, "time": state.LastProbeAt})
		if err != nil {
			return err
		}
		return s.writeSnapshot(ctx, hotstate.SnapshotWrite{Category: "probe", Key: id, Version: fmt.Sprintf("%020d", state.LastProbeAt.UnixNano()), Data: data, TTL: s.ttl})
	})
}
func (s *RedisStore) providerCopy(id string) *RedisProviderState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state := s.localMap[id]; state != nil {
		clone := *state
		return &clone
	}
	return nil
}
func (s *RedisStore) publishProvider(id string) {
	s.publish("provider:"+id, func(ctx context.Context) error {
		state := s.providerCopy(id)
		if state == nil {
			return nil
		}
		return s.syncToRedis(ctx, id, state)
	})
}

// Atomic ordered writers reject stale snapshots at the destination, so a slow
// reply must not queue newer requests for the same provider behind it. Legacy
// writers retain the per-key gate and copy the latest state after acquiring it.
// Both paths keep network I/O bounded without holding the global state mutex.
func (s *RedisStore) publish(key string, send func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
	defer cancel()
	if _, ordered := s.client.(hotstate.OrderedSnapshotWriter); ordered {
		if err := send(ctx); err != nil {
			s.logger.Error("Redis health replication failed", "key", key, "error", err)
		}
		return
	}
	s.mu.Lock()
	gate := s.gates[key]
	if gate == nil {
		gate = make(chan struct{}, 1)
		s.gates[key] = gate
	}
	s.mu.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		s.logger.Error("Redis health replication failed", "key", key, "error", ctx.Err())
		return
	}
	if err := send(ctx); err != nil {
		s.logger.Error("Redis health replication failed", "key", key, "error", err)
	}
}
func (s *RedisStore) syncToRedis(ctx context.Context, id string, state *RedisProviderState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal health state: %w", err)
	}
	return s.writeSnapshot(ctx, hotstate.SnapshotWrite{Category: "health", Key: id, Version: fmt.Sprintf("%020d", state.LastUpdated.UnixNano()), Data: data, TTL: s.ttl})
}
