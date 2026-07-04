package scheduler

import (
	"context"
	"sync"
)

type ResultRegistry struct {
	mu       sync.RWMutex
	channels map[string]chan TaskResult
}

func NewResultRegistry() *ResultRegistry {
	return &ResultRegistry{channels: map[string]chan TaskResult{}}
}

func (r *ResultRegistry) Register(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[taskID] = make(chan TaskResult, 1)
}

func (r *ResultRegistry) Deliver(taskID string, result TaskResult) bool {
	r.mu.RLock()
	ch, ok := r.channels[taskID]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ch <- result:
		return true
	default:
		return false
	}
}

func (r *ResultRegistry) Wait(ctx context.Context, taskID string) (TaskResult, error) {
	r.mu.RLock()
	ch, ok := r.channels[taskID]
	r.mu.RUnlock()
	if !ok {
		return TaskResult{}, ErrTaskNotFound
	}
	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return TaskResult{}, ctx.Err()
	}
}

func (r *ResultRegistry) Unregister(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, taskID)
}
