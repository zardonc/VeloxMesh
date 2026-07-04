package heuristic

import (
	"math"

	"veloxmesh/internal/scheduler"
)

type ScoreCalculator struct {
	cfg        Config
	classifier Classifier
}

type Score struct {
	Result               scheduler.ScoreResult
	ClassificationSource string
	UncertaintyPenaltyMs int64
}

func NewScoreCalculator(cfg Config) *ScoreCalculator {
	if cfg.Version == "" {
		cfg = DefaultConfig()
	}
	return &ScoreCalculator{cfg: cfg}
}

func (c *ScoreCalculator) Score(feature scheduler.TaskFeature) Score {
	classification := c.classifier.Classify(feature)
	predicted := c.predictedLatency(feature, classification.Kind)
	priority := c.priorityMultiplier(feature.Priority)
	uncertainty := c.uncertaintyPenalty(feature, classification.Confidence)
	final := float64(feature.EnqueueTimeMs) + float64(predicted)/priority + float64(uncertainty)
	return Score{
		Result: scheduler.ScoreResult{
			TaskID:             feature.TaskID,
			Score:              final,
			Priority:           feature.Priority,
			PredictedLatencyMs: predicted,
			Confidence:         classification.Confidence,
			SchedulerVersion:   c.cfg.Version,
			FallbackReason:     classification.Source,
		},
		ClassificationSource: classification.Source,
		UncertaintyPenaltyMs: uncertainty,
	}
}

func (c *ScoreCalculator) predictedLatency(feature scheduler.TaskFeature, kind scheduler.RequestKind) int64 {
	base := c.cfg.BaseLatencyMs[kind]
	if base == 0 {
		base = c.cfg.BaseLatencyMs[scheduler.RequestKindSimpleQA]
	}
	multiplier := c.cfg.ModelMultiplier[feature.ModelClass]
	if multiplier == 0 {
		multiplier = 1
	}
	tokens := max(feature.EstimatedInputTokens+feature.EstimatedOutputTokens, 1)
	predicted := int64(float64(base) * multiplier * math.Sqrt(float64(tokens)/256))
	if feature.HasToolCalls {
		predicted += c.cfg.ToolCallPenaltyMs
	}
	if feature.Stream && c.cfg.StreamDiscountPercent > 0 {
		predicted = predicted * (100 - c.cfg.StreamDiscountPercent) / 100
	}
	return max(predicted, 1)
}

func (c *ScoreCalculator) priorityMultiplier(priority scheduler.PriorityClass) float64 {
	multiplier := c.cfg.PriorityMultiplier[priority]
	if multiplier <= 0 {
		return 1
	}
	return multiplier
}

func (c *ScoreCalculator) uncertaintyPenalty(feature scheduler.TaskFeature, confidence float64) int64 {
	uncertainty := feature.UncertaintyHint
	if confidence < 0.8 {
		uncertainty += 1 - confidence
	}
	return int64(math.Round(uncertainty * c.cfg.UncertaintyPenaltyK * 1000))
}
