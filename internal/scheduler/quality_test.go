package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
)

func TestCalculateMAPE(t *testing.T) {
	got, ok := CalculateMAPE(125, 100)
	if !ok || got != 25 {
		t.Fatalf("expected MAPE 25, got %v ok=%v", got, ok)
	}
	if _, ok := CalculateMAPE(125, 0); ok {
		t.Fatalf("expected zero actual latency to skip MAPE")
	}
}

func TestPredictionQualityRecorderWritesDurableRollup(t *testing.T) {
	repo := &qualityRepo{}
	recorder := &PredictionQualityRecorder{Repo: repo}
	completed := time.Date(2026, 7, 4, 12, 3, 0, 0, time.UTC)

	err := recorder.Record(context.Background(), qualityTask(completed), TrainingLabels{
		ActualLatencyMs: 100,
		CompletedAt:     completed,
	}, "safe-sample-1")
	if err != nil {
		t.Fatalf("record quality: %v", err)
	}
	if repo.rollup == nil || repo.rollup.MAPESum != 25 || repo.rollup.ModelClass != "standard" {
		t.Fatalf("unexpected rollup: %#v", repo.rollup)
	}
	if repo.rollup.SchedulerType == "" {
		t.Fatalf("expected non-empty scheduler type: %#v", repo.rollup)
	}
	if repo.rollup.ConfidenceSum != 0.75 || repo.rollup.SafeSampleIDs[0] != "safe-sample-1" {
		t.Fatalf("expected confidence and safe sample link, got %#v", repo.rollup)
	}
	if repo.rollup.CoverageLevel != SemanticCoverageTenant || repo.rollup.AnomalyCount != 1 || repo.rollup.AnomalyRate != 1 {
		t.Fatalf("expected anomaly rollup fields, got %#v", repo.rollup)
	}
}

func TestPredictionQualityRecorderRecordsUnavailableAnomalySeparately(t *testing.T) {
	repo := &qualityRepo{}
	recorder := &PredictionQualityRecorder{Repo: repo}
	completed := time.Date(2026, 7, 4, 12, 3, 0, 0, time.UTC)
	task := qualityTask(completed)
	task.Metadata[schedulerAnomalyStatusMeta] = AnomalyStatusDegraded

	err := recorder.Record(context.Background(), task, TrainingLabels{
		ActualLatencyMs: 100,
		CompletedAt:     completed,
	}, "safe-sample-1")
	if err != nil {
		t.Fatalf("record quality: %v", err)
	}
	if repo.rollup.AnomalyUnavailableCount != 1 || repo.rollup.ErrorCount != 0 {
		t.Fatalf("expected anomaly unavailable separate from errors, got %#v", repo.rollup)
	}
}

func TestPredictionQualityRecorderSkipsInvalidMAPE(t *testing.T) {
	repo := &qualityRepo{}
	recorder := &PredictionQualityRecorder{Repo: repo}
	task := qualityTask(time.Now())
	task.Metadata[schedulerPredictedLatencyMeta] = "0"

	if err := recorder.Record(context.Background(), task, TrainingLabels{ActualLatencyMs: 100, CompletedAt: time.Now()}, "sample"); err != nil {
		t.Fatalf("record quality: %v", err)
	}
	if repo.rollup != nil {
		t.Fatalf("expected no rollup for invalid MAPE, got %#v", repo.rollup)
	}
}

func TestCompletionEvidenceQualityErrorDoesNotPanic(t *testing.T) {
	runner := &SynchronousRunner{
		Intake:  &TaskIntake{},
		Quality: &PredictionQualityRecorder{Repo: &qualityRepo{err: errors.New("down")}},
	}
	runner.recordCompletionEvidence(context.Background(), &llm.LLMRequest{}, qualityTask(time.Now()), time.Now(), nil, TrainingOutcomeSuccess)
}

func qualityTask(now time.Time) Task {
	return Task{
		ID:          "task-1",
		EnqueueTime: now.Add(-time.Second),
		Feature: TaskFeature{
			ModelClass:    "standard",
			RequestKind:   RequestKindCodeGen,
			CoverageLevel: SemanticCoverageTenant,
		},
		Metadata: map[string]string{
			schedulerTypeMetadata:         string(SchedulerTypeONNX),
			schedulerVersionMetadata:      "v1",
			schedulerPredictedLatencyMeta: "125",
			schedulerConfidenceMetadata:   "0.75",
			schedulerCallLatencyMetadata:  "3",
			schedulerAnomalyStatusMeta:    AnomalyStatusOOD,
		},
	}
}

type qualityRepo struct {
	rollup *controlstate.SchedulerQualityRollup
	err    error
}

func (r *qualityRepo) Upsert(_ context.Context, rollup *controlstate.SchedulerQualityRollup) error {
	if r.err != nil {
		return r.err
	}
	r.rollup = rollup
	return nil
}

func (r *qualityRepo) ListByWindow(context.Context, time.Time, time.Time, string, string, string, int) ([]*controlstate.SchedulerQualityRollup, error) {
	return nil, nil
}
