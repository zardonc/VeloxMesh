package predictive

import (
	"context"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/schedulerv1"
)

type BatchScoreService struct {
	schedulerv1.UnimplementedTaskSchedulerServer
	scorer *Scorer
}

func NewBatchScoreService(scorer *Scorer) *BatchScoreService {
	return &BatchScoreService{scorer: scorer}
}

func (s *BatchScoreService) BatchScoreTasks(ctx context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
	features := make([]scheduler.TaskFeature, 0, len(req.GetTasks()))
	for _, task := range req.GetTasks() {
		features = append(features, featureFromProto(task))
	}
	scores, err := s.scorer.Score(ctx, features)
	if err != nil {
		return nil, err
	}
	resp := &schedulerv1.BatchScoreResponse{Results: make([]*schedulerv1.ScoreResult, 0, len(scores))}
	for _, score := range scores {
		resp.Results = append(resp.Results, scoreToProto(score))
	}
	return resp, nil
}

func scoreToProto(score scheduler.ScoreResult) *schedulerv1.ScoreResult {
	return &schedulerv1.ScoreResult{
		TaskId:             score.TaskID,
		Score:              score.Score,
		Priority:           string(score.Priority),
		PredictedLatencyMs: score.PredictedLatencyMs,
		Confidence:         score.Confidence,
		SchedulerVersion:   score.SchedulerVersion,
		Reason:             score.FallbackReason,
	}
}

func featureFromProto(task *schedulerv1.TaskFeature) scheduler.TaskFeature {
	if task == nil {
		return scheduler.TaskFeature{Priority: scheduler.PriorityNormal, RequestKind: scheduler.RequestKindSimpleQA}
	}
	return scheduler.TaskFeature{
		TaskID: task.GetTaskId(), ModelClass: task.GetModelClass(), EstimatedInputTokens: task.GetEstimatedInputTokens(),
		EstimatedOutputTokens: task.GetEstimatedOutputTokens(), Stream: task.GetStream(), Priority: scheduler.NormalizePriority(task.GetPriority()),
		TimeoutClass: task.GetTimeoutClass(), EnqueueTimeMs: task.GetEnqueueTimeMs(), RequestKind: normalizeRequestKind(task.GetRequestKind()),
		RouteHint: task.GetRouteHint(), HasToolCalls: task.GetHasToolCalls(), ToolCallDepth: task.GetToolCallDepth(), TurnCount: task.GetTurnCount(),
		Multimodal: task.GetMultimodal(), QuestionCount: task.GetQuestionCount(), CodeBlockCount: task.GetCodeBlockCount(),
		EnumerationHint: task.GetEnumerationHint(), InstructionVerbCount: task.GetInstructionVerbCount(),
		MaxSentenceLengthBucket: task.GetMaxSentenceLengthBucket(), VocabularyRichnessBucket: task.GetVocabularyRichnessBucket(),
		ConfidenceHint: task.GetConfidenceHint(), UncertaintyHint: task.GetUncertaintyHint(), NeighborCount: task.GetNeighborCount(),
		LatencyP50Ms: task.GetLatencyP50Ms(), LatencyP90Ms: task.GetLatencyP90Ms(), LatencyStddevMs: task.GetLatencyStddevMs(),
		OutputTokensP70: task.GetOutputTokensP70(), SuccessRate: task.GetSuccessRate(), TimeoutRate: task.GetTimeoutRate(),
		CoverageLevel: task.GetCoverageLevel(), CoverageRatio: task.GetCoverageRatio(),
	}
}

func normalizeRequestKind(value string) scheduler.RequestKind {
	kind := scheduler.RequestKind(value)
	switch kind {
	case scheduler.RequestKindSimpleQA, scheduler.RequestKindCodeGen, scheduler.RequestKindCodeReview, scheduler.RequestKindSummarization, scheduler.RequestKindTranslation, scheduler.RequestKindStructuredOutput, scheduler.RequestKindMultiStep, scheduler.RequestKindToolCall, scheduler.RequestKindRAG, scheduler.RequestKindCreative:
		return kind
	default:
		return scheduler.RequestKindSimpleQA
	}
}
