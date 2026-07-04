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
	coverage := featureCoverage(task)
	return math.Max(0.01, math.Min(base*coverage, 1))
}

func featureCoverage(task scheduler.TaskFeature) float64 {
	covered := 0
	for _, ok := range []bool{
		task.ModelClass != "",
		task.EstimatedInputTokens > 0,
		task.EstimatedOutputTokens > 0,
		task.Priority != "",
		task.RequestKind != "",
	} {
		if ok {
			covered++
		}
	}
	return float64(covered) / 5
}
