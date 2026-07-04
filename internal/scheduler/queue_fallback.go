package scheduler

import (
	"context"
	"sync"
)

type FallbackQueue struct {
	mu               sync.Mutex
	primary          QueueBackend
	fallback         *MemoryQueue
	primaryAvailable bool
}

func NewFallbackQueue(primary QueueBackend, fallback *MemoryQueue) *FallbackQueue {
	if fallback == nil {
		fallback = NewMemoryQueue()
	}
	return &FallbackQueue{primary: primary, fallback: fallback, primaryAvailable: true}
}

func (q *FallbackQueue) MarkPrimaryAvailable() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.primaryAvailable = true
}

func (q *FallbackQueue) Push(ctx context.Context, item QueueItem) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.primaryAvailable && q.primary != nil {
		if err := q.primary.Push(ctx, item); err == nil {
			return nil
		}
		q.primaryAvailable = false
	}
	return q.fallback.Push(ctx, item)
}

func (q *FallbackQueue) PopMin(ctx context.Context) (QueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.primaryAvailable && q.primary != nil {
		item, err := q.primary.PopMin(ctx)
		if err == nil || err == ErrQueueEmpty {
			return item, err
		}
		q.primaryAvailable = false
	}
	return q.fallback.PopMin(ctx)
}

func (q *FallbackQueue) Remove(ctx context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.primaryAvailable && q.primary != nil {
		if err := q.primary.Remove(ctx, taskID); err == nil || err == ErrTaskNotFound {
			return err
		}
		q.primaryAvailable = false
	}
	return q.fallback.Remove(ctx, taskID)
}

func (q *FallbackQueue) Len(ctx context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.primaryAvailable && q.primary != nil {
		length, err := q.primary.Len(ctx)
		if err == nil {
			return length, nil
		}
		q.primaryAvailable = false
	}
	return q.fallback.Len(ctx)
}
