package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

func TestTaskIntakeDisabledSchedulerEnqueuesFIFO(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Guard: QueueGuard{}, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	task, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1", PriorityClass: "normal"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	item, err := queue.PopMin(context.Background())
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if item.TaskID != task.ID || item.Score == 0 {
		t.Fatalf("unexpected queued item: %#v task=%#v", item, task)
	}
}

func TestSynchronousRunnerReturnsResponse(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	runner := NewSynchronousRunner(intake, &Executor{Queue: queue, Registry: registry}, registry)
	resp, err := runner.RunChat(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		return &llm.LLMResponse{GatewayID: "t1"}, nil
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if resp.GatewayID != "t1" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestExecutorCancelRemovesAndUnregisters(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	registry.Register("t1")
	if err := queue.Push(context.Background(), QueueItem{TaskID: "t1", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	executor := &Executor{Queue: queue, Registry: registry}
	executor.Cancel(context.Background(), "t1")
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := registry.Wait(ctx, "t1"); err != ErrTaskNotFound {
		t.Fatalf("expected registry cleanup, got %v", err)
	}
}

func TestTaskIntakeEmptySchedulerResultFallsBackToFIFO(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: emptyScorer{}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	task, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if task.Score == 0 {
		t.Fatalf("expected FIFO fallback score, got %#v", task)
	}
}

func TestSynchronousRunnerCancelRemovesQueuedTask(t *testing.T) {
	registry := NewResultRegistry()
	queue := &recordingQueue{QueueBackend: NewMemoryQueue()}
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	runner := NewSynchronousRunner(intake, &Executor{Queue: queue, Registry: registry}, registry)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runner.RunChat(ctx, &llm.LLMRequest{RequestID: "t1"}, func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		time.Sleep(20 * time.Millisecond)
		return &llm.LLMResponse{}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if queue.removed != "t1" {
		t.Fatalf("expected queue remove for t1, got %q", queue.removed)
	}
	if _, err := registry.Wait(context.Background(), "t1"); err != ErrTaskNotFound {
		t.Fatalf("expected registry unregister, got %v", err)
	}
}

type emptyScorer struct{}

func (emptyScorer) Score(context.Context, []TaskFeature) ([]ScoreResult, error) {
	return nil, nil
}

type recordingQueue struct {
	QueueBackend
	removed string
}

func (q *recordingQueue) Remove(ctx context.Context, taskID string) error {
	q.removed = taskID
	return q.QueueBackend.Remove(ctx, taskID)
}
