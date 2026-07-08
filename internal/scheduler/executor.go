package scheduler

import (
	"context"
	"errors"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

type Executor struct {
	Queue    QueueBackend
	Registry *ResultRegistry
	Metrics  observability.Metrics
	Promoter *SLAPromoter
}

func (e *Executor) RunOne(ctx context.Context) error {
	if e.Promoter != nil {
		_, _ = e.Promoter.PromoteBeforePop(ctx, time.Now())
	}
	item, err := e.Queue.PopMin(ctx)
	if err != nil {
		return err
	}
	e.Registry.MarkRunning(item.TaskID)
	handler, ok := e.Registry.Handler(item.TaskID)
	if !ok {
		return ErrTaskNotFound
	}
	result := handler(ctx)
	returned := e.Registry.Deliver(item.TaskID, result)
	if !returned && result.Error != nil {
		return result.Error
	}
	return nil
}

func (e *Executor) Cancel(ctx context.Context, taskID string) {
	_ = e.Queue.Remove(ctx, taskID)
	e.Registry.Unregister(taskID)
}

type SynchronousRunner struct {
	Intake   *TaskIntake
	Executor *Executor
	Registry *ResultRegistry
	Recorder *TrainingRecorder
	Quality  *PredictionQualityRecorder
	Indexer  SemanticNeighborIndexer
	slots    chan struct{}
}

func NewSynchronousRunner(intake *TaskIntake, executor *Executor, registry *ResultRegistry) *SynchronousRunner {
	return NewSynchronousRunnerWithConcurrency(intake, executor, registry, 1)
}

func NewSynchronousRunnerWithConcurrency(intake *TaskIntake, executor *Executor, registry *ResultRegistry, concurrency int) *SynchronousRunner {
	if concurrency < 1 {
		concurrency = 1
	}
	return &SynchronousRunner{Intake: intake, Executor: executor, Registry: registry, slots: make(chan struct{}, concurrency)}
}

func (r *SynchronousRunner) SlotUsage() (used int, total int, ok bool) {
	if r == nil || r.slots == nil {
		return 0, 0, false
	}
	return len(r.slots), cap(r.slots), true
}

func (r *SynchronousRunner) RunChat(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error)) (*llm.LLMResponse, error) {
	start := time.Now()
	task, err := r.Intake.Submit(ctx, req, func(context.Context) TaskResult {
		resp, err := execute(ctx, req)
		return TaskResult{Response: resp, Error: err}
	})
	if err != nil {
		return nil, err
	}
	defer r.Registry.Unregister(task.ID)
	result, err := r.waitForTask(ctx, task.ID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			r.recordCompletionEvidence(ctx, req, task, start, nil, TrainingOutcomeTimeout)
		}
		r.Executor.Cancel(context.Background(), task.ID)
		return nil, err
	}
	if result.Error != nil {
		r.recordCompletionEvidence(ctx, req, task, start, nil, TrainingOutcomeFailure)
		return nil, result.Error
	}
	resp, _ := result.Response.(*llm.LLMResponse)
	if resp != nil {
		resp.QueueWaitMs = time.Since(start).Milliseconds()
	}
	r.recordWait(task, start)
	r.recordCompletionEvidence(ctx, req, task, start, resp, TrainingOutcomeSuccess)
	return resp, nil
}

type StreamResult struct {
	Events   <-chan llm.StreamEvent
	Response *llm.LLMResponse
}

type taskWaitResult struct {
	result TaskResult
	err    error
}

func (r *SynchronousRunner) RunStream(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error)) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	start := time.Now()
	task, err := r.Intake.Submit(ctx, req, func(context.Context) TaskResult {
		events, resp, err := execute(ctx, req)
		return TaskResult{Response: StreamResult{Events: events, Response: resp}, Error: err}
	})
	if err != nil {
		return nil, nil, err
	}
	defer r.Registry.Unregister(task.ID)
	result, err := r.waitForTask(ctx, task.ID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			r.recordCompletionEvidence(ctx, req, task, start, nil, TrainingOutcomeTimeout)
		}
		r.Executor.Cancel(context.Background(), task.ID)
		return nil, nil, err
	}
	if result.Error != nil {
		r.recordCompletionEvidence(ctx, req, task, start, nil, TrainingOutcomeFailure)
		return nil, nil, result.Error
	}
	stream, _ := result.Response.(StreamResult)
	if stream.Response == nil {
		stream.Response = &llm.LLMResponse{}
	}
	if stream.Response != nil {
		stream.Response.QueueWaitMs = time.Since(start).Milliseconds()
	}
	r.recordWait(task, start)
	r.recordCompletionEvidence(ctx, req, task, start, stream.Response, TrainingOutcomeSuccess)
	return stream.Events, stream.Response, nil
}

