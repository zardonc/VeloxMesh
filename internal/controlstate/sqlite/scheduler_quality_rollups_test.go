package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
)

func TestSchedulerQualityRollupsUpsertAndListByWindow(t *testing.T) {
	ctx := context.Background()
	repo, err := Open("file:scheduler-quality?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer repo.Close()
	if err := NewMigrator(repo.db).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := testSchedulerQualityRollup()
	second := testSchedulerQualityRollup()
	second.MAPESum = 75
	second.SafeSampleIDs = []string{"sample-2"}
	if err := repo.SchedulerQualityRollups().Upsert(ctx, first); err != nil {
		t.Fatalf("upsert first: %v", err)
	}
	if err := repo.SchedulerQualityRollups().Upsert(ctx, second); err != nil {
		t.Fatalf("upsert second: %v", err)
	}

	got, err := repo.SchedulerQualityRollups().ListByWindow(ctx, first.BucketStart.Add(-time.Second), first.BucketEnd.Add(time.Second), "onnx", "v1", "code_gen", 10)
	if err != nil {
		t.Fatalf("list rollups: %v", err)
	}
	if len(got) != 1 || got[0].SampleCount != 2 || got[0].MAPEAvg != 50 {
		t.Fatalf("unexpected rollup: %#v", got)
	}
	if got[0].CoverageLevel != "tenant" || got[0].AnomalyCount != 2 || got[0].AnomalyRate != 1 || got[0].AnomalyUnavailableCount != 2 {
		t.Fatalf("unexpected anomaly rollup fields: %#v", got[0])
	}
	if strings.Join(got[0].SafeSampleIDs, ",") != "sample-1,sample-2" {
		t.Fatalf("unexpected sample IDs: %#v", got[0].SafeSampleIDs)
	}
}

func TestSchedulerQualityRollupSchemaExcludesForbiddenFields(t *testing.T) {
	ctx := context.Background()
	repo, err := Open("file:scheduler-quality-schema?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer repo.Close()
	if err := NewMigrator(repo.db).Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rows, err := repo.db.QueryContext(ctx, "PRAGMA table_info(scheduler_quality_rollups)")
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
		for _, forbidden := range []string{"tenant", "api_key", "prompt", "message", "authorization", "secret", "payload", "payload_hash"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("scheduler quality column %q contains forbidden token %q", name, forbidden)
			}
		}
	}
}

func testSchedulerQualityRollup() *controlstate.SchedulerQualityRollup {
	start := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	return &controlstate.SchedulerQualityRollup{
		BucketStart: start, BucketEnd: start.Add(5 * time.Minute),
		SchedulerType: "onnx", SchedulerVersion: "v1", TaskType: "code_gen",
		ModelClass: "standard", SampleCount: 1, MAPESum: 25, WaitMSSum: 10,
		SchedulerCallLatencyMSSum: 3, ConfidenceSum: 0.8, CoverageLevel: "tenant",
		AnomalyCount: 1, AnomalyUnavailableCount: 1, SafeSampleIDs: []string{"sample-1"},
	}
}
