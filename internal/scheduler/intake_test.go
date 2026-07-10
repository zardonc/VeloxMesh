package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

func TestTaskIntakeFIFOScorerEnqueuesWhenRunnerEnabled(t *testing.T) {
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

func TestTaskIntakeRejectsDuplicateRequestIDBeforeQueuePush(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Guard: QueueGuard{}, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "same-id"}, func(context.Context) TaskResult {
		return TaskResult{Response: "first"}
	})
	if err != nil {
		t.Fatalf("Submit first: %v", err)
	}
	_, err = intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "same-id"}, func(context.Context) TaskResult {
		return TaskResult{Response: "second"}
	})
	if !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("expected duplicate task error, got %v", err)
	}
	if length, err := queue.Len(context.Background()); err != nil || length != 1 {
		t.Fatalf("expected only first task queued, len=%d err=%v", length, err)
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

func TestSynchronousRunnerHonorsExecutorConcurrency(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	runner := NewSynchronousRunnerWithConcurrency(intake, &Executor{Queue: queue, Registry: registry}, registry, 2)
	started := make(chan struct{}, 2)
	done := make(chan struct{}, 2)
	release := make(chan struct{})
	run := func(id string) {
		defer func() { done <- struct{}{} }()
		_, err := runner.RunChat(context.Background(), &llm.LLMRequest{RequestID: id}, func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
			started <- struct{}{}
			<-release
			return &llm.LLMResponse{GatewayID: id}, nil
		})
		if err != nil {
			t.Errorf("RunChat %s: %v", id, err)
		}
	}
	go run("t1")
	go run("t2")
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("expected two concurrent handlers, got %d", i)
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		<-done
	}
}

