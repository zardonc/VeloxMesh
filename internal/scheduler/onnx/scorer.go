package onnx

import (
	"context"
	"math"

	"veloxmesh/internal/observability"
	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/heuristic"
)

type Scorer struct {
	artifact   *Artifact
	calculator *heuristic.ScoreCalculator
	metrics    observability.Metrics
	loads      int
}

func NewScorer(artifactDir string) (*Scorer, error) {
	return NewScorerWithMetrics(artifactDir, nil)
}

func NewScorerWithMetrics(artifactDir string, metrics observability.Metrics) (*Scorer, error) {
	artifact, err := LoadArtifact(artifactDir)
	if err != nil {
		return nil, err
	}
	return &Scorer{artifact: artifact, calculator: heuristic.NewScoreCalculator(heuristic.DefaultConfig()), metrics: metrics, loads: 1}, nil
}

func (s *Scorer) LoadCount() int {
	return s.loads
}

func (s *Scorer) AnomalyStatus() string {
	return s.artifact.AnomalyStatus
}

func (s *Scorer) AnomalyReason() string {
	return s.artifact.AnomalyReason
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
	adjustment := s.anomalyAdjustment(task)
	if adjustment.status == scheduler.AnomalyStatusOOD {
		task.ConfidenceHint = adjustment.confidence
		task.UncertaintyHint += math.Min(adjustment.severity, 5)
	}
	predictedTokens := int64(math.Round(s.artifact.Runner.P70OutputTokens()))
	if predictedTokens > 0 {
		task.EstimatedOutputTokens = predictedTokens
	}
	score := s.calculator.Score(task).Result
	score.Confidence = adjustment.confidence
	score.SchedulerVersion = s.artifact.Manifest.SchedulerVersion
	score.SchedulerType = scheduler.SchedulerTypeONNX
	score.FallbackReason = "onnx"
	score.AnomalyStatus = adjustment.status
	s.recordAnomaly(task, adjustment.status)
	return score
}

func (s *Scorer) recordAnomaly(task scheduler.TaskFeature, status string) {
	if s.metrics != nil {
		s.metrics.IncSchedulerAnomalyStatus(s.artifact.Manifest.SchedulerVersion, string(task.RequestKind), task.CoverageLevel, status)
	}
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

type anomalyAdjustment struct {
	status     string
	confidence float64
	severity   float64
}

func (s *Scorer) anomalyAdjustment(task scheduler.TaskFeature) anomalyAdjustment {
	confidence := s.confidence(task)
	if s.artifact.AnomalyStatus == AnomalyStatusUnavailable {
		return anomalyAdjustment{status: scheduler.AnomalyStatusUnavailable, confidence: confidence}
	}
	if s.artifact.AnomalyStatus == AnomalyStatusDegraded {
		return anomalyAdjustment{status: scheduler.AnomalyStatusDegraded, confidence: confidence}
	}
	threshold, ok := s.thresholdFor(task)
	if !ok {
		return anomalyAdjustment{status: scheduler.AnomalyStatusUnavailable, confidence: confidence}
	}
	severity := anomalySeverity(anomalyDistance(task), threshold.Threshold)
	if severity <= 0 {
		return anomalyAdjustment{status: scheduler.AnomalyStatusNormal, confidence: confidence}
	}
	return anomalyAdjustment{
		status:     scheduler.AnomalyStatusOOD,
		confidence: math.Max(0.05, confidence*(1/(1+severity))),
		severity:   severity,
	}
}

func (s *Scorer) thresholdFor(task scheduler.TaskFeature) (AnomalyThreshold, bool) {
	byCoverage, ok := s.artifact.Manifest.AnomalyThresholds[string(task.RequestKind)]
	if !ok {
		return AnomalyThreshold{}, false
	}
	if threshold, ok := byCoverage[coverageLevel(task.CoverageLevel)]; ok {
		return threshold, true
	}
	threshold, ok := byCoverage[scheduler.SemanticCoverageAll]
	return threshold, ok
}

func anomalyDistance(task scheduler.TaskFeature) float64 {
	latencyP50 := math.Max(float64(task.LatencyP50Ms), 1)
	latencySpread := math.Max(float64(task.LatencyP90Ms-task.LatencyP50Ms), 0) / latencyP50
	successGap := math.Max(1-task.SuccessRate, 0)
	coverageGap := math.Max(1-task.CoverageRatio, 0)
	return latencySpread + task.TimeoutRate + successGap + coverageGap
}

func anomalySeverity(distance float64, threshold float64) float64 {
	if threshold <= 0 {
		return 0
	}
	return math.Max((distance-threshold)/threshold, 0)
}

func coverageLevel(value string) string {
	switch value {
	case scheduler.SemanticCoverageTenant, scheduler.SemanticCoverageFallback, scheduler.SemanticCoverageAll:
		return value
	default:
		return scheduler.SemanticCoverageNone
	}
}
