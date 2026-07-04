package scheduler

import (
	"context"
	"math"
	"strconv"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
)

const qualityBucketDuration = 5 * time.Minute

type PredictionQualityRecorder struct {
	Repo    controlstate.SchedulerQualityRollupRepository
	Metrics observability.Metrics
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
		return nil
	}
	r.recordMetrics(task, score, labels, mape)
	if r.Repo == nil {
		return nil
	}
	return r.Repo.Upsert(ctx, qualityRollup(task, labels, score, sampleID, mape))
}

type qualityScoreEvidence struct {
	SchedulerType        string
	SchedulerVersion     string
	TaskType             string
	PredictedLatencyMs   int64
	SchedulerCallLatency float64
	Confidence           float64
}

func scoreEvidence(task Task) qualityScoreEvidence {
	return qualityScoreEvidence{
		SchedulerType:        task.Metadata[schedulerTypeMetadata],
		SchedulerVersion:     task.Metadata[schedulerVersionMetadata],
		TaskType:             string(task.Feature.RequestKind),
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
		TaskType: score.TaskType, ModelClass: task.Feature.ModelClass, SampleCount: 1,
		MAPESum: mape, WaitMSSum: float64(labels.CompletedAt.Sub(task.EnqueueTime).Milliseconds()),
		SchedulerCallLatencyMSSum: score.SchedulerCallLatency, ConfidenceSum: score.Confidence,
		SafeSampleIDs: []string{sampleID},
	}
}

func (r *PredictionQualityRecorder) recordMetrics(task Task, score qualityScoreEvidence, labels TrainingLabels, mape float64) {
	if r.Metrics == nil {
		return
	}
	r.Metrics.RecordSchedulerPredictionMAPE(score.SchedulerType, score.SchedulerVersion, score.TaskType, mape)
	r.Metrics.RecordSchedulerComparisonWait(score.SchedulerType, score.SchedulerVersion, score.TaskType, float64(labels.CompletedAt.Sub(task.EnqueueTime).Milliseconds()))
	r.Metrics.RecordSchedulerComparisonCall(score.SchedulerType, score.SchedulerVersion, score.TaskType, score.SchedulerCallLatency)
}

func (r *PredictionQualityRecorder) incError(score qualityScoreEvidence) {
	if r.Metrics != nil {
		r.Metrics.IncSchedulerComparisonError(score.SchedulerType, score.SchedulerVersion, score.TaskType)
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
