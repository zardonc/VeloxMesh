package scheduler

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"veloxmesh/internal/scheduler/schedulerv1"
)

func TestTaskFeatureProtoMapsSemanticAggregates(t *testing.T) {
	feature := TaskFeature{
		NeighborCount:   23,
		LatencyP50Ms:    110,
		LatencyP90Ms:    250,
		LatencyStddevMs: 12.5,
		OutputTokensP70: 900,
		SuccessRate:     0.75,
		TimeoutRate:     0.1,
		CoverageLevel:   SemanticCoverageTenant,
		CoverageRatio:   0.8,
	}

	got := feature.proto()
	if got.GetNeighborCount() != 23 || got.GetLatencyP50Ms() != 110 || got.GetLatencyP90Ms() != 250 {
		t.Fatalf("latency aggregate fields not mapped: %#v", got)
	}
	if got.GetLatencyStddevMs() != 12.5 || got.GetOutputTokensP70() != 900 {
		t.Fatalf("distribution aggregate fields not mapped: %#v", got)
	}
	if got.GetSuccessRate() != 0.75 || got.GetTimeoutRate() != 0.1 {
		t.Fatalf("rate aggregate fields not mapped: %#v", got)
	}
	if got.GetCoverageLevel() != SemanticCoverageTenant || got.GetCoverageRatio() != 0.8 {
		t.Fatalf("coverage aggregate fields not mapped: %#v", got)
	}

	data, err := proto.Marshal(got)
	if err != nil {
		t.Fatalf("marshal task feature: %v", err)
	}
	var roundTrip schedulerv1.TaskFeature
	if err := proto.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("unmarshal task feature: %v", err)
	}
	if roundTrip.GetNeighborCount() != 23 || roundTrip.GetCoverageLevel() != SemanticCoverageTenant {
		t.Fatalf("semantic aggregate fields missing from wire round trip: %#v", &roundTrip)
	}
}
