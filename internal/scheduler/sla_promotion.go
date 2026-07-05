package scheduler

import (
	"context"
	"math"
	"time"

	"veloxmesh/internal/config"
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
		return SLAPromotionResult{Outcome: SLAPromotionOutcomeError}, err
	}
	result, score, ok := p.selectCandidate(items, now)
	if !ok || result.Outcome != SLAPromotionOutcomePromoted {
		return result, nil
	}
	if err := p.Queue.Push(ctx, QueueItem{TaskID: result.TaskID, Score: score}); err != nil {
		result.Outcome = SLAPromotionOutcomeError
		return result, err
	}
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
