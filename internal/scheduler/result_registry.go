package scheduler

import (
	"context"
	"sync"
)

type ResultRegistry struct {
	mu       sync.RWMutex
	channels map[string]chan TaskResult
	handlers map[string]TaskHandler
}

func NewResultRegistry() *ResultRegistry {
	return &ResultRegistry{
		channels: map[string]chan TaskResult{},
		handlers: map[string]TaskHandler{},
	}
}

func (r *ResultRegistry) Register(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[taskID] = make(chan TaskResult, 1)
}

type TaskHandler func(context.Context) TaskResult

func (r *ResultRegistry) RegisterHandler(taskID string, handler TaskHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[taskID] = handler
}

func (r *ResultRegistry) Handler(taskID string) (TaskHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[taskID]
	return handler, ok
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
	delete(r.handlers, taskID)
}
