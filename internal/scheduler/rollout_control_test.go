package scheduler

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/config"
)

func TestRolloutControllerUpdateChangesWeightedScorerWithoutRebuild(t *testing.T) {
	controller := NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 100})
	heuristic := &recordingScorer{result: ScoreResult{SchedulerVersion: "heuristic-v1"}}
	onnx := &recordingScorer{result: ScoreResult{SchedulerVersion: "onnx-v1"}}
	scorer := WeightedScorer{Heuristic: heuristic, ONNX: onnx, Controller: controller}

	if _, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1"}}); err != nil {
		t.Fatalf("score onnx: %v", err)
	}
	if onnx.calls != 1 {
		t.Fatalf("expected ONNX call before rollback")
	}
	if _, err := controller.SetONNXRolloutPercent(0); err != nil {
		t.Fatalf("set percent: %v", err)
	}
	if _, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t2"}}); err != nil {
		t.Fatalf("score heuristic: %v", err)
	}
	if heuristic.calls != 1 || onnx.calls != 1 {
		t.Fatalf("expected heuristic after rollback, heuristic=%d onnx=%d", heuristic.calls, onnx.calls)
	}
}

func TestPredictionQualityAlertsDoNotChangeRolloutPercent(t *testing.T) {
	controller := NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 50, QualityMAPEAlertPercent: 10, QualitySampleWindow: 3})
	recorder := &PredictionQualityRecorder{Controller: controller}
	task := qualityTask(time.Now())

	for i := 0; i < 2; i++ {
		if err := recorder.Record(context.Background(), task, TrainingLabels{ActualLatencyMs: 100, CompletedAt: time.Now()}, "sample"); err != nil {
			t.Fatalf("record quality: %v", err)
		}
	}
	if alerts := controller.Snapshot().Alerts; len(alerts) != 0 {
		t.Fatalf("expected no alert before sample window fills, got %#v", alerts)
	}
	for i := 0; i < 2; i++ {
		if err := recorder.Record(context.Background(), task, TrainingLabels{ActualLatencyMs: 100, CompletedAt: time.Now()}, "sample"); err != nil {
			t.Fatalf("record quality: %v", err)
		}
	}
	status := controller.Snapshot()
	if status.ONNXRolloutPercent != 50 {
		t.Fatalf("alert changed rollout percent: %#v", status)
	}
	if len(status.Alerts) != 1 || status.Alerts[0].Reason != RolloutAlertMAPEDegradation {
		t.Fatalf("expected mape alert, got %#v", status.Alerts)
	}
}

func TestPredictionQualityErrorSpikeAlertDoesNotChangeRolloutPercent(t *testing.T) {
	controller := NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 50, ErrorSpikeAlertRate: 0.5, QualitySampleWindow: 3})
	recorder := &PredictionQualityRecorder{Controller: controller}
	errorTask := qualityTask(time.Now())
	errorTask.Metadata[schedulerPredictedLatencyMeta] = "0"
	validTask := qualityTask(time.Now())
	validTask.Metadata[schedulerPredictedLatencyMeta] = "100"

	for _, task := range []Task{errorTask, validTask, errorTask, errorTask} {
		if err := recorder.Record(context.Background(), task, TrainingLabels{ActualLatencyMs: 100, CompletedAt: time.Now()}, "sample"); err != nil {
			t.Fatalf("record quality: %v", err)
		}
	}
	status := controller.Snapshot()
	if status.ONNXRolloutPercent != 50 {
		t.Fatalf("alert changed rollout percent: %#v", status)
	}
	if len(status.Alerts) != 1 || status.Alerts[0].Reason != RolloutAlertSchedulerErrorSpike {
		t.Fatalf("expected error spike alert, got %#v", status.Alerts)
	}
}

func TestRolloutControllerKeepsLatestHundredAlerts(t *testing.T) {
	controller := NewSchedulerRolloutController(config.SchedulerConfig{QualitySampleWindow: 100})
	for i := 0; i < 105; i++ {
		controller.RecordAlert(RolloutAlertMAPEDegradation, string(rune('a'+i%26)))
	}
	alerts := controller.Snapshot().Alerts
	if len(alerts) != 100 {
		t.Fatalf("expected latest 100 alerts, got %d", len(alerts))
	}
}
