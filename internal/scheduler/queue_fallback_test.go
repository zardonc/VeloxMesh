package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestFallbackQueueConcurrentPrimaryFailure(t *testing.T) {
	ctx := context.Background()
	q := NewFallbackQueue(failingQueue{}, NewMemoryQueue())
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := q.Push(ctx, QueueItem{TaskID: fmt.Sprintf("t%d", i), Score: float64(i)})
			if err != nil {
				t.Errorf("Push: %v", err)
			}
		}(i)
	}
	wg.Wait()
	length, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 32 {
		t.Fatalf("Len=%d, want 32", length)
	}
}

func TestFallbackQueuePeekMinUsesMemoryAfterPrimaryError(t *testing.T) {
	ctx := context.Background()
	q := NewFallbackQueue(failingQueue{}, NewMemoryQueue())
	if err := q.Push(ctx, QueueItem{TaskID: "fallback", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	items, err := q.PeekMin(ctx, 1)
	if err != nil {
		t.Fatalf("PeekMin: %v", err)
	}
	if len(items) != 1 || items[0].TaskID != "fallback" {
		t.Fatalf("unexpected fallback peek: %#v", items)
	}
	got, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "fallback" {
		t.Fatalf("unexpected pop after fallback peek: %#v", got)
	}
}

func TestFallbackQueueRetriesPrimaryAfterFailure(t *testing.T) {
	ctx := context.Background()
	primary := &flakyQueue{queue: NewMemoryQueue()}
	q := NewFallbackQueue(primary, NewMemoryQueue())

	primary.available = false
	if err := q.Push(ctx, QueueItem{TaskID: "fallback", Score: 1}); err != nil {
		t.Fatalf("Push fallback: %v", err)
	}

	primary.available = true
	if err := q.Push(ctx, QueueItem{TaskID: "primary", Score: 2}); err != nil {
		t.Fatalf("Push primary: %v", err)
	}
	length, err := primary.queue.Len(ctx)
	if err != nil {
		t.Fatalf("primary Len: %v", err)
	}
	if length != 1 {
		t.Fatalf("primary Len=%d, want 1", length)
	}
}

func TestFallbackQueuePrimaryRecoveryPushRemovesFallbackDuplicate(t *testing.T) {
	ctx := context.Background()
	primary := &flakyQueue{queue: NewMemoryQueue()}
	q := NewFallbackQueue(primary, NewMemoryQueue())

	primary.available = false
	if err := q.Push(ctx, QueueItem{TaskID: "same-task", Score: 1}); err != nil {
		t.Fatalf("Push fallback: %v", err)
	}
	primary.available = true
	if err := q.Push(ctx, QueueItem{TaskID: "same-task", Score: 1}); err != nil {
		t.Fatalf("Push recovered primary: %v", err)
	}

	length, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 1 {
		t.Fatalf("Len=%d, want 1", length)
	}
}

func TestFallbackQueuePopMinReadsMemoryWhenPrimaryEmptyAfterRecovery(t *testing.T) {
	ctx := context.Background()
	primary := &flakyQueue{queue: NewMemoryQueue()}
	q := NewFallbackQueue(primary, NewMemoryQueue())

	primary.available = false
	if err := q.Push(ctx, QueueItem{TaskID: "fallback", Score: 1}); err != nil {
		t.Fatalf("Push fallback: %v", err)
	}
	primary.available = true
	q.MarkPrimaryAvailable()

	got, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "fallback" {
		t.Fatalf("unexpected item after primary recovery: %#v", got)
	}
}

func TestFallbackQueuePopMinMergesPrimaryAndFallback(t *testing.T) {
	ctx := context.Background()
	primary := &flakyQueue{queue: NewMemoryQueue(), available: true}
	q := NewFallbackQueue(primary, NewMemoryQueue())

	primary.available = false
	if err := q.Push(ctx, QueueItem{TaskID: "fallback", Score: 1}); err != nil {
		t.Fatalf("Push fallback: %v", err)
	}
	primary.available = true
	if err := q.Push(ctx, QueueItem{TaskID: "primary", Score: 2}); err != nil {
		t.Fatalf("Push primary: %v", err)
	}

	first, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("first PopMin: %v", err)
	}
	second, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("second PopMin: %v", err)
	}
	if first.TaskID != "fallback" || second.TaskID != "primary" {
		t.Fatalf("unexpected pop order: %#v then %#v", first, second)
	}
}

type failingQueue struct{}

func (failingQueue) Push(context.Context, QueueItem) error { return errors.New("primary down") }
func (failingQueue) PeekMin(context.Context, int) ([]QueueItem, error) {
	return nil, errors.New("primary down")
}
func (failingQueue) PopMin(context.Context) (QueueItem, error) {
	return QueueItem{}, errors.New("primary down")
}
func (failingQueue) Remove(context.Context, string) error { return errors.New("primary down") }
func (failingQueue) Len(context.Context) (int64, error)   { return 0, errors.New("primary down") }

type flakyQueue struct {
	queue     *MemoryQueue
	available bool
}

func (q *flakyQueue) Push(ctx context.Context, item QueueItem) error {
	if !q.available {
		return errors.New("primary down")
	}
	return q.queue.Push(ctx, item)
}

func (q *flakyQueue) PeekMin(ctx context.Context, limit int) ([]QueueItem, error) {
	if !q.available {
		return nil, errors.New("primary down")
	}
	return q.queue.PeekMin(ctx, limit)
}

func (q *flakyQueue) PopMin(ctx context.Context) (QueueItem, error) {
	if !q.available {
		return QueueItem{}, errors.New("primary down")
	}
	return q.queue.PopMin(ctx)
}

func (q *flakyQueue) Remove(ctx context.Context, taskID string) error {
	if !q.available {
		return errors.New("primary down")
	}
	return q.queue.Remove(ctx, taskID)
}

func (q *flakyQueue) Len(ctx context.Context) (int64, error) {
	if !q.available {
		return 0, errors.New("primary down")
	}
	return q.queue.Len(ctx)
}
