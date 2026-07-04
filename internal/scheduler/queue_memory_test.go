package scheduler

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryQueuePopMinScoreAndFIFO(t *testing.T) {
	ctx := context.Background()
	q := NewMemoryQueue()
	for _, item := range []QueueItem{
		{TaskID: "later", Score: 2},
		{TaskID: "first", Score: 1},
		{TaskID: "second", Score: 1},
	} {
		if err := q.Push(ctx, item); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	for _, want := range []string{"first", "second", "later"} {
		got, err := q.PopMin(ctx)
		if err != nil {
			t.Fatalf("PopMin: %v", err)
		}
		if got.TaskID != want {
			t.Fatalf("got %s, want %s", got.TaskID, want)
		}
	}
}

func TestMemoryQueueRemove(t *testing.T) {
	ctx := context.Background()
	q := NewMemoryQueue()
	if err := q.Push(ctx, QueueItem{TaskID: "t1", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := q.Remove(ctx, "t1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := q.Remove(ctx, "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
	if _, err := q.PopMin(ctx); !errors.Is(err, ErrQueueEmpty) {
		t.Fatalf("expected empty queue, got %v", err)
	}
}
