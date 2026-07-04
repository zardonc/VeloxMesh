package scheduler

import (
	"context"
	"errors"
	"time"

	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

type TaskIntake struct {
	Queue     QueueBackend
	Guard     QueueGuard
	Scorer    Scorer
	Registry  *ResultRegistry
	Priority  *PriorityResolver
	Policy    PriorityPolicy
	Metrics   observability.Metrics
	Backend   string
	RouteHint string
}

var ErrTaskIntakeNotConfigured = errors.New("task intake not configured")

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
	scoreStart := time.Now()
	scores, err := i.Scorer.Score(ctx, []TaskFeature{feature})
	if err != nil {
		scores, _ = FIFOScorer{Reason: "scheduler_error"}.Score(ctx, []TaskFeature{feature})
	}
	if len(scores) == 0 {
		scores, _ = FIFOScorer{Reason: "missing_score"}.Score(ctx, []TaskFeature{feature})
	}
	score := scores[0]
	i.recordSchedulerResult(score.FallbackReason, time.Since(scoreStart), score.FallbackReason)
	guard := i.Guard.Check(ctx, i.Queue, priority.Resolved)
	if guard.Err != nil {
		return Task{}, guard.Err
	}
	task := Task{
		ID:          req.RequestID,
		Feature:     feature,
		Score:       score.Score,
		EnqueueTime: now,
		State:       TaskStateQueued,
		Metadata:    map[string]string{schedulerVersionMetadata: score.SchedulerVersion},
	}
	i.Registry.Register(task.ID)
	i.Registry.RegisterHandler(task.ID, handler)
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
