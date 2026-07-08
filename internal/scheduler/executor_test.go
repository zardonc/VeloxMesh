package scheduler_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/scheduler"
)

// slowQueue Backend adds a delay between popping the item and returning it,
// maximizing the chance of hitting the MarkRunning race condition.
type slowQueue struct {
	backend scheduler.QueueBackend
}

func (q *slowQueue) Push(ctx context.Context, item scheduler.QueueItem) error {
	return q.backend.Push(ctx, item)
}

func (q *slowQueue) PeekMin(ctx context.Context, limit int) ([]scheduler.QueueItem, error) {
	return q.backend.PeekMin(ctx, limit)
}

func (q *slowQueue) PopMin(ctx context.Context) (scheduler.QueueItem, error) {
	item, err := q.backend.PopMin(ctx)
	if err == nil {
		// Delay to allow another goroutine to check queue length or call RunOne
		// BEFORE the original goroutine can call MarkRunning.
		time.Sleep(10 * time.Millisecond)
	}
	return item, err
}

func (q *slowQueue) Remove(ctx context.Context, taskID string) error {
	return q.backend.Remove(ctx, taskID)
}

func (q *slowQueue) Len(ctx context.Context) (int64, error) {
	return q.backend.Len(ctx)
}

func TestExecutorRaceCondition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	queue := &slowQueue{backend: scheduler.NewMemoryQueue()}
	registry := scheduler.NewResultRegistry()
	scorer := scheduler.FIFOScorer{Reason: "test"}
	intake := &scheduler.TaskIntake{
		Queue:    queue,
		Guard:    scheduler.QueueGuard{SoftLimit: 100, HardLimit: 100},
		Scorer:   scorer,
		Registry: registry,
		Priority: scheduler.NewPriorityResolver(nil),
		Policy:   scheduler.PriorityPolicy{},
	}
	executor := &scheduler.Executor{
		Queue:    queue,
		Registry: registry,
	}

	// Concurrency 2 is required to trigger the race condition
	runner := scheduler.NewSynchronousRunnerWithConcurrency(intake, executor, registry, 2)

	var successCount int32
	var wg sync.WaitGroup

	// Run 10 concurrent requests
	numRequests := 10
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()
			req := &llm.LLMRequest{
				RequestID: "test-req-" + string(rune(id)),
			}
			_, err := runner.RunChat(ctx, req, func(ctx context.Context, r *llm.LLMRequest) (*llm.LLMResponse, error) {
				// Simulate some work
				time.Sleep(20 * time.Millisecond)
				return &llm.LLMResponse{}, nil
			})
			if err != nil {
				if errors.Is(err, scheduler.ErrQueueEmpty) {
					t.Errorf("Race condition triggered: task dropped with ErrQueueEmpty")
				} else {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if successCount != int32(numRequests) {
		t.Fatalf("Expected %d successes, got %d", numRequests, successCount)
	}
}

func TestExecutorRunOneDeliversPanicToTaskOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	queue := scheduler.NewMemoryQueue()
	registry := scheduler.NewResultRegistry()
	task := scheduler.Task{ID: "panic-task", Feature: scheduler.TaskFeature{TaskID: "panic-task"}}
	registry.RegisterTask(task, func(context.Context) scheduler.TaskResult {
		panic("boom")
	})
	if err := queue.Push(ctx, scheduler.QueueItem{TaskID: task.ID, Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	executor := &scheduler.Executor{Queue: queue, Registry: registry}

	if err := executor.RunOne(ctx); err != nil {
		t.Fatalf("RunOne should deliver panic to owner, got %v", err)
	}
	result, err := registry.Wait(ctx, task.ID)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "boom") {
		t.Fatalf("panic was not delivered to owner: %#v", result)
	}
}
