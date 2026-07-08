package scheduler

import (
	"context"
	"sort"
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
	if q.primary == nil {
		return q.fallback.Push(ctx, item)
	}
	if err := q.primary.Push(ctx, item); err == nil {
		q.primaryAvailable = true
		if err := q.fallback.Remove(ctx, item.TaskID); err != nil && err != ErrTaskNotFound {
			return err
		}
		return nil
	}
	q.primaryAvailable = false
	return q.fallback.Push(ctx, item)
}

func (q *FallbackQueue) PeekMin(ctx context.Context, limit int) ([]QueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	fallbackItems, err := q.fallback.PeekMin(ctx, limit)
	if err != nil || q.primary == nil {
		return fallbackItems, err
	}
	primaryItems, err := q.primary.PeekMin(ctx, limit)
	if err != nil {
		q.primaryAvailable = false
		return fallbackItems, nil
	}
	q.primaryAvailable = true
	return mergeQueueItems(primaryItems, fallbackItems, limit), nil
}

func (q *FallbackQueue) PopMin(ctx context.Context) (QueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	fallbackItems, err := q.fallback.PeekMin(ctx, 1)
	if err != nil || q.primary == nil {
		return q.fallback.PopMin(ctx)
	}
	primaryItems, err := q.primary.PeekMin(ctx, 1)
	if err != nil {
		q.primaryAvailable = false
		return q.fallback.PopMin(ctx)
	}
	q.primaryAvailable = true
	if len(primaryItems) == 0 {
		return q.fallback.PopMin(ctx)
	}
	if len(fallbackItems) > 0 && fallbackItems[0].Score <= primaryItems[0].Score {
		return q.fallback.PopMin(ctx)
	}
	item, err := q.primary.PopMin(ctx)
	if err != nil {
		if err != ErrQueueEmpty {
			q.primaryAvailable = false
		}
		if len(fallbackItems) > 0 {
			return q.fallback.PopMin(ctx)
		}
	}
	return item, err
}

func (q *FallbackQueue) Remove(ctx context.Context, taskID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.primary == nil {
		return q.fallback.Remove(ctx, taskID)
	}
	err := q.primary.Remove(ctx, taskID)
	if err == nil {
		q.primaryAvailable = true
		if fallbackErr := q.fallback.Remove(ctx, taskID); fallbackErr != nil && fallbackErr != ErrTaskNotFound {
			return fallbackErr
		}
		return nil
	}
	if err != ErrTaskNotFound {
		q.primaryAvailable = false
	}
	return q.fallback.Remove(ctx, taskID)
}

func (q *FallbackQueue) Len(ctx context.Context) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	fallbackLen, err := q.fallback.Len(ctx)
	if err != nil || q.primary == nil {
		return fallbackLen, err
	}
	primaryLen, err := q.primary.Len(ctx)
	if err != nil {
		q.primaryAvailable = false
		return fallbackLen, nil
	}
	q.primaryAvailable = true
	return primaryLen + fallbackLen, nil
}

func mergeQueueItems(primary []QueueItem, fallback []QueueItem, limit int) []QueueItem {
	if limit < 1 {
		return []QueueItem{}
	}
	items := append(append([]QueueItem{}, primary...), fallback...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Score < items[j].Score
	})
	if len(items) > limit {
		return items[:limit]
	}
	return items
}
