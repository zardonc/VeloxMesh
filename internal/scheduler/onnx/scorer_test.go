package onnx

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/scheduler"
)

func TestScorerLoadsArtifactOnceAndScoresRequests(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifact(t, "scheduler-p70-v1", 512, "scheduler-training-v1"))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	task := scheduler.TaskFeature{
		TaskID: "t1", ModelClass: "standard", EstimatedInputTokens: 100,
		EstimatedOutputTokens: 10, Priority: scheduler.PriorityNormal,
		RequestKind: scheduler.RequestKindSimpleQA, EnqueueTimeMs: time.Now().UnixMilli(),
	}
	first, err := scorer.Score(context.Background(), []scheduler.TaskFeature{task})
	if err != nil {
		t.Fatalf("Score first: %v", err)
	}
	second, err := scorer.Score(context.Background(), []scheduler.TaskFeature{task})
	if err != nil {
		t.Fatalf("Score second: %v", err)
	}
	if scorer.LoadCount() != 1 {
		t.Fatalf("expected one startup load, got %d", scorer.LoadCount())
	}
	if first[0].SchedulerVersion != "scheduler-p70-v1" || first[0].FallbackReason != "onnx" {
		t.Fatalf("unexpected score: %#v", first[0])
	}
	if second[0].PredictedLatencyMs != first[0].PredictedLatencyMs {
		t.Fatalf("expected stable prediction, got %d and %d", first[0].PredictedLatencyMs, second[0].PredictedLatencyMs)
	}
}

func TestLowFeatureCoverageLowersConfidence(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifact(t, "scheduler-p70-v1", 128, "scheduler-training-v1"))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	full, _ := scorer.Score(context.Background(), []scheduler.TaskFeature{{
		TaskID: "full", ModelClass: "standard", EstimatedInputTokens: 10,
		EstimatedOutputTokens: 10, Priority: scheduler.PriorityNormal, RequestKind: scheduler.RequestKindSimpleQA,
	}})
	low, _ := scorer.Score(context.Background(), []scheduler.TaskFeature{{TaskID: "low"}})
	if low[0].Confidence >= full[0].Confidence {
		t.Fatalf("expected low coverage confidence < full confidence: low=%f full=%f", low[0].Confidence, full[0].Confidence)
	}
}

func TestUnsupportedArtifactIgnoresSemanticCoverage(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifact(t, "scheduler-p70-v1", 128, "scheduler-training-v1"))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	base := fullCoverageTask("base")
	enriched := fullCoverageTask("enriched")
	enriched.NeighborCount = 20
	enriched.LatencyP50Ms = 100
	enriched.LatencyP90Ms = 200
	enriched.LatencyStddevMs = 20
	enriched.OutputTokensP70 = 70
	enriched.SuccessRate = 0.9
	enriched.TimeoutRate = 0.1
	enriched.CoverageLevel = scheduler.SemanticCoverageTenant
	enriched.CoverageRatio = 1

	got, err := scorer.Score(context.Background(), []scheduler.TaskFeature{base, enriched})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got[0].Confidence != got[1].Confidence || got[0].PredictedLatencyMs != got[1].PredictedLatencyMs {
		t.Fatalf("unsupported artifact changed score: %#v", got)
	}
}

func TestSupportedArtifactCountsSemanticCoverage(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifactWithSemanticSupport(t, "scheduler-p70-v2", true))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	missing := fullCoverageTask("missing")
	enriched := fullCoverageTask("enriched")
	enriched.NeighborCount = 20
	enriched.LatencyP50Ms = 100
	enriched.LatencyP90Ms = 200
	enriched.LatencyStddevMs = 20
	enriched.OutputTokensP70 = 70
	enriched.SuccessRate = 0.9
	enriched.TimeoutRate = 0.1
	enriched.CoverageLevel = scheduler.SemanticCoverageTenant
	enriched.CoverageRatio = 1

	got, err := scorer.Score(context.Background(), []scheduler.TaskFeature{missing, enriched})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got[1].Confidence <= got[0].Confidence {
		t.Fatalf("expected semantic coverage to raise confidence: %#v", got)
	}
}

func fullCoverageTask(id string) scheduler.TaskFeature {
	return scheduler.TaskFeature{
		TaskID: id, ModelClass: "standard", EstimatedInputTokens: 10,
		EstimatedOutputTokens: 10, Priority: scheduler.PriorityNormal,
		RequestKind: scheduler.RequestKindSimpleQA,
	}
}
