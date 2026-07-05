package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
)

type SLAPromotionOutcome string

const (
	SLAPromotionOutcomePromoted                 SLAPromotionOutcome = "promoted"
	SLAPromotionOutcomeNotEligible              SLAPromotionOutcome = "not_eligible"
	SLAPromotionOutcomeBlockedByPriorityOrQuota SLAPromotionOutcome = "blocked_by_priority_or_quota"
	SLAPromotionOutcomeDisabled                 SLAPromotionOutcome = "disabled"
	SLAPromotionOutcomeError                    SLAPromotionOutcome = "error"
)

type SLAPromotionResult struct {
	Outcome     SLAPromotionOutcome
	TaskID      string
	PolicyID    string
	TenantID    string
	TenantClass string
	ModelClass  string
	RequestKind string
	Priority    PriorityClass
}

type SLAPromoter struct {
	Enabled         bool
	CandidateWindow int
	Rules           []config.SLAPromotionRule
	Queue           QueueBackend
	Registry        *ResultRegistry
	Audit           controlstate.AuditRepository
	Logger          *slog.Logger
}

func (p *SLAPromoter) PromoteBeforePop(ctx context.Context, now time.Time) (SLAPromotionResult, error) {
	if p == nil || !p.Enabled {
		return SLAPromotionResult{Outcome: SLAPromotionOutcomeDisabled}, nil
	}
	if p.Queue == nil || p.Registry == nil || p.CandidateWindow < 1 {
		return SLAPromotionResult{Outcome: SLAPromotionOutcomeNotEligible}, nil
	}
	items, err := p.Queue.PeekMin(ctx, p.CandidateWindow)
	if err != nil {
		result := SLAPromotionResult{Outcome: SLAPromotionOutcomeError}
		p.emitEvidence(ctx, result)
		return result, err
	}
	result, score, ok := p.selectCandidate(items, now)
	if !ok || result.Outcome != SLAPromotionOutcomePromoted {
		p.emitEvidence(ctx, result)
		return result, nil
	}
	if err := p.Queue.Push(ctx, QueueItem{TaskID: result.TaskID, Score: score}); err != nil {
		result.Outcome = SLAPromotionOutcomeError
		p.emitEvidence(ctx, result)
		return result, err
	}
	p.emitEvidence(ctx, result)
	return result, nil
}

func (p *SLAPromoter) selectCandidate(items []QueueItem, now time.Time) (SLAPromotionResult, float64, bool) {
	firstScore := map[PriorityClass]float64{}
	highestEarlierRank := 0
	for _, item := range items {
		task, ok := p.Registry.Task(item.TaskID)
		if !ok {
			continue
		}
		priority := task.Feature.Priority
		if _, ok := firstScore[priority]; !ok {
			firstScore[priority] = item.Score
		}
		rule, matched := p.matchRule(task)
		if matched && waitedLongEnough(task, rule, now) {
			return p.candidateResult(task, rule, highestEarlierRank, firstScore[priority])
		}
		highestEarlierRank = max(highestEarlierRank, priorityRank(priority))
	}
	return SLAPromotionResult{Outcome: SLAPromotionOutcomeNotEligible}, 0, false
}

func (p *SLAPromoter) candidateResult(task Task, rule config.SLAPromotionRule, highestEarlierRank int, firstScore float64) (SLAPromotionResult, float64, bool) {
	result := promotionResult(task, rule, SLAPromotionOutcomePromoted)
	if highestEarlierRank > priorityRank(task.Feature.Priority) {
		result.Outcome = SLAPromotionOutcomeBlockedByPriorityOrQuota
		return result, 0, true
	}
	return result, math.Nextafter(firstScore, math.Inf(-1)), true
}

func (p *SLAPromoter) matchRule(task Task) (config.SLAPromotionRule, bool) {
	for _, rule := range p.Rules {
		tenantMatch := rule.TenantID != "" && rule.TenantID == task.TenantID
		classMatch := rule.TenantClass != "" && rule.TenantClass == task.TenantClass
		if !tenantMatch && !classMatch {
			continue
		}
		if rule.ModelClass == task.Feature.ModelClass && rule.RequestKind == string(task.Feature.RequestKind) {
			return rule, true
		}
	}
	return config.SLAPromotionRule{}, false
}

func waitedLongEnough(task Task, rule config.SLAPromotionRule, now time.Time) bool {
	threshold, err := time.ParseDuration(rule.WaitThreshold)
	return err == nil && now.Sub(task.EnqueueTime) >= threshold
}

func promotionResult(task Task, rule config.SLAPromotionRule, outcome SLAPromotionOutcome) SLAPromotionResult {
	return SLAPromotionResult{
		Outcome:     outcome,
		TaskID:      task.ID,
		PolicyID:    rule.PolicyID,
		TenantID:    task.TenantID,
		TenantClass: task.TenantClass,
		ModelClass:  task.Feature.ModelClass,
		RequestKind: string(task.Feature.RequestKind),
		Priority:    task.Feature.Priority,
	}
}

func (p *SLAPromoter) emitEvidence(ctx context.Context, result SLAPromotionResult) {
	switch result.Outcome {
	case SLAPromotionOutcomePromoted:
		p.writeAudit(ctx, result)
		p.writeLog(slog.LevelInfo, result)
	case SLAPromotionOutcomeBlockedByPriorityOrQuota:
		p.writeAudit(ctx, result)
		p.writeLog(slog.LevelWarn, result)
	case SLAPromotionOutcomeError:
		p.writeLog(slog.LevelWarn, result)
	}
}

func (p *SLAPromoter) writeAudit(ctx context.Context, result SLAPromotionResult) {
	if p.Audit == nil {
		return
	}
	now := time.Now().UTC()
	_ = p.Audit.Log(ctx, &controlstate.AuditEvent{
		ID:        fmt.Sprintf("scheduler.sla_promotion-%d", now.UnixNano()),
		Actor:     "system",
		Action:    "scheduler.sla_promotion",
		TargetID:  result.PolicyID,
		Outcome:   string(result.Outcome),
		Metadata:  controlstate.SafeAuditMetadata(slaEvidence(result)),
		Timestamp: now,
	})
}

func (p *SLAPromoter) writeLog(level slog.Level, result SLAPromotionResult) {
	if p.Logger == nil {
		return
	}
	p.Logger.Log(context.Background(), level, "scheduler SLA promotion",
		"policy_id", result.PolicyID,
		"tenant_id", result.TenantID,
		"tenant_class", result.TenantClass,
		"model_class", result.ModelClass,
		"request_kind", result.RequestKind,
		"priority", string(result.Priority),
		"outcome", string(result.Outcome),
	)
}

func slaEvidence(result SLAPromotionResult) map[string]interface{} {
	return map[string]interface{}{
		"policy_id":    result.PolicyID,
		"tenant_id":    result.TenantID,
		"tenant_class": result.TenantClass,
		"model_class":  result.ModelClass,
		"request_kind": result.RequestKind,
		"priority":     string(result.Priority),
		"outcome":      string(result.Outcome),
	}
}
