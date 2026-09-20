package scheduler

import (
	"context"
	"sync"
)

type ResultRegistry struct {
	mu       sync.RWMutex
	channels map[string]chan TaskResult
	handlers map[string]TaskHandler
	tasks    map[string]Task
	contexts map[string]context.Context
	cancels  map[string]context.CancelFunc
	running  map[string]struct{}
}

func NewResultRegistry() *ResultRegistry {
	return &ResultRegistry{
		channels: map[string]chan TaskResult{},
		handlers: map[string]TaskHandler{},
		tasks:    map[string]Task{},
		contexts: map[string]context.Context{},
		cancels:  map[string]context.CancelFunc{},
		running:  map[string]struct{}{},
	}
}

func (r *ResultRegistry) Register(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[taskID] = make(chan TaskResult, 1)
}

type TaskHandler func(context.Context) TaskResult

func (r *ResultRegistry) RegisterTask(task Task, handler TaskHandler) error {
	return r.RegisterTaskWithContext(context.Background(), task, handler)
}

func (r *ResultRegistry) RegisterTaskWithContext(ctx context.Context, task Task, handler TaskHandler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	execCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.channels[task.ID]; ok {
		cancel()
		return ErrDuplicateTask
	}
	r.channels[task.ID] = make(chan TaskResult, 1)
	r.handlers[task.ID] = handler
	r.tasks[task.ID] = cloneTask(task)
	r.contexts[task.ID] = execCtx
	r.cancels[task.ID] = cancel
	return nil
}

func (r *ResultRegistry) RegisterHandler(taskID string, handler TaskHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[taskID] = handler
}

func (r *ResultRegistry) Task(taskID string) (Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return Task{}, false
	}
	return cloneTask(task), true
}

func (r *ResultRegistry) Handler(taskID string) (TaskHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[taskID]
	return handler, ok
}

func (r *ResultRegistry) HandlerContext(taskID string) (TaskHandler, context.Context, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, handlerOK := r.handlers[taskID]
	ctx, ctxOK := r.contexts[taskID]
	return handler, ctx, handlerOK && ctxOK
}

func (r *ResultRegistry) MarkRunning(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[taskID] = struct{}{}
}

func (r *ResultRegistry) IsRunning(taskID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.running[taskID]
	return ok
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
	cancel := r.cancels[taskID]
	delete(r.channels, taskID)
	delete(r.handlers, taskID)
	delete(r.tasks, taskID)
	delete(r.contexts, taskID)
	delete(r.cancels, taskID)
	delete(r.running, taskID)
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func cloneTask(task Task) Task {
	if task.Metadata == nil {
		return task
	}
	metadata := make(map[string]string, len(task.Metadata))
	for k, v := range task.Metadata {
		metadata[k] = v
	}
	task.Metadata = metadata
	return task
}
