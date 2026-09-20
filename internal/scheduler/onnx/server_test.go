package onnx

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/schedulerv1"
)

func TestBatchScoreTasksReturnsExistingContractFields(t *testing.T) {
	scorer, err := NewScorer(writeTestArtifact(t, "scheduler-p70-v1", 256, "scheduler-training-v1"))
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	service := NewBatchScoreService(scorer)
	resp, err := service.BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{{
		TaskId: "t1", ModelClass: "standard", EstimatedInputTokens: 64,
		EstimatedOutputTokens: 16, Priority: "normal", RequestKind: "simple_qa",
		EnqueueTimeMs: time.Now().UnixMilli(),
	}}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	if len(resp.GetResults()) != 1 {
		t.Fatalf("expected one result, got %d", len(resp.GetResults()))
	}
	result := resp.GetResults()[0]
	if result.GetPredictedLatencyMs() <= 0 || result.GetConfidence() <= 0 || result.GetSchedulerVersion() != "scheduler-p70-v1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFeatureFromProtoPreservesSafeFeatureBuckets(t *testing.T) {
	got := featureFromProto(&schedulerv1.TaskFeature{
		TaskId: "t1", ModelClass: "standard", EstimatedInputTokens: 64,
		EstimatedOutputTokens: 16, Priority: "normal", RequestKind: "code_gen",
		MaxSentenceLengthBucket: 3, VocabularyRichnessBucket: 4,
	})
	if got.Priority != scheduler.PriorityNormal || got.RequestKind != scheduler.RequestKindCodeGen {
		t.Fatalf("unexpected normalized fields: %#v", got)
	}
	if got.MaxSentenceLengthBucket != 3 || got.VocabularyRichnessBucket != 4 {
		t.Fatalf("safe feature buckets were not preserved: %#v", got)
	}
}

func TestScoreToProtoMapsMetadata(t *testing.T) {
	got := scoreToProto(scheduler.ScoreResult{
		TaskID:               "t1",
		ClassificationSource: "onnx",
		AnomalyStatus:        scheduler.AnomalyStatusOOD,
	})

	if got.GetClassificationSource() != "onnx" || got.GetAnomalyStatus() != scheduler.AnomalyStatusOOD {
		t.Fatalf("score metadata not mapped: %#v", got)
	}
}
