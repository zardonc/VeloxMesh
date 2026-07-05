package heuristic

import (
	"testing"

	"veloxmesh/internal/scheduler"
)

func TestScoreCalculatorEnqueueAging(t *testing.T) {
	calc := NewScoreCalculator(DefaultConfig())
	old := baseFeature()
	newer := baseFeature()
	old.EnqueueTimeMs = 1000
	newer.EnqueueTimeMs = 2000
	if calc.Score(old).Result.Score >= calc.Score(newer).Result.Score {
		t.Fatalf("earlier enqueue should score lower")
	}
}

func TestScoreCalculatorPriorityMultipliersOrderScores(t *testing.T) {
	calc := NewScoreCalculator(DefaultConfig())
	high, normal, low := baseFeature(), baseFeature(), baseFeature()
	high.Priority = scheduler.PriorityHigh
	normal.Priority = scheduler.PriorityNormal
	low.Priority = scheduler.PriorityLow
	hs, ns, ls := calc.Score(high).Result.Score, calc.Score(normal).Result.Score, calc.Score(low).Result.Score
	if !(hs < ns && ns < ls) {
		t.Fatalf("priority scores out of order: high=%f normal=%f low=%f", hs, ns, ls)
	}
}

func TestScoreCalculatorUncertaintyPenalty(t *testing.T) {
	calc := NewScoreCalculator(DefaultConfig())
	base := baseFeature()
	uncertain := baseFeature()
	uncertain.ConfidenceHint = 0.4
	uncertain.UncertaintyHint = 2
	score := calc.Score(uncertain)
	if score.Result.Score <= calc.Score(base).Result.Score {
		t.Fatalf("uncertainty should increase final score")
	}
	if score.UncertaintyPenaltyMs <= 0 {
		t.Fatalf("expected uncertainty penalty")
	}
}

func TestClassifierFallback(t *testing.T) {
	got := (Classifier{}).Classify(scheduler.TaskFeature{})
	if got.Kind != scheduler.RequestKindSimpleQA || got.Source != "fallback" {
		t.Fatalf("unexpected fallback classification: %#v", got)
	}
}

func TestScoreCalculatorIgnoresSemanticAggregates(t *testing.T) {
	calc := NewScoreCalculator(DefaultConfig())
	base := baseFeature()
	enriched := baseFeature()
	enriched.NeighborCount = 20
	enriched.LatencyP50Ms = 100
	enriched.LatencyP90Ms = 200
	enriched.LatencyStddevMs = 20
	enriched.OutputTokensP70 = 70
	enriched.SuccessRate = 0.9
	enriched.TimeoutRate = 0.1
	enriched.CoverageLevel = scheduler.SemanticCoverageTenant
	enriched.CoverageRatio = 1

	baseScore := calc.Score(base).Result
	enrichedScore := calc.Score(enriched).Result
	if baseScore != enrichedScore {
		t.Fatalf("semantic aggregates changed heuristic score: base=%#v enriched=%#v", baseScore, enrichedScore)
	}
}

func baseFeature() scheduler.TaskFeature {
	return scheduler.TaskFeature{
		TaskID:                "t1",
		ModelClass:            "standard",
		EstimatedInputTokens:  256,
		EstimatedOutputTokens: 256,
		Priority:              scheduler.PriorityNormal,
		RequestKind:           scheduler.RequestKindSimpleQA,
		EnqueueTimeMs:         1000,
		ConfidenceHint:        1,
	}
}
