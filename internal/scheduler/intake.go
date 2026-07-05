package scheduler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

type TaskIntake struct {
	Queue                       QueueBackend
	Guard                       QueueGuard
	Scorer                      Scorer
	Registry                    *ResultRegistry
	Priority                    *PriorityResolver
	Policy                      PriorityPolicy
	Metrics                     observability.Metrics
	Backend                     string
	RouteHint                   string
	SemanticNeighbors           SemanticNeighborEnricher
	SemanticNeighborTaskTimeout time.Duration
}

var ErrTaskIntakeNotConfigured = errors.New("task intake not configured")

const (
	schedulerTypeMetadata         = "scheduler_type"
	schedulerPredictedLatencyMeta = "predicted_latency_ms"
	schedulerConfidenceMetadata   = "scheduler_confidence"
	schedulerCallLatencyMetadata  = "scheduler_call_latency_ms"
	schedulerAnomalyStatusMeta    = "scheduler_anomaly_status"
)

func (i *TaskIntake) Submit(ctx context.Context, req *llm.LLMRequest, handler TaskHandler) (Task, error) {
	if i.Queue == nil || i.Registry == nil || i.Scorer == nil || i.Priority == nil {
		return Task{}, ErrTaskIntakeNotConfigured
	}
	priority := i.Priority.Resolve(ctx, identityID(ctx), req.PriorityClass, "", i.Policy)
	if priority.Rejected {
		return Task{}, priority.Err
	}
	if priority.DowngradeReason != "" && i.Metrics != nil {
		i.Metrics.IncPriorityDowngrade(priority.DowngradeReason, string(priority.Declared), string(priority.Resolved))
	}
	now := time.Now()
	feature := ExtractSafeFeatures(req, priority.Resolved, i.RouteHint, now)
	feature = i.enrichFeatures(ctx, req, feature)
	scoreStart := time.Now()
	scores, err := i.Scorer.Score(ctx, []TaskFeature{feature})
	if err != nil {
		scores, _ = FIFOScorer{Reason: "scheduler_error"}.Score(ctx, []TaskFeature{feature})
	}
	if len(scores) == 0 {
		scores, _ = FIFOScorer{Reason: "missing_score"}.Score(ctx, []TaskFeature{feature})
	}
	score := scores[0]
	scoreLatency := time.Since(scoreStart)
	i.recordSchedulerResult(score.FallbackReason, scoreLatency, score.FallbackReason)
	guard := i.Guard.Check(ctx, i.Queue, priority.Resolved)
	if guard.Err != nil {
		return Task{}, guard.Err
	}
	task := Task{
		ID:          req.RequestID,
		TenantID:    identityID(ctx),
		TenantClass: identityClass(ctx),
		Feature:     feature,
		Score:       score.Score,
		EnqueueTime: now,
		State:       TaskStateQueued,
		Metadata:    scoreMetadata(score, scoreLatency),
	}
	i.Registry.RegisterTask(task, handler)
	if err := i.Queue.Push(ctx, QueueItem{TaskID: task.ID, Score: task.Score}); err != nil {
		i.Registry.Unregister(task.ID)
		if i.Metrics != nil {
			i.Metrics.IncSchedulerError("queue")
		}
		return Task{}, err
	}
	if i.Metrics != nil {
		if length, err := i.Queue.Len(ctx); err == nil {
			i.Metrics.RecordQueueDepth(i.Backend, string(priority.Resolved), length)
		}
	}
	return task, nil
}

func (i *TaskIntake) enrichFeatures(ctx context.Context, req *llm.LLMRequest, feature TaskFeature) TaskFeature {
	if i.SemanticNeighbors == nil {
		return feature
	}
	enrichCtx := ctx
	cancel := func() {}
	if i.SemanticNeighborTaskTimeout > 0 {
		enrichCtx, cancel = context.WithTimeout(ctx, i.SemanticNeighborTaskTimeout)
	}
	defer cancel()
	enriched, err := i.SemanticNeighbors.Enrich(enrichCtx, req, feature)
	if err != nil {
		i.recordSemanticNeighborError(err)
		return feature
	}
	return enriched
}

func (i *TaskIntake) recordSemanticNeighborError(err error) {
	if i.Metrics == nil {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		i.Metrics.IncSemanticNeighborTimeout()
		i.Metrics.IncSemanticNeighborFallback("timeout")
		return
	}
	i.Metrics.IncSemanticNeighborError("error")
	i.Metrics.IncSemanticNeighborFallback("error")
}

func scoreMetadata(score ScoreResult, latency time.Duration) map[string]string {
	metadata := map[string]string{
		schedulerVersionMetadata:      score.SchedulerVersion,
		schedulerTypeMetadata:         string(score.SchedulerType),
		schedulerPredictedLatencyMeta: strconv.FormatInt(score.PredictedLatencyMs, 10),
		schedulerConfidenceMetadata:   strconv.FormatFloat(score.Confidence, 'f', -1, 64),
		schedulerCallLatencyMetadata:  strconv.FormatInt(latency.Milliseconds(), 10),
	}
	if score.AnomalyStatus != "" {
		metadata[schedulerAnomalyStatusMeta] = score.AnomalyStatus
	}
	return metadata
}

func (i *TaskIntake) recordSchedulerResult(reason string, latency time.Duration, source string) {
	if i.Metrics == nil {
		return
	}
	result := schedulerCallResult(reason)
	i.Metrics.RecordSchedulerCall(result, float64(latency.Milliseconds()))
	if result == "timeout" || result == "error" {
		i.Metrics.IncSchedulerError(result)
	}
	i.Metrics.IncSchedulerClassificationSource(classificationSource(source))
}

func schedulerCallResult(reason string) string {
	switch reason {
	case "":
		return "ok"
	case "timeout":
		return "timeout"
	case "scheduler_error":
		return "error"
	default:
		return "fallback"
	}
}

func classificationSource(source string) string {
	switch source {
	case "structured", "rule", "fallback":
		return source
	default:
		return "fallback"
	}
}

func identityID(ctx context.Context) string {
	identity := middleware.GetAuthIdentity(ctx)
	if identity == nil || identity.ID == "" {
		return "anonymous"
	}
	return identity.ID
}

func identityClass(ctx context.Context) string {
	identity := middleware.GetAuthIdentity(ctx)
	if identity == nil || identity.Role == "" {
		return "anonymous"
	}
	return identity.Role
}
