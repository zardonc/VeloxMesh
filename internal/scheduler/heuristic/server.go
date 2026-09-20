package heuristic

import (
	"context"
	"time"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/schedulerv1"
)

type BatchScoreService struct {
	schedulerv1.UnimplementedTaskSchedulerServer
	calculator *ScoreCalculator
	metrics    *Metrics
}

func NewBatchScoreService(calculator *ScoreCalculator, metrics *Metrics) *BatchScoreService {
	if calculator == nil {
		calculator = NewScoreCalculator(DefaultConfig())
	}
	return &BatchScoreService{calculator: calculator, metrics: metrics}
}

func (s *BatchScoreService) BatchScoreTasks(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
	start := time.Now()
	resp := &schedulerv1.BatchScoreResponse{Results: make([]*schedulerv1.ScoreResult, 0, len(req.GetTasks()))}
	source := "fallback"
	for _, task := range req.GetTasks() {
		score := s.calculator.Score(featureFromProto(task))
		source = score.ClassificationSource
		resp.Results = append(resp.Results, &schedulerv1.ScoreResult{
			TaskId:             score.Result.TaskID,
			Score:              score.Result.Score,
			Priority:           string(score.Result.Priority),
			PredictedLatencyMs: score.Result.PredictedLatencyMs,
			Confidence:         score.Result.Confidence,
			SchedulerVersion:   score.Result.SchedulerVersion,
			Reason:             score.Result.FallbackReason,
		})
	}
	if s.metrics != nil {
		s.metrics.Observe(float64(time.Since(start).Milliseconds()), source, len(req.GetTasks()))
	}
	return resp, nil
}

func featureFromProto(task *schedulerv1.TaskFeature) scheduler.TaskFeature {
	if task == nil {
		return scheduler.TaskFeature{Priority: scheduler.PriorityNormal, RequestKind: scheduler.RequestKindSimpleQA}
	}
	priority := scheduler.PriorityClass(task.GetPriority())
	if priority != scheduler.PriorityHigh && priority != scheduler.PriorityNormal && priority != scheduler.PriorityLow {
		priority = scheduler.PriorityNormal
	}
	kind := scheduler.RequestKind(task.GetRequestKind())
	switch kind {
	case scheduler.RequestKindSimpleQA, scheduler.RequestKindCodeGen, scheduler.RequestKindCodeReview, scheduler.RequestKindSummarization, scheduler.RequestKindTranslation, scheduler.RequestKindStructuredOutput, scheduler.RequestKindMultiStep, scheduler.RequestKindToolCall, scheduler.RequestKindRAG, scheduler.RequestKindCreative:
	default:
		kind = scheduler.RequestKindSimpleQA
	}
	return scheduler.TaskFeature{
		TaskID:                task.GetTaskId(),
		ModelClass:            task.GetModelClass(),
		EstimatedInputTokens:  task.GetEstimatedInputTokens(),
		EstimatedOutputTokens: task.GetEstimatedOutputTokens(),
		Stream:                task.GetStream(),
		Priority:              priority,
		EnqueueTimeMs:         task.GetEnqueueTimeMs(),
		RequestKind:           kind,
		HasToolCalls:          task.GetHasToolCalls(),
		ToolCallDepth:         task.GetToolCallDepth(),
		TurnCount:             task.GetTurnCount(),
		ConfidenceHint:        task.GetConfidenceHint(),
		UncertaintyHint:       task.GetUncertaintyHint(),
	}
}