func (r *SynchronousRunner) waitForTask(ctx context.Context, taskID string) (TaskResult, error) {
	waitDone := make(chan taskWaitResult, 1)
	go func() {
		result, err := r.Registry.Wait(ctx, taskID)
		waitDone <- taskWaitResult{result: result, err: err}
	}()

	for {
		select {
		case waited := <-waitDone:
			return waited.result, waited.err
		case r.slots <- struct{}{}:
		case <-ctx.Done():
			return TaskResult{}, ctx.Err()
		}
		err := r.Executor.RunOne(ctx)
		<-r.slots
		if err != nil {
			if errors.Is(err, ErrQueueEmpty) {
				select {
				case waited := <-waitDone:
					return waited.result, waited.err
				default:
				}
				if !r.Registry.IsRunning(taskID) {
					return TaskResult{}, err
				}
				return waitForRegistryResult(ctx, waitDone)
			}
			return TaskResult{}, err
		}
		select {
		case waited := <-waitDone:
			return waited.result, waited.err
		default:
		}
		depth, err := r.Executor.Queue.Len(ctx)
		if err != nil {
			return TaskResult{}, err
		}
		if depth == 0 {
			if !r.Registry.IsRunning(taskID) {
				return TaskResult{}, ErrQueueEmpty
			}
			return waitForRegistryResult(ctx, waitDone)
		}
	}
}

func waitForRegistryResult(ctx context.Context, waitDone <-chan taskWaitResult) (TaskResult, error) {
	select {
	case waited := <-waitDone:
		return waited.result, waited.err
	case <-ctx.Done():
		return TaskResult{}, ctx.Err()
	}
}

func (r *SynchronousRunner) recordWait(task Task, start time.Time) {
	if r.Intake.Metrics != nil {
		r.Intake.Metrics.RecordTaskWait(string(task.Feature.Priority), float64(time.Since(start).Milliseconds()))
	}
}

func (r *SynchronousRunner) recordCompletionEvidence(ctx context.Context, req *llm.LLMRequest, task Task, start time.Time, resp *llm.LLMResponse, outcome string) {
	labels := trainingLabels(start, resp, outcome)
	sampleID := r.recordTrainingSample(ctx, task, labels)
	r.indexCompletedSample(ctx, req, task, labels, sampleID)
	if r.Quality == nil {
		return
	}
	if err := r.Quality.Record(ctx, task, labels, sampleID); err != nil && r.Intake.Metrics != nil {
		r.Intake.Metrics.IncSchedulerError("feedback")
	}
}

func (r *SynchronousRunner) recordTrainingSample(ctx context.Context, task Task, labels TrainingLabels) string {
	if r.Recorder == nil {
		return ""
	}
	sampleID, err := r.Recorder.Record(ctx, task, labels)
	if err != nil && r.Intake.Metrics != nil {
		r.Intake.Metrics.IncSchedulerError("feedback")
	}
	return sampleID
}

func (r *SynchronousRunner) indexCompletedSample(ctx context.Context, req *llm.LLMRequest, task Task, labels TrainingLabels, sampleID string) {
	if r.Indexer == nil || sampleID == "" {
		return
	}
	if err := r.Indexer.IndexCompletedSample(ctx, req, task, labels, sampleID); err != nil && r.Intake.Metrics != nil {
		r.Intake.Metrics.IncSemanticNeighborError("index")
	}
}

func trainingLabels(start time.Time, resp *llm.LLMResponse, outcome string) TrainingLabels {
	labels := TrainingLabels{ActualLatencyMs: time.Since(start).Milliseconds(), Outcome: outcome, CompletedAt: time.Now().UTC()}
	if resp == nil {
		return labels
	}
	labels.ProviderClass = resp.Provider
	if resp.Usage != nil {
		labels.InputTokens = int64(resp.Usage.PromptTokens)
		labels.OutputTokens = int64(resp.Usage.CompletionTokens)
	}
	return labels
}
