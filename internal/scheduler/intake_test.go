package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
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

func TestTaskIntakeScorerErrorRecordsOneSchedulerCall(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	metrics := &schedulerMetricsSpy{StubMetrics: observability.NewStubMetrics()}
	intake := &TaskIntake{
		Queue: queue, Scorer: errorScorer{}, Registry: registry, Metrics: metrics,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if metrics.schedulerCalls != 1 || metrics.schedulerCallResult != "error" {
		t.Fatalf("scheduler calls = %d/%q, want 1/error", metrics.schedulerCalls, metrics.schedulerCallResult)
	}
	if metrics.schedulerErrors != 1 || metrics.schedulerErrorReason != "error" {
		t.Fatalf("scheduler errors = %d/%q, want 1/error", metrics.schedulerErrors, metrics.schedulerErrorReason)
	}
}

func TestTaskIntakeSemanticNeighborsRunBeforeScoring(t *testing.T) {
	events := []string{}
	scorer := &captureScorer{events: &events}
	intake := semanticNeighborIntake(scorer, &captureEnricher{events: &events})

	task, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(events) != 2 || events[0] != "enrich" || events[1] != "score" {
		t.Fatalf("unexpected ordering: %v", events)
	}
	if scorer.feature.NeighborCount != 2 || scorer.feature.CoverageLevel != SemanticCoverageTenant {
		t.Fatalf("scorer saw unenriched feature: %#v", scorer.feature)
	}
	if task.Feature.NeighborCount != 2 {
		t.Fatalf("queued task lost enriched feature: %#v", task.Feature)
	}
}

func TestTaskIntakeSemanticNeighborErrorFailsOpen(t *testing.T) {
	scorer := &captureScorer{}
	intake := semanticNeighborIntake(scorer, errorEnricher{})

	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if scorer.feature.NeighborCount != 0 || scorer.feature.CoverageLevel != SemanticCoverageNone {
		t.Fatalf("expected neutral scorer feature after enrichment error: %#v", scorer.feature)
	}
}

func TestTaskIntakeSemanticNeighborTimeoutFailsOpen(t *testing.T) {
	scorer := &captureScorer{}
	intake := semanticNeighborIntake(scorer, blockingEnricher{})
	intake.SemanticNeighborTaskTimeout = time.Millisecond

	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "t1"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if scorer.feature.NeighborCount != 0 || scorer.feature.CoverageLevel != SemanticCoverageNone {
		t.Fatalf("expected neutral scorer feature after enrichment timeout: %#v", scorer.feature)
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

type errorScorer struct{}

func (errorScorer) Score(context.Context, []TaskFeature) ([]ScoreResult, error) {
	return nil, errors.New("scheduler unavailable")
}

func semanticNeighborIntake(scorer Scorer, enricher SemanticNeighborEnricher) *TaskIntake {
	return &TaskIntake{
		Queue: NewMemoryQueue(), Scorer: scorer, Registry: NewResultRegistry(),
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh},
		Backend: "memory", Metrics: observability.NewStubMetrics(), SemanticNeighbors: enricher,
	}
}

type captureScorer struct {
	events  *[]string
	feature TaskFeature
}

func (s *captureScorer) Score(_ context.Context, tasks []TaskFeature) ([]ScoreResult, error) {
	if s.events != nil {
		*s.events = append(*s.events, "score")
	}
	s.feature = tasks[0]
	return []ScoreResult{{TaskID: tasks[0].TaskID, Score: 1, Priority: tasks[0].Priority}}, nil
}

type captureEnricher struct {
	events *[]string
}

func (e *captureEnricher) Enrich(_ context.Context, _ *llm.LLMRequest, feature TaskFeature) (TaskFeature, error) {
	*e.events = append(*e.events, "enrich")
	feature.NeighborCount = 2
	feature.CoverageLevel = SemanticCoverageTenant
	return feature, nil
}

type errorEnricher struct{}

func (errorEnricher) Enrich(context.Context, *llm.LLMRequest, TaskFeature) (TaskFeature, error) {
	return TaskFeature{}, errors.New("enrichment failed")
}

type blockingEnricher struct{}

func (blockingEnricher) Enrich(ctx context.Context, _ *llm.LLMRequest, feature TaskFeature) (TaskFeature, error) {
	<-ctx.Done()
	return feature, ctx.Err()
}

type schedulerMetricsSpy struct {
	*observability.StubMetrics
	schedulerCalls       int
	schedulerCallResult  string
	schedulerErrors      int
	schedulerErrorReason string
}

func (m *schedulerMetricsSpy) RecordSchedulerCall(result string, _ float64) {
	m.schedulerCalls++
	m.schedulerCallResult = result
}

func (m *schedulerMetricsSpy) IncSchedulerError(reason string) {
	m.schedulerErrors++
	m.schedulerErrorReason = reason
}

type recordingQueue struct {
	QueueBackend
	removed string
}

func (q *recordingQueue) Remove(ctx context.Context, taskID string) error {
	q.removed = taskID
	return q.QueueBackend.Remove(ctx, taskID)
}
