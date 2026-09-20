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
	if first[0].SchedulerVersion != "scheduler-p70-v1" || first[0].FallbackReason != "" || first[0].ClassificationSource != "onnx" {
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

func TestAnomalyThresholdLowersConfidenceAndRaisesScore(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifactWithAnomaly(t, "tenant", 0.5))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	normal := semanticTask("normal", 100, 120)
	ood := semanticTask("ood", 100, 300)

	got, err := scorer.Score(context.Background(), []scheduler.TaskFeature{normal, ood})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got[1].Confidence >= got[0].Confidence {
		t.Fatalf("expected OOD confidence lower: %#v", got)
	}
	if got[1].Score <= got[0].Score {
		t.Fatalf("expected OOD score more conservative: %#v", got)
	}
	if got[1].AnomalyStatus != scheduler.AnomalyStatusOOD {
		t.Fatalf("expected OOD anomaly status: %#v", got[1])
	}
}

func TestAnomalyThresholdFallsBackToAllCoverage(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifactWithAnomaly(t, "all", 0.5))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	got, err := scorer.Score(context.Background(), []scheduler.TaskFeature{semanticTask("fallback", 100, 300)})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if got[0].AnomalyStatus != scheduler.AnomalyStatusOOD {
		t.Fatalf("expected all coverage fallback to mark OOD: %#v", got[0])
	}
}

func TestMissingAnomalyMetadataLeavesScoreUnchanged(t *testing.T) {
	withMissing, err := NewScorer(writeTestArtifactManifest(t, "scheduler-p70-v2", 128, "scheduler-training-v1", func(manifest *Manifest) {
		manifest.SemanticSupport = true
		manifest.SemanticFeatures = append([]string{}, semanticAggregateFeatureNames...)
		manifest.Features = append([]string{"estimated_input_tokens"}, semanticAggregateFeatureNames...)
	}))
	if err != nil {
		t.Fatalf("NewScorer missing: %v", err)
	}
	withAnomaly, err := NewScorer(writeTestArtifactWithAnomaly(t, "tenant", 100))
	if err != nil {
		t.Fatalf("NewScorer anomaly: %v", err)
	}
	task := semanticTask("same", 100, 120)
	missing, _ := withMissing.Score(context.Background(), []scheduler.TaskFeature{task})
	normal, _ := withAnomaly.Score(context.Background(), []scheduler.TaskFeature{task})

	if missing[0].Score != normal[0].Score || missing[0].Confidence != normal[0].Confidence {
		t.Fatalf("missing anomaly metadata changed scoring: missing=%#v normal=%#v", missing[0], normal[0])
	}
	if missing[0].AnomalyStatus != scheduler.AnomalyStatusUnavailable {
		t.Fatalf("expected unavailable anomaly status: %#v", missing[0])
	}
}

func fullCoverageTask(id string) scheduler.TaskFeature {
	return scheduler.TaskFeature{
		TaskID: id, ModelClass: "standard", EstimatedInputTokens: 10,
		EstimatedOutputTokens: 10, Priority: scheduler.PriorityNormal,
		RequestKind: scheduler.RequestKindSimpleQA,
	}
}

func semanticTask(id string, latencyP50, latencyP90 int64) scheduler.TaskFeature {
	task := fullCoverageTask(id)
	task.NeighborCount = 20
	task.LatencyP50Ms = latencyP50
	task.LatencyP90Ms = latencyP90
	task.LatencyStddevMs = 10
	task.OutputTokensP70 = 70
	task.SuccessRate = 1
	task.TimeoutRate = 0
	task.CoverageLevel = scheduler.SemanticCoverageTenant
	task.CoverageRatio = 1
	return task
}

func writeTestArtifactWithAnomaly(t *testing.T, coverage string, threshold float64) string {
	return writeTestArtifactManifest(t, "scheduler-p70-v2", 128, "scheduler-training-v1", func(manifest *Manifest) {
		manifest.SemanticSupport = true
		manifest.SemanticFeatures = append([]string{}, semanticAggregateFeatureNames...)
		manifest.Features = append([]string{"estimated_input_tokens"}, semanticAggregateFeatureNames...)
		manifest.AnomalyThresholds = map[string]map[string]AnomalyThreshold{
			string(scheduler.RequestKindSimpleQA): {
				coverage: {Threshold: threshold, SampleCount: 20, Mean: 0.1, Stddev: 0.1},
			},
		}
	})
}
