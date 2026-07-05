package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
)

func TestSchedulerTrainingSamplesInsertAndListByWindow(t *testing.T) {
	ctx := context.Background()
	repo, err := Open("file:scheduler-samples?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer repo.Close()
	if err := NewMigrator(repo.db).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sample := testSchedulerTrainingSample()
	if err := repo.SchedulerTrainingSamples().Insert(ctx, sample); err != nil {
		t.Fatalf("insert sample: %v", err)
	}
	got, err := repo.SchedulerTrainingSamples().ListByWindow(ctx, sample.CompletedAt.Add(-time.Second), sample.CompletedAt.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != sample.TaskID || got[0].OutputTokens != sample.OutputTokens {
		t.Fatalf("unexpected samples: %#v", got)
	}
	assertSemanticAggregates(t, got[0])
}

func TestSchedulerTrainingSamplesListDefaultsLegacyAggregates(t *testing.T) {
	ctx := context.Background()
	repo, err := Open("file:scheduler-samples-defaults?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer repo.Close()
	if err := NewMigrator(repo.db).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	sample := testSchedulerTrainingSample()
	sample.ID = "legacy-sample"
	if err := insertLegacySchedulerTrainingSample(ctx, repo, sample); err != nil {
		t.Fatalf("legacy insert: %v", err)
	}
	got, err := repo.SchedulerTrainingSamples().ListByWindow(ctx, sample.CompletedAt.Add(-time.Second), sample.CompletedAt.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one sample, got %#v", got)
	}
	assertNeutralSemanticAggregates(t, got[0])
}

func TestSchedulerTrainingSampleSchemaExcludesForbiddenFields(t *testing.T) {
	ctx := context.Background()
	repo, err := Open("file:scheduler-sample-schema?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer repo.Close()
	if err := NewMigrator(repo.db).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rows, err := repo.db.QueryContext(ctx, "PRAGMA table_info(scheduler_training_samples)")
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		assertSafeTrainingColumn(t, name)
	}
}

func TestSchedulerTrainingSampleWithCreatedAtDoesNotMutateInput(t *testing.T) {
	sample := testSchedulerTrainingSample()
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

func assertSafeTrainingColumn(t *testing.T, name string) {
	t.Helper()
	for _, forbidden := range []string{"prompt", "message", "authorization", "api_key", "secret", "payload", "hash"} {
		if strings.Contains(name, forbidden) {
			t.Fatalf("scheduler training column %q contains forbidden token %q", name, forbidden)
		}
	}
}

func testSchedulerTrainingSample() *controlstate.SchedulerTrainingSample {
	completed := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	return &controlstate.SchedulerTrainingSample{
		ID: "sample-1", TaskID: "task-1", ModelClass: "standard",
		EstimatedInputTokens: 12, EstimatedOutputTokens: 128, Stream: false,
		Priority: "normal", TimeoutClass: "standard", EnqueueTimeMs: completed.Add(-time.Second).UnixMilli(),
		RequestKind: "simple_qa", RouteHint: "openai", HasToolCalls: false,
		ToolCallDepth: 0, TurnCount: 1, Multimodal: false, QuestionCount: 1,
		CodeBlockCount: 0, EnumerationHint: false, InstructionVerbCount: 1,
		MaxSentenceLengthBucket: 2, VocabularyRichnessBucket: 3, ConfidenceHint: 1,
		UncertaintyHint: 0, ActualLatencyMs: 42, InputTokens: 12, OutputTokens: 80,
		Outcome: "success", ProviderClass: "openai-compatible", SchedulerVersion: "heuristic-v1",
		NeighborCount: 7, LatencyP50Ms: 120, LatencyP90Ms: 240, LatencyStddevMs: 12.5,
		OutputTokensP70: 90, SuccessRate: 0.8, TimeoutRate: 0.1, CoverageLevel: "tenant",
		CoverageRatio: 0.7,
		CompletedAt:   completed,
	}
}

func insertLegacySchedulerTrainingSample(ctx context.Context, repo *Repository, sample *controlstate.SchedulerTrainingSample) error {
	_, err := repo.db.ExecContext(ctx, `INSERT INTO scheduler_training_samples (
		id, task_id, model_class, estimated_input_tokens, estimated_output_tokens,
		stream, priority, timeout_class, enqueue_time_ms, request_kind, route_hint,
		has_tool_calls, tool_call_depth, turn_count, multimodal, question_count,
		code_block_count, enumeration_hint, instruction_verb_count,
		max_sentence_length_bucket, vocabulary_richness_bucket, confidence_hint,
		uncertainty_hint, actual_latency_ms, input_tokens, output_tokens, outcome,
		provider_class, scheduler_version, completed_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
