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

func TestMemoryQueuePeekMinDoesNotMutateAndPushReplacesScore(t *testing.T) {
	ctx := context.Background()
	q := NewMemoryQueue()
	for _, item := range []QueueItem{
		{TaskID: "later", Score: 3},
		{TaskID: "first", Score: 2},
		{TaskID: "later", Score: 1},
	} {
		if err := q.Push(ctx, item); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	items, err := q.PeekMin(ctx, 2)
	if err != nil {
		t.Fatalf("PeekMin: %v", err)
	}
	if len(items) != 2 || items[0].TaskID != "later" || items[0].Score != 1 || items[1].TaskID != "first" {
		t.Fatalf("unexpected peek order: %#v", items)
	}
	length, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 2 {
		t.Fatalf("PeekMin mutated queue length to %d", length)
	}
	if empty, err := q.PeekMin(ctx, 0); err != nil || len(empty) != 0 {
		t.Fatalf("PeekMin limit 0 = %#v, %v; want empty nil", empty, err)
	}
	for _, want := range []string{"later", "first"} {
		got, err := q.PopMin(ctx)
		if err != nil {
			t.Fatalf("PopMin: %v", err)
		}
		if got.TaskID != want {
			t.Fatalf("got %s, want %s", got.TaskID, want)
		}
	}
}

func TestMemoryQueuePeekMinBoundedKeepsScoreAndFIFOOrder(t *testing.T) {
	ctx := context.Background()
	q := NewMemoryQueue()
	for _, item := range []QueueItem{
		{TaskID: "five", Score: 5},
		{TaskID: "first-one", Score: 1},
		{TaskID: "three", Score: 3},
		{TaskID: "second-one", Score: 1},
		{TaskID: "two", Score: 2},
	} {
		if err := q.Push(ctx, item); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	items, err := q.PeekMin(ctx, 3)
	if err != nil {
		t.Fatalf("PeekMin: %v", err)
	}
	want := []string{"first-one", "second-one", "two"}
	for i, item := range items {
		if item.TaskID != want[i] {
			t.Fatalf("peek[%d] = %s, want %s; all=%#v", i, item.TaskID, want[i], items)
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
