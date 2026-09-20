package predictive

import (
	"context"
	"errors"
	"testing"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/predictor"
	"veloxmesh/internal/scheduler/schedulerv1"
)

func TestPolicyQuantileChangesWithoutPredictorContractChange(t *testing.T) {
	task := taskFeature("q")
	pred := fixedPredictor{predictions: []predictor.Prediction{{ModelVersion: "v1", Quantiles: map[int]float64{50: 50, 70: 70, 90: 900}}}}
	p50, _ := NewScorer(&pred, Config{Quantile: 50}).Score(context.Background(), []scheduler.TaskFeature{task})
	p90, _ := NewScorer(&pred, Config{Quantile: 90}).Score(context.Background(), []scheduler.TaskFeature{task})
	if p90[0].PredictedLatencyMs <= p50[0].PredictedLatencyMs {
		t.Fatalf("expected P90 policy to raise predicted latency: p50=%#v p90=%#v", p50[0], p90[0])
	}
}

func TestOODPolicyUsesPredictorSignalsInScheduler(t *testing.T) {
	task := taskFeature("ood")
	normal := fixedPredictor{predictions: []predictor.Prediction{{ModelVersion: "v1", Quantiles: map[int]float64{70: 70}, Signals: map[string]float64{"ood_distance": 0.1}}}}
	ood := fixedPredictor{predictions: []predictor.Prediction{{ModelVersion: "v1", Quantiles: map[int]float64{70: 70}, Signals: map[string]float64{"ood_distance": 3}}}}
	normalScore, _ := NewScorer(&normal, Config{OODThreshold: 1}).Score(context.Background(), []scheduler.TaskFeature{task})
	oodScore, _ := NewScorer(&ood, Config{OODThreshold: 1}).Score(context.Background(), []scheduler.TaskFeature{task})
	if oodScore[0].AnomalyStatus != scheduler.AnomalyStatusOOD || oodScore[0].Confidence >= normalScore[0].Confidence {
		t.Fatalf("expected scheduler OOD policy to lower confidence: normal=%#v ood=%#v", normalScore[0], oodScore[0])
	}
}

func TestMalformedPredictionFallsBackWithoutBlockingSibling(t *testing.T) {
	pred := fixedPredictor{predictions: []predictor.Prediction{
		{ModelVersion: "v1", Err: errors.New("bad_task")},
		{ModelVersion: "v1", Quantiles: map[int]float64{70: 70}},
	}}
	got, _ := NewScorer(&pred, Config{}).Score(context.Background(), []scheduler.TaskFeature{taskFeature("bad"), taskFeature("ok")})
	if got[0].FallbackReason != "predictor_task_error" || got[1].SchedulerType != scheduler.SchedulerTypePredictive {
		t.Fatalf("unexpected partial fallback: %#v", got)
	}
}

func TestServiceMappingPreservesSemanticAggregateFields(t *testing.T) {
	capturing := fixedPredictor{predictions: []predictor.Prediction{{ModelVersion: "v1", Quantiles: map[int]float64{70: 70}}}}
	service := NewBatchScoreService(NewScorer(&capturing, Config{}))
	_, err := service.BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{{
		TaskId: "semantic", NeighborCount: 7, LatencyP50Ms: 100, LatencyP90Ms: 200,
		LatencyStddevMs: 12, OutputTokensP70: 99, SuccessRate: 0.9, TimeoutRate: 0.1,
		CoverageLevel: scheduler.SemanticCoverageTenant, CoverageRatio: 0.8,
	}}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	got := capturing.tasks[0]
	if got.NeighborCount != 7 || got.OutputTokensP70 != 99 || got.CoverageLevel != scheduler.SemanticCoverageTenant {
		t.Fatalf("semantic fields were not preserved: %#v", got)
	}
}

type fixedPredictor struct {
	predictions []predictor.Prediction
	tasks       []scheduler.TaskFeature
}

func (p *fixedPredictor) Predict(_ context.Context, tasks []scheduler.TaskFeature) ([]predictor.Prediction, error) {
	p.tasks = append([]scheduler.TaskFeature(nil), tasks...)
	return p.predictions, nil
}

func taskFeature(id string) scheduler.TaskFeature {
	return scheduler.TaskFeature{
		TaskID: id, ModelClass: "standard", EstimatedInputTokens: 100, EstimatedOutputTokens: 10,
		Priority: scheduler.PriorityNormal, RequestKind: scheduler.RequestKindSimpleQA,
	}
}
