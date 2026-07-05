package scheduler

import (
	"context"
	"math"
	"testing"
	"time"

	"veloxmesh/internal/config"
)

func TestSLAPromoterPromotesEligibleSamePriorityTask(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "first", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 1, enqueue: now.Add(-time.Second),
	})
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "eligible", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 2, enqueue: now.Add(-3 * time.Second),
	})
	promoter := testSLAPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomePromoted || result.TaskID != "eligible" {
		t.Fatalf("unexpected promotion result: %#v", result)
	}
	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "eligible" || !(got.Score < 1) || math.IsInf(got.Score, 0) {
		t.Fatalf("eligible task was not promoted safely: %#v", got)
	}
}

func TestSLAPromoterBlocksPriorityBoundaryCrossing(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "high", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityHigh, score: 1, enqueue: now.Add(-time.Second),
	})
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "eligible-normal", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 2, enqueue: now.Add(-3 * time.Second),
	})
	promoter := testSLAPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomeBlockedByPriorityOrQuota {
		t.Fatalf("expected blocked outcome, got %#v", result)
	}
	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "high" {
		t.Fatalf("promotion crossed priority boundary: %#v", got)
	}
}

func TestSLAPromoterNoMatchPreservesQueueOrder(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "first", tenantID: "tenant-b", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 1, enqueue: now.Add(-3 * time.Second),
	})
	promoter := testSLAPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomeNotEligible {
		t.Fatalf("expected not eligible, got %#v", result)
	}
	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "first" {
		t.Fatalf("queue order changed without matching rule: %#v", got)
	}
}

func TestSLAPromoterCandidateWindowLimitsInspection(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "first", tenantID: "tenant-b", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 1, enqueue: now.Add(-time.Second),
	})
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "outside-window", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 2, enqueue: now.Add(-3 * time.Second),
	})
	promoter := testSLAPromoter(queue, registry)
	promoter.CandidateWindow = 1

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomeNotEligible {
		t.Fatalf("expected not eligible, got %#v", result)
	}
	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "first" {
		t.Fatalf("candidate outside window changed order: %#v", got)
	}
}

func TestSLAPromoterIgnoresPromptDerivedUrgencyFields(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	task := registerPromotionTask(t, registry, queue, promotionTask{
		id: "urgent-looking", tenantID: "tenant-a", modelClass: "small", requestKind: RequestKindSimpleQA,
		priority: PriorityNormal, score: 1, enqueue: now.Add(-3 * time.Second),
	})
	task.Feature.QuestionCount = 99
	task.Feature.CodeBlockCount = 99
	task.Feature.EnumerationHint = true
	task.Feature.InstructionVerbCount = 99
	task.Feature.MaxSentenceLengthBucket = 4
	task.Feature.VocabularyRichnessBucket = 4
	registry.RegisterTask(task, func(context.Context) TaskResult { return TaskResult{} })
	promoter := testSLAPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomeNotEligible {
		t.Fatalf("prompt-derived fields affected promotion: %#v", result)
	}
}

type promotionTask struct {
	id          string
	tenantID    string
	tenantClass string
	modelClass  string
	requestKind RequestKind
	priority    PriorityClass
	score       float64
	enqueue     time.Time
}

func registerPromotionTask(t *testing.T, registry *ResultRegistry, queue QueueBackend, in promotionTask) Task {
	t.Helper()
	task := Task{
		ID:          in.id,
		TenantID:    in.tenantID,
		TenantClass: in.tenantClass,
		Feature: TaskFeature{
			TaskID:        in.id,
			ModelClass:    in.modelClass,
			RequestKind:   in.requestKind,
			Priority:      in.priority,
			EnqueueTimeMs: in.enqueue.UnixMilli(),
		},
		Score:       in.score,
		EnqueueTime: in.enqueue,
		State:       TaskStateQueued,
	}
	registry.RegisterTask(task, func(context.Context) TaskResult { return TaskResult{} })
	if err := queue.Push(context.Background(), QueueItem{TaskID: in.id, Score: in.score}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	return task
}

func testSLAPromoter(queue QueueBackend, registry *ResultRegistry) *SLAPromoter {
	return &SLAPromoter{
		Enabled:         true,
		CandidateWindow: 8,
		Queue:           queue,
		Registry:        registry,
		Rules: []config.SLAPromotionRule{{
			PolicyID:      "tier-gold-code",
			TenantID:      "tenant-a",
			ModelClass:    "large",
			RequestKind:   string(RequestKindCodeGen),
			WaitThreshold: "2s",
		}},
	}
}
