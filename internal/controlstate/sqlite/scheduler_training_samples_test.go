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
		CompletedAt: completed,
	}
}
