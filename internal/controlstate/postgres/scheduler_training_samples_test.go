package postgres

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
)

func TestPostgresSchedulerTrainingSamplesInsertAndListByWindow(t *testing.T) {
	ctx := context.Background()
	repo := openMigratedPostgres(t)
	sample := testSchedulerTrainingSample(uniquePostgresID(t, "scheduler-sample"))

	if err := repo.SchedulerTrainingSamples().Insert(ctx, sample); err != nil {
		t.Fatalf("insert sample: %v", err)
	}
	got, err := repo.SchedulerTrainingSamples().ListByWindow(ctx, sample.CompletedAt.Add(-time.Second), sample.CompletedAt.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one sample")
	}
	if got[0].ID != sample.ID || got[0].OutputTokens != sample.OutputTokens {
		t.Fatalf("unexpected sample: %#v", got[0])
	}
}

func testSchedulerTrainingSample(id string) *controlstate.SchedulerTrainingSample {
	completed := time.Now().UTC().Truncate(time.Microsecond)
	return &controlstate.SchedulerTrainingSample{
		ID: id, TaskID: id + "-task", ModelClass: "standard",
		EstimatedInputTokens: 12, EstimatedOutputTokens: 128, Priority: "normal",
		TimeoutClass: "standard", EnqueueTimeMs: completed.Add(-time.Second).UnixMilli(),
		RequestKind: "simple_qa", RouteHint: "openai", TurnCount: 1,
		QuestionCount: 1, InstructionVerbCount: 1, MaxSentenceLengthBucket: 2,
		VocabularyRichnessBucket: 3, ConfidenceHint: 1, ActualLatencyMs: 42,
		InputTokens: 12, OutputTokens: 80, Outcome: "success",
		ProviderClass: "openai-compatible", SchedulerVersion: "heuristic-v1",
		CompletedAt: completed,
	}
}
