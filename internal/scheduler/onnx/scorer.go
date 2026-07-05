package onnx

import (
	"context"
	"math"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/heuristic"
)

type Scorer struct {
	artifact   *Artifact
	calculator *heuristic.ScoreCalculator
	loads      int
}

func NewScorer(artifactDir string) (*Scorer, error) {
	artifact, err := LoadArtifact(artifactDir)
	if err != nil {
		return nil, err
	}
	return &Scorer{artifact: artifact, calculator: heuristic.NewScoreCalculator(heuristic.DefaultConfig()), loads: 1}, nil
}

func (s *Scorer) LoadCount() int {
	return s.loads
}

func (s *Scorer) Score(_ context.Context, tasks []scheduler.TaskFeature) ([]scheduler.ScoreResult, error) {
	results := make([]scheduler.ScoreResult, 0, len(tasks))
	for _, task := range tasks {
		scored := s.scoreTask(task)
		results = append(results, scored)
	}
	return results, nil
}

func (s *Scorer) scoreTask(task scheduler.TaskFeature) scheduler.ScoreResult {
	task = normalizeSemanticAggregates(task, s.artifact.Manifest.SupportsSemanticAggregates())
	predictedTokens := int64(math.Round(s.artifact.Manifest.ModelParameters.P70OutputTokens))
	if predictedTokens > 0 {
		task.EstimatedOutputTokens = predictedTokens
	}
	score := s.calculator.Score(task).Result
	score.Confidence = s.confidence(task)
	score.SchedulerVersion = s.artifact.Manifest.SchedulerVersion
	score.SchedulerType = scheduler.SchedulerTypeONNX
	score.FallbackReason = "onnx"
	return score
}

func (s *Scorer) confidence(task scheduler.TaskFeature) float64 {
	base := 1.0
	if mae, ok := s.artifact.Manifest.Metrics["mae"]; ok {
		base = 1 / (1 + math.Max(mae, 0)/100)
	}
	coverage := featureCoverage(task, s.artifact.Manifest.SupportsSemanticAggregates())
	return math.Max(0.01, math.Min(base*coverage, 1))
}

func normalizeSemanticAggregates(task scheduler.TaskFeature, supported bool) scheduler.TaskFeature {
	if supported {
		if task.CoverageLevel == "" {
			task.CoverageLevel = scheduler.SemanticCoverageNone
		}
		return task
	}
	task.NeighborCount = 0
	task.LatencyP50Ms = 0
	task.LatencyP90Ms = 0
	task.LatencyStddevMs = 0
	task.OutputTokensP70 = 0
	task.SuccessRate = 0
	task.TimeoutRate = 0
	task.CoverageLevel = scheduler.SemanticCoverageNone
	task.CoverageRatio = 0
	return task
}

func featureCoverage(task scheduler.TaskFeature, semanticSupported bool) float64 {
	covered := 0
	checks := []bool{
		task.ModelClass != "",
		task.EstimatedInputTokens > 0,
		task.EstimatedOutputTokens > 0,
		task.Priority != "",
		task.RequestKind != "",
	}
	if semanticSupported {
		checks = append(checks, semanticCoverageChecks(task)...)
	}
	for _, ok := range checks {
		if ok {
			covered++
		}
	}
	return float64(covered) / float64(len(checks))
}

func semanticCoverageChecks(task scheduler.TaskFeature) []bool {
	return []bool{
		task.NeighborCount > 0,
		task.LatencyP50Ms > 0,
		task.LatencyP90Ms > 0,
		task.LatencyStddevMs > 0,
		task.OutputTokensP70 > 0,
		task.SuccessRate > 0,
		task.TimeoutRate > 0,
		task.CoverageLevel != "" && task.CoverageLevel != scheduler.SemanticCoverageNone,
		task.CoverageRatio > 0,
	}
}