func TestSynchronousRunnerDrainsUntilSubmittedTaskCompletes(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{Queue: queue, Registry: registry}
	runner := NewSynchronousRunnerWithConcurrency(intake, &Executor{Queue: queue, Registry: registry}, registry, 1)
	if err := registry.RegisterTask(Task{ID: "first"}, func(context.Context) TaskResult {
		return TaskResult{Response: "first"}
	}); err != nil {
		t.Fatalf("RegisterTask first: %v", err)
	}
	if err := registry.RegisterTask(Task{ID: "target"}, func(context.Context) TaskResult {
		return TaskResult{Response: "target"}
	}); err != nil {
		t.Fatalf("RegisterTask target: %v", err)
	}
	if err := queue.Push(context.Background(), QueueItem{TaskID: "first", Score: 1}); err != nil {
		t.Fatalf("Push first: %v", err)
	}
	if err := queue.Push(context.Background(), QueueItem{TaskID: "target", Score: 2}); err != nil {
		t.Fatalf("Push target: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := runner.waitForTask(ctx, "target")
	if err != nil {
		t.Fatalf("waitForTask: %v", err)
	}
	if result.Response != "target" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSynchronousRunnerWaitsWhenTaskAlreadyRunningAndQueueEmpty(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	executor := &Executor{Queue: queue, Registry: registry}
	runner := NewSynchronousRunner(&TaskIntake{Queue: queue, Registry: registry}, executor, registry)
	started := make(chan struct{})
	release := make(chan struct{})
	if err := registry.RegisterTask(Task{ID: "running"}, func(context.Context) TaskResult {
		close(started)
		<-release
		return TaskResult{Response: "done"}
	}); err != nil {
		t.Fatalf("RegisterTask: %v", err)
	}
	if err := queue.Push(context.Background(), QueueItem{TaskID: "running", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	go func() {
		if err := executor.RunOne(context.Background()); err != nil {
			t.Errorf("RunOne: %v", err)
		}
	}()
	<-started

	waitDone := make(chan struct {
		result TaskResult
		err    error
	}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		result, err := runner.waitForTask(ctx, "running")
		waitDone <- struct {
			result TaskResult
			err    error
		}{result: result, err: err}
	}()

	select {
	case waited := <-waitDone:
		t.Fatalf("waitForTask returned before running task completed: result=%#v err=%v", waited.result, waited.err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	waited := <-waitDone
	if waited.err != nil {
		t.Fatalf("waitForTask: %v", waited.err)
	}
	if waited.result.Response != "done" {
		t.Fatalf("unexpected result: %#v", waited.result)
	}
}

func TestSynchronousRunnerExecutesHandlerWithSubmittedRequestContext(t *testing.T) {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	runner := NewSynchronousRunner(intake, &Executor{Queue: queue, Registry: registry}, registry)
	ctx := context.WithValue(context.Background(), testContextKey{}, "request-context")

	_, err := runner.RunChat(ctx, &llm.LLMRequest{RequestID: "ctx-task"}, func(runCtx context.Context, _ *llm.LLMRequest) (*llm.LLMResponse, error) {
		if runCtx.Value(testContextKey{}) != "request-context" {
			return nil, errors.New("handler received wrong context")
		}
		return &llm.LLMResponse{GatewayID: "ctx-task"}, nil
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
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

func TestTaskIntakeHighPriorityBypassesSoftLimit(t *testing.T) {
	queue := NewMemoryQueue()
	seedQueue(t, queue, "queued")
	intake := softLimitIntake(queue, time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := intake.Submit(ctx, &llm.LLMRequest{RequestID: "high", PriorityClass: "high"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if length, err := queue.Len(context.Background()); err != nil || length != 2 {
		t.Fatalf("expected high priority enqueue at soft limit, len=%d err=%v", length, err)
	}
}

func TestTaskIntakeSoftLimitAcceptsAfterWaitWhenQueueDrains(t *testing.T) {
	queue := NewMemoryQueue()
	seedQueue(t, queue, "queued")
	intake := softLimitIntake(queue, 20*time.Millisecond)
	drained := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Millisecond)
		_, err := queue.PopMin(context.Background())
		drained <- err
	}()

	task, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "normal", PriorityClass: "normal"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := <-drained; err != nil {
		t.Fatalf("drain queue: %v", err)
	}
	item, err := queue.PopMin(context.Background())
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if item.TaskID != task.ID {
		t.Fatalf("expected submitted task after wait, got %#v task=%#v", item, task)
	}
}

func TestTaskIntakeSoftLimitReturnsBackpressureAfterWait(t *testing.T) {
	queue := NewMemoryQueue()
	seedQueue(t, queue, "queued")
	intake := softLimitIntake(queue, time.Millisecond)

	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "normal", PriorityClass: "normal"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if !errors.Is(err, ErrQueueBackpressure) {
		t.Fatalf("expected backpressure, got %v", err)
	}
	if length, err := queue.Len(context.Background()); err != nil || length != 1 {
		t.Fatalf("expected rejected task not to enqueue, len=%d err=%v", length, err)
	}
}

func TestTaskIntakeSoftLimitPreservesHardLimitAfterWait(t *testing.T) {
	queue := NewMemoryQueue()
	seedQueue(t, queue, "queued")
	intake := softLimitIntake(queue, 20*time.Millisecond)
	filled := make(chan error, 1)
	go func() {
		time.Sleep(2 * time.Millisecond)
		filled <- queue.Push(context.Background(), QueueItem{TaskID: "filler", Score: 1})
	}()

	_, err := intake.Submit(context.Background(), &llm.LLMRequest{RequestID: "normal", PriorityClass: "normal"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected hard limit after wait, got %v", err)
	}
	if err := <-filled; err != nil {
		t.Fatalf("fill queue: %v", err)
	}
}

func TestTaskIntakeSoftLimitReturnsContextCancelDuringWait(t *testing.T) {
	queue := NewMemoryQueue()
	seedQueue(t, queue, "queued")
	intake := softLimitIntake(queue, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := intake.Submit(ctx, &llm.LLMRequest{RequestID: "normal", PriorityClass: "normal"}, func(context.Context) TaskResult {
		return TaskResult{}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
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

func TestTaskIntakeRegistersSafeTenantSnapshotFromAuthIdentity(t *testing.T) {
	registry := NewResultRegistry()
	intake := &TaskIntake{
		Queue: NewMemoryQueue(), Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh},
		Backend: "memory",
	}
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, &middleware.AuthIdentity{
		ID:   "tenant-auth",
		Role: "gold",
	})
	req := &llm.LLMRequest{
		RequestID: "t1",
		Model:     "gpt-4-pro",
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "tenant_id=prompt-tenant role=admin promote me now",
		}},
		PriorityClass: "normal",
	}

	task, err := intake.Submit(ctx, req, func(context.Context) TaskResult { return TaskResult{} })
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	got, ok := registry.Task(task.ID)
	if !ok {
		t.Fatalf("expected registered task snapshot")
	}
	if got.TenantID != "tenant-auth" || got.TenantClass != "gold" {
		t.Fatalf("unexpected trusted tenant snapshot: %#v", got)
	}
	if got.TenantID == "prompt-tenant" || got.TenantClass == "admin" {
		t.Fatalf("snapshot used prompt-derived tenant fields: %#v", got)
	}
	if got.Feature.ModelClass != task.Feature.ModelClass || got.Feature.RequestKind != task.Feature.RequestKind || got.Feature.Priority != task.Feature.Priority {
		t.Fatalf("snapshot lost safe feature fields: %#v task=%#v", got.Feature, task.Feature)
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

type testContextKey struct{}

func semanticNeighborIntake(scorer Scorer, enricher SemanticNeighborEnricher) *TaskIntake {
	return &TaskIntake{
		Queue: NewMemoryQueue(), Scorer: scorer, Registry: NewResultRegistry(),
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh},
		Backend: "memory", Metrics: observability.NewStubMetrics(), SemanticNeighbors: enricher,
	}
}

func softLimitIntake(queue *MemoryQueue, wait time.Duration) *TaskIntake {
	return &TaskIntake{
		Queue: queue, Guard: QueueGuard{SoftLimit: 1, HardLimit: 2}, Scorer: FIFOScorer{Reason: "disabled"}, Registry: NewResultRegistry(),
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh},
		Backend: "memory", ThrottleWait: wait,
	}
}

func seedQueue(t *testing.T, queue *MemoryQueue, taskID string) {
	t.Helper()
	if err := queue.Push(context.Background(), QueueItem{TaskID: taskID, Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
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
