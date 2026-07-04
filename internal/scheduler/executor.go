package scheduler

import (
	"context"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

type Executor struct {
	Queue    QueueBackend
	Registry *ResultRegistry
	Metrics  observability.Metrics
}

func (e *Executor) RunOne(ctx context.Context) error {
	item, err := e.Queue.PopMin(ctx)
	if err != nil {
		return err
	}
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
}

func NewSynchronousRunner(intake *TaskIntake, executor *Executor, registry *ResultRegistry) *SynchronousRunner {
	return &SynchronousRunner{Intake: intake, Executor: executor, Registry: registry}
}

func (r *SynchronousRunner) RunChat(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error)) (*llm.LLMResponse, error) {
	start := time.Now()
	task, err := r.Intake.Submit(ctx, req, func(runCtx context.Context) TaskResult {
		resp, err := execute(runCtx, req)
		return TaskResult{Response: resp, Error: err}
	})
	if err != nil {
		return nil, err
	}
	defer r.Registry.Unregister(task.ID)
	result, err := r.waitForTask(ctx, task.ID)
	if err != nil {
		r.Executor.Cancel(context.Background(), task.ID)
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	resp, _ := result.Response.(*llm.LLMResponse)
	if resp != nil {
		resp.QueueWaitMs = time.Since(start).Milliseconds()
	}
	r.recordWait(task, start)
	return resp, nil
}

type StreamResult struct {
	Events   <-chan llm.StreamEvent
	Response *llm.LLMResponse
}

func (r *SynchronousRunner) RunStream(ctx context.Context, req *llm.LLMRequest, execute func(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, *llm.LLMResponse, error)) (<-chan llm.StreamEvent, *llm.LLMResponse, error) {
	start := time.Now()
	task, err := r.Intake.Submit(ctx, req, func(runCtx context.Context) TaskResult {
		events, resp, err := execute(runCtx, req)
		return TaskResult{Response: StreamResult{Events: events, Response: resp}, Error: err}
	})
	if err != nil {
		return nil, nil, err
	}
	defer r.Registry.Unregister(task.ID)
	result, err := r.waitForTask(ctx, task.ID)
	if err != nil {
		r.Executor.Cancel(context.Background(), task.ID)
		return nil, nil, err
	}
	if result.Error != nil {
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
	return stream.Events, stream.Response, nil
}

func (r *SynchronousRunner) waitForTask(ctx context.Context, taskID string) (TaskResult, error) {
	execDone := make(chan error, 1)
	waitDone := make(chan struct {
		result TaskResult
		err    error
	}, 1)
	go func() { execDone <- r.Executor.RunOne(ctx) }()
	go func() {
		result, err := r.Registry.Wait(ctx, taskID)
		waitDone <- struct {
			result TaskResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case waited := <-waitDone:
		return waited.result, waited.err
	case err := <-execDone:
		if err != nil {
			return TaskResult{}, err
		}
		waited := <-waitDone
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
