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
