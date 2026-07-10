package scheduler

import (
	"context"
	"errors"
	"testing"
)

func TestQueueGuardHardLimitRejectsAllPriorities(t *testing.T) {
	ctx := context.Background()
	for _, priority := range []PriorityClass{PriorityHigh, PriorityNormal, PriorityLow} {
		q := NewMemoryQueue()
		if err := q.Push(ctx, QueueItem{TaskID: string(priority), Score: 1}); err != nil {
			t.Fatalf("Push: %v", err)
		}
		got := (QueueGuard{HardLimit: 1}).Check(ctx, q, priority)
		if !errors.Is(got.Err, ErrQueueFull) {
			t.Fatalf("priority %s bypassed hard limit: %#v", priority, got)
		}
	}
}

func TestQueueGuardSoftLimitThrottlesBeforeHardLimit(t *testing.T) {
	ctx := context.Background()
	q := NewMemoryQueue()
	if err := q.Push(ctx, QueueItem{TaskID: "t1", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	got := (QueueGuard{SoftLimit: 1, HardLimit: 2}).Check(ctx, q, PriorityNormal)
	if !got.Allowed || !got.Throttled || got.Err != nil {
		t.Fatalf("expected throttled allowance, got %#v", got)
	}
}
