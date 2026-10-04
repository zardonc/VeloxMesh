package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"veloxmesh/internal/hotstate"
)

func (s *RedisStore) Snapshot(id string) ProviderSnapshot {
	ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
	defer cancel()
	data, err := s.client.GetHealthSnapshot(ctx, id)
	if err != nil {
		if errors.Is(err, hotstate.ErrCacheMiss) {
			state := s.providerCopy(id)
			if state == nil {
				return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
			}
			return s.buildSnapshot(state)
		}
		s.logger.Error("Redis health snapshot read failed", "provider", id, "error", err)
		return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
	}
	var remote RedisProviderState
	if err := json.Unmarshal(data, &remote); err != nil {
		s.logger.Error("Redis health snapshot decode failed", "provider", id, "error", err)
		return ProviderSnapshot{ID: id, Status: StatusUnhealthy}
	}
	s.mu.Lock()
	chosen := remote
	if local := s.localMap[id]; local != nil && !remote.LastUpdated.After(local.LastUpdated) {
		chosen = *local
	}
	s.localMap[id] = &chosen
	s.mu.Unlock()
	return s.buildSnapshot(&chosen)
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
	return ProviderSnapshot{ID: state.ID, Status: status, EWMALatency: state.EWMALatency, PendingRequests: state.PendingRequests,
		ConsecutiveFailures: state.ConsecutiveFailures, TotalSuccesses: state.TotalSuccesses, TotalFailures: state.TotalFailures,
		LastError: lastErr, LastUpdated: state.LastUpdated, LastProbeAt: state.LastProbeAt, LastProbeSuccess: state.LastProbeSuccess,
		LastProbeError: state.LastProbeError, LastProbeDuration: state.LastProbeDuration}
}

func (s *RedisStore) RecordModelOutcome(providerID, model string, success bool) {
	key := providerID + ":" + model
	s.mu.Lock()
	state := RedisModelState{ProviderID: providerID, Model: model}
	if existing := s.localModels[key]; existing != nil {
		state = *existing
	}
	if success {
		state.TotalSuccesses++
	} else {
		state.TotalFailures++
	}
	state.LastUpdated = time.Now()
	s.localModels[key] = &state
	s.mu.Unlock()
	s.publish("model:"+key, func(ctx context.Context) error {
		latest := s.modelCopy(key)
		data, err := json.Marshal(latest)
		if err != nil {
			return err
		}
		return s.writeSnapshot(ctx, hotstate.SnapshotWrite{Category: "bytes", Key: "model_snapshot:" + key, Version: fmt.Sprintf("%020d", latest.LastUpdated.UnixNano()), Data: data, TTL: s.ttl})
	})
}

func (s *RedisStore) modelCopy(key string) RedisModelState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state := s.localModels[key]; state != nil {
		return *state
	}
	return RedisModelState{}
}

func (s *RedisStore) ModelSnapshot(providerID, model string) ModelSnapshot {
	key := providerID + ":" + model
	ctx, cancel := context.WithTimeout(context.Background(), s.syncTimeout)
	defer cancel()
	data, err := s.client.GetBytes(ctx, "model_snapshot:"+key)
	chosen := s.modelCopy(key)
	if err != nil && !errors.Is(err, hotstate.ErrCacheMiss) {
		s.logger.Error("Redis model snapshot read failed", "key", key, "error", err)
	}
	if err == nil {
		chosen = s.mergeModelSnapshot(key, data)
	}
	return ModelSnapshot{ProviderID: providerID, Model: model, TotalSuccesses: chosen.TotalSuccesses, TotalFailures: chosen.TotalFailures, LastUpdated: chosen.LastUpdated}
}

func (s *RedisStore) mergeModelSnapshot(key string, data []byte) RedisModelState {
	var remote RedisModelState
	if err := json.Unmarshal(data, &remote); err != nil {
		s.logger.Error("Redis model snapshot decode failed", "key", key, "error", err)
		return s.modelCopy(key)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	chosen := remote
	if local := s.localModels[key]; local != nil && !remote.LastUpdated.After(local.LastUpdated) {
		chosen = *local
	}
	s.localModels[key] = &chosen
	return chosen
}
