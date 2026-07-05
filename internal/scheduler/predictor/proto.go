package predictor

import (
	"errors"
	"fmt"
	"time"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/predictorv1"
)

func taskToProto(task scheduler.TaskFeature) *predictorv1.TaskFeature {
	return &predictorv1.TaskFeature{
		TaskId: task.TaskID, ModelClass: task.ModelClass,
		EstimatedInputTokens: task.EstimatedInputTokens, EstimatedOutputTokens: task.EstimatedOutputTokens,
		Stream: task.Stream, Priority: string(task.Priority), TimeoutClass: task.TimeoutClass,
		EnqueueTimeMs: task.EnqueueTimeMs, RequestKind: string(task.RequestKind), RouteHint: task.RouteHint,
		HasToolCalls: task.HasToolCalls, ToolCallDepth: task.ToolCallDepth, TurnCount: task.TurnCount,
		Multimodal: task.Multimodal, QuestionCount: task.QuestionCount, CodeBlockCount: task.CodeBlockCount,
		EnumerationHint: task.EnumerationHint, InstructionVerbCount: task.InstructionVerbCount,
		MaxSentenceLengthBucket: task.MaxSentenceLengthBucket, VocabularyRichnessBucket: task.VocabularyRichnessBucket,
		ConfidenceHint: task.ConfidenceHint, UncertaintyHint: task.UncertaintyHint, NeighborCount: task.NeighborCount,
		LatencyP50Ms: task.LatencyP50Ms, LatencyP90Ms: task.LatencyP90Ms, LatencyStddevMs: task.LatencyStddevMs,
		OutputTokensP70: task.OutputTokensP70, SuccessRate: task.SuccessRate, TimeoutRate: task.TimeoutRate,
		CoverageLevel: task.CoverageLevel, CoverageRatio: task.CoverageRatio,
	}
}

func predictionsFromProto(resp *predictorv1.BatchPredictResponse, want int) ([]Prediction, error) {
	got := resp.GetPredictions()
	if len(got) != want {
		return nil, fmt.Errorf("predictor response length mismatch: got %d want %d", len(got), want)
	}
	predictions := make([]Prediction, len(got))
	for i, prediction := range got {
		predictions[i] = Prediction{
			Quantiles:    quantilesFromProto(prediction.GetQuantiles()),
			ModelVersion: prediction.GetModelVersion(),
			Signals:      prediction.GetSignals(),
			Err:          errorFromString(prediction.GetError()),
		}
	}
	return predictions, nil
}

func quantilesFromProto(values map[int32]float64) map[int]float64 {
	quantiles := make(map[int]float64, len(values))
	for key, value := range values {
		quantiles[int(key)] = value
	}
	return quantiles
}

func errorFromString(value string) error {
	if value == "" {
		return nil
	}
	return errors.New(value)
}

func timeoutOrDefault(value time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return 15 * time.Millisecond
}
