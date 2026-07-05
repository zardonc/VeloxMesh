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

func TestResultRegistryRegisterTaskStoresSafeSnapshot(t *testing.T) {
	registry := NewResultRegistry()
	enqueue := time.Now().UTC()
	task := Task{
		ID:          "t1",
		TenantID:    "tenant-a",
		TenantClass: "gold",
		Feature: TaskFeature{
			ModelClass:    "large",
			RequestKind:   RequestKindCodeGen,
			Priority:      PriorityNormal,
			EnqueueTimeMs: enqueue.UnixMilli(),
		},
		EnqueueTime: enqueue,
		Metadata:    map[string]string{"scheduler_type": "fifo"},
	}
	registry.RegisterTask(task, func(context.Context) TaskResult { return TaskResult{} })

	got, ok := registry.Task("t1")
	if !ok {
		t.Fatalf("expected task snapshot")
	}
	if got.TenantID != "tenant-a" || got.TenantClass != "gold" {
		t.Fatalf("unexpected tenant snapshot: %#v", got)
	}
	if got.Feature.ModelClass != "large" || got.Feature.RequestKind != RequestKindCodeGen || got.Feature.Priority != PriorityNormal {
		t.Fatalf("unexpected safe feature snapshot: %#v", got.Feature)
	}
	if !got.EnqueueTime.Equal(enqueue) || got.Feature.EnqueueTimeMs != enqueue.UnixMilli() {
		t.Fatalf("unexpected enqueue time snapshot: %#v", got)
	}
	got.Metadata["scheduler_type"] = "mutated"
	again, _ := registry.Task("t1")
	if again.Metadata["scheduler_type"] != "fifo" {
		t.Fatalf("task snapshot metadata was mutated: %#v", again.Metadata)
	}
}

func TestResultRegistryUnregisterRemovesTaskSnapshot(t *testing.T) {
	registry := NewResultRegistry()
	registry.RegisterTask(Task{ID: "t1"}, func(context.Context) TaskResult { return TaskResult{} })
	registry.Unregister("t1")
	if _, ok := registry.Task("t1"); ok {
		t.Fatalf("expected task snapshot removed")
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
