package scheduler

import (
	"context"

	"veloxmesh/internal/scheduler/schedulerv1"
)

type PriorityClass string

const (
	PriorityHigh   PriorityClass = "high"
	PriorityNormal PriorityClass = "normal"
	PriorityLow    PriorityClass = "low"
)

type RequestKind string

const (
	RequestKindSimpleQA         RequestKind = "simple_qa"
	RequestKindCodeGen          RequestKind = "code_gen"
	RequestKindCodeReview       RequestKind = "code_review"
	RequestKindSummarization    RequestKind = "summarization"
	RequestKindTranslation      RequestKind = "translation"
	RequestKindStructuredOutput RequestKind = "structured_output"
	RequestKindMultiStep        RequestKind = "multi_step"
	RequestKindToolCall         RequestKind = "tool_call"
	RequestKindRAG              RequestKind = "rag"
	RequestKindCreative         RequestKind = "creative"
)

type SchedulerType string

const (
	SchedulerTypeFIFO       SchedulerType = "fifo"
	SchedulerTypeHeuristic  SchedulerType = "heuristic"
	SchedulerTypeONNX       SchedulerType = "onnx"
	SchedulerTypePredictive SchedulerType = "predictive"
)

const (
	SemanticCoverageNone     = "none"
	SemanticCoverageTenant   = "tenant"
	SemanticCoverageFallback = "fallback"
	SemanticCoverageAll      = "all"
)

const (
	AnomalyStatusNormal      = "normal"
	AnomalyStatusOOD         = "ood"
	AnomalyStatusUnavailable = "unavailable"
	AnomalyStatusDegraded    = "degraded"
)

type TaskFeature struct {
	TaskID                   string
	ModelClass               string
	EstimatedInputTokens     int64
	EstimatedOutputTokens    int64
	Stream                   bool
	Priority                 PriorityClass
	TimeoutClass             string
	EnqueueTimeMs            int64
	RequestKind              RequestKind
	RouteHint                string
	HasToolCalls             bool
	ToolCallDepth            int32
	TurnCount                int32
	Multimodal               bool
	QuestionCount            int32
	CodeBlockCount           int32
	EnumerationHint          bool
	InstructionVerbCount     int32
	MaxSentenceLengthBucket  int32
	VocabularyRichnessBucket int32
	ConfidenceHint           float64
	UncertaintyHint          float64
	NeighborCount            int64
	LatencyP50Ms             int64
	LatencyP90Ms             int64
	LatencyStddevMs          float64
	OutputTokensP70          int64
	SuccessRate              float64
	TimeoutRate              float64
	CoverageLevel            string
	CoverageRatio            float64
}

type ScoreResult struct {
	TaskID               string
	Score                float64
	Priority             PriorityClass
	PredictedLatencyMs   int64
	Confidence           float64
	SchedulerVersion     string
	SchedulerType        SchedulerType
	FallbackReason       string
	ClassificationSource string
	AnomalyStatus        string
}

type Scorer interface {
	Score(ctx context.Context, tasks []TaskFeature) ([]ScoreResult, error)
}

func (f TaskFeature) proto() *schedulerv1.TaskFeature {
	return &schedulerv1.TaskFeature{
		TaskId:                   f.TaskID,
		ModelClass:               f.ModelClass,
		EstimatedInputTokens:     f.EstimatedInputTokens,
		EstimatedOutputTokens:    f.EstimatedOutputTokens,
		Stream:                   f.Stream,
		Priority:                 string(f.Priority),
		TimeoutClass:             f.TimeoutClass,
		EnqueueTimeMs:            f.EnqueueTimeMs,
		RequestKind:              string(f.RequestKind),
		RouteHint:                f.RouteHint,
		HasToolCalls:             f.HasToolCalls,
		ToolCallDepth:            f.ToolCallDepth,
		TurnCount:                f.TurnCount,
		Multimodal:               f.Multimodal,
		QuestionCount:            f.QuestionCount,
		CodeBlockCount:           f.CodeBlockCount,
		EnumerationHint:          f.EnumerationHint,
		InstructionVerbCount:     f.InstructionVerbCount,
		MaxSentenceLengthBucket:  f.MaxSentenceLengthBucket,
		VocabularyRichnessBucket: f.VocabularyRichnessBucket,
		ConfidenceHint:           f.ConfidenceHint,
		UncertaintyHint:          f.UncertaintyHint,
		NeighborCount:            f.NeighborCount,
		LatencyP50Ms:             f.LatencyP50Ms,
		LatencyP90Ms:             f.LatencyP90Ms,
		LatencyStddevMs:          f.LatencyStddevMs,
		OutputTokensP70:          f.OutputTokensP70,
		SuccessRate:              f.SuccessRate,
		TimeoutRate:              f.TimeoutRate,
		CoverageLevel:            f.CoverageLevel,
		CoverageRatio:            f.CoverageRatio,
	}
}

func scoreFromProto(r *schedulerv1.ScoreResult) ScoreResult {
	fallbackReason, classificationSource := splitLegacyScoreReason(r.GetReason())
	return ScoreResult{
		TaskID:               r.GetTaskId(),
		Score:                r.GetScore(),
		Priority:             PriorityClass(r.GetPriority()),
		PredictedLatencyMs:   r.GetPredictedLatencyMs(),
		Confidence:           r.GetConfidence(),
		SchedulerVersion:     r.GetSchedulerVersion(),
		FallbackReason:       fallbackReason,
		ClassificationSource: classificationSource,
	}
}

func splitLegacyScoreReason(reason string) (string, string) {
	switch reason {
	case "", "structured", "rule", "fallback", "onnx":
		return "", reason
	default:
		return reason, ""
	}
}
