package scheduler

import (
	"context"
	"fmt"
	"time"

	"veloxmesh/internal/controlstate"
)

const (
	TrainingOutcomeSuccess = "success"
	TrainingOutcomeFailure = "failure"

	schedulerVersionMetadata = "scheduler_version"
)

type TrainingLabels struct {
	ActualLatencyMs int64
	InputTokens     int64
	OutputTokens    int64
	Outcome         string
	ProviderClass   string
	CompletedAt     time.Time
}

type TrainingRecorder struct {
	Repo controlstate.SchedulerTrainingSampleRepository
}

func (r *TrainingRecorder) Record(ctx context.Context, task Task, labels TrainingLabels) (string, error) {
	if r == nil || r.Repo == nil {
		return "", nil
	}
	sample := schedulerTrainingSample(task, labels)
	return sample.ID, r.Repo.Insert(ctx, sample)
}

func schedulerTrainingSample(task Task, labels TrainingLabels) *controlstate.SchedulerTrainingSample {
	completedAt := labels.CompletedAt
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	feature := task.Feature
	return &controlstate.SchedulerTrainingSample{
		ID: newTrainingSampleID(task.ID, completedAt), TaskID: task.ID,
		ModelClass: feature.ModelClass, EstimatedInputTokens: feature.EstimatedInputTokens,
		EstimatedOutputTokens: feature.EstimatedOutputTokens, Stream: feature.Stream,
		Priority: string(feature.Priority), TimeoutClass: feature.TimeoutClass,
		EnqueueTimeMs: feature.EnqueueTimeMs, RequestKind: string(feature.RequestKind),
		RouteHint: feature.RouteHint, HasToolCalls: feature.HasToolCalls,
		ToolCallDepth: feature.ToolCallDepth, TurnCount: feature.TurnCount,
		Multimodal: feature.Multimodal, QuestionCount: feature.QuestionCount,
		CodeBlockCount: feature.CodeBlockCount, EnumerationHint: feature.EnumerationHint,
		InstructionVerbCount:     feature.InstructionVerbCount,
		MaxSentenceLengthBucket:  feature.MaxSentenceLengthBucket,
		VocabularyRichnessBucket: feature.VocabularyRichnessBucket,
		ConfidenceHint:           feature.ConfidenceHint, UncertaintyHint: feature.UncertaintyHint,
		ActualLatencyMs: labels.ActualLatencyMs, InputTokens: labels.InputTokens,
		OutputTokens: labels.OutputTokens, Outcome: labels.Outcome,
		ProviderClass:    lowCardinality(labels.ProviderClass),
		SchedulerVersion: task.Metadata[schedulerVersionMetadata], CompletedAt: completedAt,
	}
}

func newTrainingSampleID(taskID string, completedAt time.Time) string {
	return fmt.Sprintf("%s-%d", taskID, completedAt.UnixNano())
}
