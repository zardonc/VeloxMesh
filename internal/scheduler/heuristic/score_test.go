package heuristic

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoadConfigFileAppliesNarrowOverrides(t *testing.T) {
	path := writeHeuristicConfig(t, `{"base_latency":{"simple_qa":1600},"model_multipliers":{"standard":2}}`)
	cfg, err := LoadConfigFile(path, DefaultConfig())
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if cfg.BaseLatencyMs[scheduler.RequestKindSimpleQA] != 1600 {
		t.Fatalf("base latency override missing: %#v", cfg.BaseLatencyMs)
	}
	if cfg.ModelMultiplier["standard"] != 2 {
		t.Fatalf("model multiplier override missing: %#v", cfg.ModelMultiplier)
	}
	if cfg.PriorityMultiplier[scheduler.PriorityHigh] != DefaultConfig().PriorityMultiplier[scheduler.PriorityHigh] {
		t.Fatalf("omitted priority defaults changed: %#v", cfg.PriorityMultiplier)
	}
}

func TestLoadConfigFileRejectsUnknownFields(t *testing.T) {
	path := writeHeuristicConfig(t, `{"priority_multipliers":{"high":1}}`)
	_, err := LoadConfigFile(path, DefaultConfig())
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestScoreCalculatorSetsSchedulerType(t *testing.T) {
	score := NewScoreCalculator(DefaultConfig()).Score(baseFeature()).Result
	if score.SchedulerType != scheduler.SchedulerTypeHeuristic {
		t.Fatalf("expected heuristic scheduler type, got %#v", score)
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

func writeHeuristicConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "heuristic.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write heuristic config: %v", err)
	}
	return path
}
