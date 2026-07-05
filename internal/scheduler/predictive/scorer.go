package predictive

import (
	"context"
	"math"

	"veloxmesh/internal/observability"
	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/heuristic"
	"veloxmesh/internal/scheduler/predictor"
)

type Config struct {
	Quantile     int
	OODThreshold float64
	Version      string
	Metrics      observability.Metrics
}

type Scorer struct {
	predictor  predictor.OutputTokenPredictor
	calculator *heuristic.ScoreCalculator
	cfg        Config
}

func NewScorer(predictorImpl predictor.OutputTokenPredictor, cfg Config) *Scorer {
	if predictorImpl == nil {
		predictorImpl = predictor.NoopPredictor{}
	}
	if cfg.Quantile == 0 {
		cfg.Quantile = 70
	}
	if cfg.OODThreshold <= 0 {
		cfg.OODThreshold = 1
	}
	if cfg.Version == "" {
		cfg.Version = "predictive"
	}
	return &Scorer{predictor: predictorImpl, calculator: heuristic.NewScoreCalculator(heuristic.DefaultConfig()), cfg: cfg}
}

func (s *Scorer) Score(ctx context.Context, tasks []scheduler.TaskFeature) ([]scheduler.ScoreResult, error) {
	predictions, err := s.predictor.Predict(ctx, tasks)
	if err != nil || len(predictions) != len(tasks) {
		return s.fallback(tasks, "predictor_error"), nil
	}
	results := make([]scheduler.ScoreResult, len(tasks))
	for i, task := range tasks {
		results[i] = s.scoreTask(task, predictions[i])
	}
	return results, nil
}

func (s *Scorer) scoreTask(task scheduler.TaskFeature, prediction predictor.Prediction) scheduler.ScoreResult {
	if prediction.Err != nil {
		return s.fallbackOne(task, "predictor_task_error")
	}
	estimate, ok := prediction.Quantiles[s.cfg.Quantile]
	if !ok || estimate <= 0 {
		return s.fallbackOne(task, "missing_quantile")
	}
	task.EstimatedOutputTokens = int64(math.Round(estimate))
	adjusted, status, confidence := s.applySignals(task, prediction.Signals)
	score := s.calculator.Score(adjusted).Result
	score.Confidence = confidence
	score.SchedulerType = scheduler.SchedulerTypePredictive
	score.SchedulerVersion = firstNonEmpty(prediction.ModelVersion, s.cfg.Version)
	score.FallbackReason = ""
	score.AnomalyStatus = status
	s.recordAnomaly(adjusted, score.SchedulerVersion, status)
	return score
}

func (s *Scorer) applySignals(task scheduler.TaskFeature, signals map[string]float64) (scheduler.TaskFeature, string, float64) {
	spreadRatio := signals["quantile_spread"] / math.Max(float64(task.EstimatedOutputTokens), 1)
	oodDistance := signals["ood_distance"]
	severity := math.Max((oodDistance-s.cfg.OODThreshold)/s.cfg.OODThreshold, 0)
	task.UncertaintyHint += math.Max(spreadRatio, 0) + math.Min(severity, 5)
	confidence := math.Max(0.05, 1/(1+math.Max(spreadRatio, 0)+severity))
	if severity > 0 {
		return task, scheduler.AnomalyStatusOOD, confidence
	}
	return task, scheduler.AnomalyStatusNormal, confidence
}

func (s *Scorer) fallback(tasks []scheduler.TaskFeature, reason string) []scheduler.ScoreResult {
	results := make([]scheduler.ScoreResult, len(tasks))
	for i, task := range tasks {
		results[i] = s.fallbackOne(task, reason)
	}
	return results
}

func (s *Scorer) fallbackOne(task scheduler.TaskFeature, reason string) scheduler.ScoreResult {
	score := s.calculator.Score(task).Result
	score.SchedulerType = scheduler.SchedulerTypeHeuristic
	score.FallbackReason = reason
	score.AnomalyStatus = scheduler.AnomalyStatusUnavailable
	return score
}

func (s *Scorer) recordAnomaly(task scheduler.TaskFeature, version string, status string) {
	if s.cfg.Metrics != nil {
		s.cfg.Metrics.IncSchedulerAnomalyStatus(version, string(task.RequestKind), task.CoverageLevel, status)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
