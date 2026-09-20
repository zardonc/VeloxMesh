package postgres

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
)

func TestPostgresSchedulerQualityRollupsUpsertAndListByWindow(t *testing.T) {
	ctx := context.Background()
	repo := openMigratedPostgres(t)
	first := testSchedulerQualityRollup(uniquePostgresID(t, "sample-1"))
	second := testSchedulerQualityRollup(uniquePostgresID(t, "sample-2"))
	second.BucketStart = first.BucketStart
	second.BucketEnd = first.BucketEnd
	second.MAPESum = 75

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
	if len(got) == 0 || got[0].SampleCount != 2 || got[0].MAPEAvg != 50 {
		t.Fatalf("unexpected rollup: %#v", got)
	}
	if got[0].CoverageLevel != "tenant" || got[0].AnomalyCount != 2 || got[0].AnomalyRate != 1 || got[0].AnomalyUnavailableCount != 2 {
		t.Fatalf("unexpected anomaly rollup fields: %#v", got[0])
	}
}

func testSchedulerQualityRollup(sampleID string) *controlstate.SchedulerQualityRollup {
	start := time.Now().UTC().Truncate(time.Microsecond)
	return &controlstate.SchedulerQualityRollup{
		BucketStart: start, BucketEnd: start.Add(5 * time.Minute),
		SchedulerType: "onnx", SchedulerVersion: "v1", TaskType: "code_gen",
		ModelClass: "standard", SampleCount: 1, MAPESum: 25, WaitMSSum: 10,
		SchedulerCallLatencyMSSum: 3, ConfidenceSum: 0.8, CoverageLevel: "tenant",
		AnomalyCount: 1, AnomalyUnavailableCount: 1, SafeSampleIDs: []string{sampleID},
	}
}
