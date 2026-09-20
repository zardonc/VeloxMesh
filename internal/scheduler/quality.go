package scheduler

import (
	"context"
	"math"
	"strconv"
	"sync"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
)

const qualityBucketDuration = 5 * time.Minute
const qualityWarmupMaxSamples = 20

type PredictionQualityRecorder struct {
	Repo        controlstate.SchedulerQualityRollupRepository
	Metrics     observability.Metrics
	Controller  *SchedulerRolloutController
	mu          sync.Mutex
	samples     []onnxQualitySample
	mapeActive  bool
	errorActive bool
}

type onnxQualitySample struct {
	validMAPE bool
	mape      float64
	err       bool
}

type qualityWindowStats struct {
	total   int
	errors  int
	valid   int
	mapeSum float64
}

func CalculateMAPE(predictedMs, actualMs int64) (float64, bool) {
	if predictedMs <= 0 || actualMs <= 0 {
		return 0, false
	}
	return math.Abs(float64(predictedMs-actualMs)) / float64(actualMs) * 100, true
}

func (r *PredictionQualityRecorder) Record(ctx context.Context, task Task, labels TrainingLabels, sampleID string) error {
	if r == nil {
		return nil
	}
	score := scoreEvidence(task)
	mape, ok := CalculateMAPE(score.PredictedLatencyMs, labels.ActualLatencyMs)
	if !ok {
		r.incError(score)
		r.recordErrorSpikeAlert(score)
		return nil
	}
	r.recordMetrics(task, score, labels, mape)
	r.recordMAPEAlert(score, mape)
	if r.Repo == nil {
		return nil
	}
	return r.Repo.Upsert(ctx, qualityRollup(task, labels, score, sampleID, mape))
}

type qualityScoreEvidence struct {
	SchedulerType        string
	SchedulerVersion     string
	TaskType             string
	CoverageLevel        string
	AnomalyStatus        string
	PredictedLatencyMs   int64
	SchedulerCallLatency float64
	Confidence           float64
}

func scoreEvidence(task Task) qualityScoreEvidence {
	return qualityScoreEvidence{
		SchedulerType:        task.Metadata[schedulerTypeMetadata],
		SchedulerVersion:     task.Metadata[schedulerVersionMetadata],
		TaskType:             string(task.Feature.RequestKind),
		CoverageLevel:        coverageLevel(task.Feature.CoverageLevel),
		AnomalyStatus:        anomalyStatus(task.Metadata[schedulerAnomalyStatusMeta]),
		PredictedLatencyMs:   parseInt64(task.Metadata[schedulerPredictedLatencyMeta]),
		SchedulerCallLatency: float64(parseInt64(task.Metadata[schedulerCallLatencyMetadata])),
		Confidence:           parseFloat(task.Metadata[schedulerConfidenceMetadata]),
	}
}

func qualityRollup(task Task, labels TrainingLabels, score qualityScoreEvidence, sampleID string, mape float64) *controlstate.SchedulerQualityRollup {
	bucketStart := labels.CompletedAt.Truncate(qualityBucketDuration)
	return &controlstate.SchedulerQualityRollup{
		BucketStart: bucketStart, BucketEnd: bucketStart.Add(qualityBucketDuration),
		SchedulerType: score.SchedulerType, SchedulerVersion: score.SchedulerVersion,
		TaskType: score.TaskType, CoverageLevel: score.CoverageLevel, ModelClass: task.Feature.ModelClass, SampleCount: 1,
		MAPESum: mape, WaitMSSum: float64(labels.CompletedAt.Sub(task.EnqueueTime).Milliseconds()),
		SchedulerCallLatencyMSSum: score.SchedulerCallLatency, ConfidenceSum: score.Confidence,
		AnomalyCount: anomalyCount(score.AnomalyStatus), AnomalyRate: float64(anomalyCount(score.AnomalyStatus)),
		AnomalyUnavailableCount: anomalyUnavailableCount(score.AnomalyStatus),
		SafeSampleIDs:           []string{sampleID},
	}
}

