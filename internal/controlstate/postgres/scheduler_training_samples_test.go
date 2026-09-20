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
	found := findSample(got, sample.ID)
	if found == nil || found.OutputTokens != sample.OutputTokens {
		t.Fatalf("expected sample %q in %#v", sample.ID, got)
	}
	assertSemanticAggregates(t, found)
}

func TestPostgresSchedulerTrainingSamplesListDefaultsLegacyAggregates(t *testing.T) {
	ctx := context.Background()
	repo := openMigratedPostgres(t)
	sample := testSchedulerTrainingSample(uniquePostgresID(t, "scheduler-legacy-sample"))

	if err := insertLegacySchedulerTrainingSample(ctx, repo, sample); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	got, err := repo.SchedulerTrainingSamples().ListByWindow(ctx, sample.CompletedAt.Add(-time.Second), sample.CompletedAt.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected at least one sample")
	}
	found := findSample(got, sample.ID)
	if found == nil {
		t.Fatalf("expected sample %q in %#v", sample.ID, got)
	}
	assertNeutralSemanticAggregates(t, found)
}

func TestPostgresSchedulerTrainingSamplesListByIDsPreservesOrderAndOmitsMissing(t *testing.T) {
	ctx := context.Background()
	repo := openMigratedPostgres(t)
	first := testSchedulerTrainingSample(uniquePostgresID(t, "scheduler-sample-first"))
	second := testSchedulerTrainingSample(uniquePostgresID(t, "scheduler-sample-second"))
	for _, sample := range []*controlstate.SchedulerTrainingSample{first, second} {
		if err := repo.SchedulerTrainingSamples().Insert(ctx, sample); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
	}
	got, err := repo.SchedulerTrainingSamples().ListByIDs(ctx, []string{second.ID, "missing", first.ID})
	if err != nil {
		t.Fatalf("list by ids: %v", err)
	}
	if len(got) != 2 || got[0].ID != second.ID || got[1].ID != first.ID {
		t.Fatalf("unexpected ordered samples: %#v", got)
	}
	empty, err := repo.SchedulerTrainingSamples().ListByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("empty list by ids: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result, got %#v", empty)
	}
}

func TestSchedulerTrainingSampleWithCreatedAtDoesNotMutateInput(t *testing.T) {
	sample := testSchedulerTrainingSample(uniquePostgresID(t, "scheduler-sample"))
	sample.CreatedAt = time.Time{}
	fallback := time.Date(2026, 7, 4, 12, 1, 0, 0, time.UTC)

	got := schedulerTrainingSampleWithCreatedAt(sample, fallback)

	if !sample.CreatedAt.IsZero() {
		t.Fatalf("input sample was mutated: %#v", sample)
	}
	if got == sample || !got.CreatedAt.Equal(fallback) {
		t.Fatalf("unexpected prepared sample: got=%#v input=%#v", got, sample)
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
		NeighborCount: 7, LatencyP50Ms: 120, LatencyP90Ms: 240, LatencyStddevMs: 12.5,
		OutputTokensP70: 90, SuccessRate: 0.8, TimeoutRate: 0.1, CoverageLevel: "tenant",
		CoverageRatio: 0.7,
		CompletedAt:   completed,
	}
}

func insertLegacySchedulerTrainingSample(ctx context.Context, repo *Repository, sample *controlstate.SchedulerTrainingSample) error {
	_, err := repo.pool.Exec(ctx, `INSERT INTO scheduler_training_samples (
		id, task_id, model_class, estimated_input_tokens, estimated_output_tokens,
		stream, priority, timeout_class, enqueue_time_ms, request_kind, route_hint,
		has_tool_calls, tool_call_depth, turn_count, multimodal, question_count,
		code_block_count, enumeration_hint, instruction_verb_count,
		max_sentence_length_bucket, vocabulary_richness_bucket, confidence_hint,
		uncertainty_hint, actual_latency_ms, input_tokens, output_tokens, outcome,
		provider_class, scheduler_version, completed_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31)`,
		sample.ID, sample.TaskID, sample.ModelClass, sample.EstimatedInputTokens,
		sample.EstimatedOutputTokens, sample.Stream, sample.Priority, sample.TimeoutClass,
		sample.EnqueueTimeMs, sample.RequestKind, sample.RouteHint, sample.HasToolCalls,
		sample.ToolCallDepth, sample.TurnCount, sample.Multimodal, sample.QuestionCount,
		sample.CodeBlockCount, sample.EnumerationHint, sample.InstructionVerbCount,
		sample.MaxSentenceLengthBucket, sample.VocabularyRichnessBucket, sample.ConfidenceHint,
		sample.UncertaintyHint, sample.ActualLatencyMs, sample.InputTokens, sample.OutputTokens,
		sample.Outcome, sample.ProviderClass, sample.SchedulerVersion, sample.CompletedAt,
		sample.CompletedAt)
	return err
}

func assertSemanticAggregates(t *testing.T, sample *controlstate.SchedulerTrainingSample) {
	t.Helper()
	if sample.NeighborCount != 7 || sample.LatencyP50Ms != 120 || sample.LatencyP90Ms != 240 {
		t.Fatalf("unexpected semantic aggregates: %#v", sample)
	}
	if sample.CoverageLevel != "tenant" || sample.CoverageRatio != 0.7 {
		t.Fatalf("unexpected coverage aggregates: %#v", sample)
	}
}

func assertNeutralSemanticAggregates(t *testing.T, sample *controlstate.SchedulerTrainingSample) {
	t.Helper()
	if sample.NeighborCount != 0 || sample.LatencyP50Ms != 0 || sample.LatencyP90Ms != 0 {
		t.Fatalf("expected zero semantic aggregates: %#v", sample)
	}
	if sample.CoverageLevel != "none" || sample.CoverageRatio != 0 {
		t.Fatalf("expected neutral coverage: %#v", sample)
	}
}

func findSample(samples []*controlstate.SchedulerTrainingSample, id string) *controlstate.SchedulerTrainingSample {
	for _, sample := range samples {
		if sample.ID == id {
			return sample
		}
	}
	return nil
}
