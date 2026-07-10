package scheduler

import (
	"context"
	"errors"
	"strings"
	"time"

	"veloxmesh/internal/hotstate"
)

var ErrPriorityQuotaUnavailable = errors.New("priority quota unavailable")

type PriorityPolicy struct {
	Default            PriorityClass
	Max                PriorityClass
	HighQuotaPerMinute int64
	Strict             bool
}

type PriorityDecision struct {
	Declared        PriorityClass
	Resolved        PriorityClass
	DowngradeReason string
	Rejected        bool
	Err             error
}

type PriorityResolver struct {
	limiter hotstate.AtomicLimiter
}

func NewPriorityResolver(limiter hotstate.AtomicLimiter) *PriorityResolver {
	return &PriorityResolver{limiter: limiter}
}

func (r *PriorityResolver) Resolve(ctx context.Context, identityID string, trustedPriority string, structuredPriority string, policy PriorityPolicy) PriorityDecision {
	declared := NormalizePriority(firstNonEmpty(trustedPriority, structuredPriority, string(policy.Default)))
	if declared == "" {
		declared = PriorityNormal
	}
	maxPriority := policy.Max
	if maxPriority == "" {
		maxPriority = PriorityHigh
	}
	resolved := declared
	reason := ""
	if priorityRank(resolved) > priorityRank(maxPriority) {
		resolved = maxPriority
		reason = "policy"
	}
	if resolved == PriorityHigh && policy.HighQuotaPerMinute > 0 && r.limiter == nil {
		if policy.Strict {
			return PriorityDecision{Declared: declared, Resolved: PriorityNormal, DowngradeReason: "quota", Rejected: true, Err: ErrPriorityQuotaUnavailable}
		}
		resolved = PriorityNormal
		reason = "quota"
	}
	if resolved == PriorityHigh && policy.HighQuotaPerMinute > 0 && r.limiter != nil {
		key := "scheduler:priority_high:" + identityID
		_, allowed, err := r.limiter.CheckAndIncrement(ctx, key, policy.HighQuotaPerMinute, time.Minute)
		if err != nil && policy.Strict {
			return PriorityDecision{Declared: declared, Resolved: PriorityNormal, DowngradeReason: "quota", Rejected: true, Err: err}
		}
		if err != nil || !allowed {
			resolved = PriorityNormal
			reason = "quota"
		}
	}
	return PriorityDecision{Declared: declared, Resolved: resolved, DowngradeReason: reason}
}

func NormalizePriority(value string) PriorityClass {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "interactive":
		return PriorityHigh
	case "normal", "batch":
		return PriorityNormal
	case "low", "background":
		return PriorityLow
	default:
		return ""
	}
}

func priorityRank(priority PriorityClass) int {
	switch priority {
	case PriorityHigh:
		return 3
	case PriorityNormal:
		return 2
	case PriorityLow:
		return 1
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
