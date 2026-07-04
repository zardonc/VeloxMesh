package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestResultRegistryDeliverReturnsFalseAfterUnregister(t *testing.T) {
	registry := NewResultRegistry()
	registry.Register("t1")
	registry.Unregister("t1")

	if registry.Deliver("t1", TaskResult{Response: "done"}) {
		t.Fatalf("expected Deliver to return false after unregister")
	}
}

func TestResultRegistryDeliverDoesNotBlockAfterWaitTimeout(t *testing.T) {
	registry := NewResultRegistry()
	registry.Register("t1")

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := registry.Wait(ctx, "t1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	done := make(chan bool, 1)
	go func() {
		done <- registry.Deliver("t1", TaskResult{Response: "late"})
	}()
	select {
	case delivered := <-done:
		if !delivered {
			t.Fatalf("expected buffered late result to deliver once")
		}
	case <-time.After(time.Second):
		t.Fatalf("Deliver blocked after wait timeout")
	}
}

func TestResultRegistryWaitReturnsContextCancellation(t *testing.T) {
	registry := NewResultRegistry()
	registry.Register("t1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := registry.Wait(ctx, "t1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestTaskStateVocabulary(t *testing.T) {
	states := []TaskState{TaskStateQueued, TaskStateRunning, TaskStateCompleted, TaskStateCanceled, TaskStateFailed}
	for _, state := range states {
		if state == "" {
			t.Fatalf("empty task state")
		}
	}
}