func (r *PredictionQualityRecorder) recordMetrics(task Task, score qualityScoreEvidence, labels TrainingLabels, mape float64) {
	if r.Metrics == nil {
		return
	}
	r.Metrics.RecordSchedulerPredictionMAPE(score.SchedulerType, score.SchedulerVersion, score.TaskType, score.CoverageLevel, score.AnomalyStatus, mape)
	r.Metrics.RecordSchedulerComparisonWait(score.SchedulerType, score.SchedulerVersion, score.TaskType, score.CoverageLevel, score.AnomalyStatus, float64(labels.CompletedAt.Sub(task.EnqueueTime).Milliseconds()))
	r.Metrics.RecordSchedulerComparisonCall(score.SchedulerType, score.SchedulerVersion, score.TaskType, score.CoverageLevel, score.AnomalyStatus, score.SchedulerCallLatency)
	r.Metrics.IncSchedulerAnomalyStatus(score.SchedulerVersion, score.TaskType, score.CoverageLevel, score.AnomalyStatus)
}

func (r *PredictionQualityRecorder) incError(score qualityScoreEvidence) {
	if r.Metrics != nil {
		r.Metrics.IncSchedulerComparisonError(score.SchedulerType, score.SchedulerVersion, score.TaskType)
	}
}

func (r *PredictionQualityRecorder) recordMAPEAlert(score qualityScoreEvidence, mape float64) {
	r.recordONNXQualitySample(score, onnxQualitySample{validMAPE: true, mape: mape})
}

func (r *PredictionQualityRecorder) recordErrorSpikeAlert(score qualityScoreEvidence) {
	r.recordONNXQualitySample(score, onnxQualitySample{err: true})
}

func (r *PredictionQualityRecorder) recordONNXQualitySample(score qualityScoreEvidence, sample onnxQualitySample) {
	if score.SchedulerType != string(SchedulerTypeONNX) {
		return
	}
	status := SchedulerRolloutStatus{}
	if r.Controller != nil {
		status = r.Controller.Snapshot()
	}
	window := qualitySampleWindowOrDefault(status.QualitySampleWindow)
	alerts := r.recordQualitySampleLocked(sample, status, window)
	for _, alert := range alerts {
		r.recordAlert(alert.reason, alert.message)
	}
}

type qualityAlert struct {
	reason  string
	message string
}

func (r *PredictionQualityRecorder) recordQualitySampleLocked(sample onnxQualitySample, status SchedulerRolloutStatus, window int) []qualityAlert {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, sample)
	if len(r.samples) > window {
		r.samples = r.samples[len(r.samples)-window:]
	}
	stats := qualityStats(r.samples)
	minSamples := min(qualityWarmupMaxSamples, window)
	alerts := []qualityAlert{}
	if stats.valid >= minSamples && status.QualityMAPEAlertPercent > 0 {
		above := stats.mapeSum/float64(stats.valid) > status.QualityMAPEAlertPercent
		if above && !r.mapeActive {
			alerts = append(alerts, qualityAlert{reason: RolloutAlertMAPEDegradation, message: "ONNX scheduler MAPE exceeded configured threshold"})
		}
		r.mapeActive = above
	} else {
		r.mapeActive = false
	}
	if stats.total >= minSamples && status.ErrorSpikeAlertRate > 0 {
		above := float64(stats.errors)/float64(stats.total) > status.ErrorSpikeAlertRate
		if above && !r.errorActive {
			alerts = append(alerts, qualityAlert{reason: RolloutAlertSchedulerErrorSpike, message: "ONNX scheduler error rate exceeded configured threshold"})
		}
		r.errorActive = above
	} else {
		r.errorActive = false
	}
	return alerts
}

func qualityStats(samples []onnxQualitySample) qualityWindowStats {
	stats := qualityWindowStats{total: len(samples)}
	for _, sample := range samples {
		if sample.err {
			stats.errors++
		}
		if sample.validMAPE {
			stats.valid++
			stats.mapeSum += sample.mape
		}
	}
	return stats
}

func (r *PredictionQualityRecorder) recordAlert(reason string, message string) {
	if r.Controller != nil {
		r.Controller.RecordAlert(reason, message)
	}
	if r.Metrics != nil {
		r.Metrics.IncSchedulerRolloutAlert(reason)
	}
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func parseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func anomalyCount(status string) int64 {
	if status == AnomalyStatusOOD {
		return 1
	}
	return 0
}

func anomalyUnavailableCount(status string) int64 {
	if status == AnomalyStatusUnavailable || status == AnomalyStatusDegraded {
		return 1
	}
	return 0
}

func anomalyStatus(status string) string {
	switch status {
	case AnomalyStatusOOD, AnomalyStatusUnavailable, AnomalyStatusDegraded:
		return status
	default:
		return AnomalyStatusNormal
	}
}

func coverageLevel(level string) string {
	switch level {
	case SemanticCoverageTenant, SemanticCoverageFallback, SemanticCoverageAll:
		return level
	default:
		return SemanticCoverageNone
	}
}
