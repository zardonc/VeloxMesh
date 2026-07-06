package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
)

func TestSynchronousRunnerRecordsSuccessSample(t *testing.T) {
	repo := &memoryTrainingSampleRepo{}
	runner := testTrainingRunner(repo)
	resp, err := runner.RunChat(context.Background(), testTrainingRequest(), func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		return &llm.LLMResponse{Provider: "openai-primary", Usage: &llm.Usage{PromptTokens: 3, CompletionTokens: 5}}, nil
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if resp == nil || len(repo.samples) != 1 {
		t.Fatalf("expected response and one sample, got resp=%#v samples=%d", resp, len(repo.samples))
	}
	sample := repo.samples[0]
	if sample.Outcome != TrainingOutcomeSuccess || sample.OutputTokens != 5 || sample.ProviderClass != "openai-primary" {
		t.Fatalf("unexpected sample: %#v", sample)
	}
}

func TestSynchronousRunnerRecordsFailureSample(t *testing.T) {
	repo := &memoryTrainingSampleRepo{}
	runner := testTrainingRunner(repo)
	boom := errors.New("provider failed")
	_, err := runner.RunChat(context.Background(), testTrainingRequest(), func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		return nil, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if len(repo.samples) != 1 || repo.samples[0].Outcome != TrainingOutcomeFailure {
		t.Fatalf("expected one failure sample, got %#v", repo.samples)
	}
}

func TestSynchronousRunnerDoesNotRecordAtEnqueue(t *testing.T) {
	repo := &memoryTrainingSampleRepo{}
	runner := testTrainingRunner(repo)
	_, err := runner.RunChat(context.Background(), testTrainingRequest(), func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		if len(repo.samples) != 0 {
			t.Fatalf("sample written before completion: %#v", repo.samples)
		}
		return &llm.LLMResponse{}, nil
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
}

func TestRecorderErrorDoesNotChangeResponse(t *testing.T) {
	repo := &memoryTrainingSampleRepo{err: errors.New("store unavailable")}
	runner := testTrainingRunner(repo)
	resp, err := runner.RunChat(context.Background(), testTrainingRequest(), func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		return &llm.LLMResponse{GatewayID: "ok"}, nil
	})
	if err != nil || resp.GatewayID != "ok" {
		t.Fatalf("recorder changed response: resp=%#v err=%v", resp, err)
	}
}

func TestTrainingSampleCopiesSemanticAggregates(t *testing.T) {
	task := Task{
		ID: "task-semantic",
		Feature: TaskFeature{
			NeighborCount: 7, LatencyP50Ms: 120, LatencyP90Ms: 240,
			LatencyStddevMs: 12.5, OutputTokensP70: 90, SuccessRate: 0.8,
			TimeoutRate: 0.1, CoverageLevel: SemanticCoverageTenant, CoverageRatio: 0.7,
		},
	}
	sample := schedulerTrainingSample(task, TrainingLabels{Outcome: TrainingOutcomeSuccess})
	assertSemanticAggregates(t, sample)
}

func testTrainingRunner(repo controlstate.SchedulerTrainingSampleRepository) *SynchronousRunner {
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh}, Backend: "memory",
	}
	runner := NewSynchronousRunner(intake, &Executor{Queue: queue, Registry: registry}, registry)
	runner.Recorder = &TrainingRecorder{Repo: repo}
	return runner
}

func testTrainingRequest() *llm.LLMRequest {
	return &llm.LLMRequest{RequestID: "task-1", Model: "gpt-4o-mini", Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello?"}}}
}

type memoryTrainingSampleRepo struct {
	samples []*controlstate.SchedulerTrainingSample
	err     error
}

func (r *memoryTrainingSampleRepo) Insert(ctx context.Context, sample *controlstate.SchedulerTrainingSample) error {
	if r.err != nil {
		return r.err
	}
	r.samples = append(r.samples, sample)
	return nil
}

func (r *memoryTrainingSampleRepo) ListByWindow(ctx context.Context, start, end time.Time, limit int) ([]*controlstate.SchedulerTrainingSample, error) {
	return r.samples, nil
}

func (r *memoryTrainingSampleRepo) ListByIDs(ctx context.Context, ids []string) ([]*controlstate.SchedulerTrainingSample, error) {
	byID := make(map[string]*controlstate.SchedulerTrainingSample, len(r.samples))
	for _, sample := range r.samples {
		byID[sample.ID] = sample
	}
	out := make([]*controlstate.SchedulerTrainingSample, 0, len(ids))
	for _, id := range ids {
		if sample := byID[id]; sample != nil {
			out = append(out, sample)
		}
	}
	return out, nil
}

func assertSemanticAggregates(t *testing.T, sample *controlstate.SchedulerTrainingSample) {
	t.Helper()
	if sample.NeighborCount != 7 || sample.LatencyP50Ms != 120 || sample.LatencyP90Ms != 240 {
		t.Fatalf("unexpected latency aggregates: %#v", sample)
	}
	if sample.LatencyStddevMs != 12.5 || sample.OutputTokensP70 != 90 {
		t.Fatalf("unexpected distribution aggregates: %#v", sample)
	}
	if sample.SuccessRate != 0.8 || sample.TimeoutRate != 0.1 {
		t.Fatalf("unexpected rate aggregates: %#v", sample)
	}
	if sample.CoverageLevel != SemanticCoverageTenant || sample.CoverageRatio != 0.7 {
		t.Fatalf("unexpected coverage aggregates: %#v", sample)
	}
}
