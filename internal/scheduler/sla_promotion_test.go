package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
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

func TestSLAPromoterPromotedWritesSanitizedAuditAndLog(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "eligible", tenantID: "tenant-a", tenantClass: "gold", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 2, enqueue: now.Add(-3 * time.Second),
	})
	promoter, audit, logs := auditedPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomePromoted {
		t.Fatalf("expected promoted, got %#v", result)
	}
	assertAuditEvent(t, audit, "tier-gold-code", "promoted")
	assertSanitizedLog(t, logs.String(), "promoted")
}

func TestSLAPromoterBlockedWritesSanitizedAuditAndLog(t *testing.T) {
	ctx := context.Background()
	queue := NewMemoryQueue()
	registry := NewResultRegistry()
	now := time.Now()
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "high", tenantID: "tenant-a", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityHigh, score: 1, enqueue: now.Add(-time.Second),
	})
	registerPromotionTask(t, registry, queue, promotionTask{
		id: "blocked", tenantID: "tenant-a", tenantClass: "gold", modelClass: "large", requestKind: RequestKindCodeGen,
		priority: PriorityNormal, score: 2, enqueue: now.Add(-3 * time.Second),
	})
	promoter, audit, logs := auditedPromoter(queue, registry)

	result, err := promoter.PromoteBeforePop(ctx, now)
	if err != nil {
		t.Fatalf("PromoteBeforePop: %v", err)
	}
	if result.Outcome != SLAPromotionOutcomeBlockedByPriorityOrQuota {
		t.Fatalf("expected blocked, got %#v", result)
	}
	assertAuditEvent(t, audit, "tier-gold-code", "blocked_by_priority_or_quota")
	assertSanitizedLog(t, logs.String(), "blocked_by_priority_or_quota")
}

func TestSLAPromoterSkipsAuditForDisabledNotEligibleAndError(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name     string
		promoter *SLAPromoter
	}{
		{name: "disabled", promoter: &SLAPromoter{Enabled: false}},
		{name: "not eligible", promoter: testSLAPromoter(NewMemoryQueue(), NewResultRegistry())},
		{name: "error", promoter: &SLAPromoter{Enabled: true, CandidateWindow: 1, Queue: errorQueue{}, Registry: NewResultRegistry()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			audit := &recordingAuditRepo{}
			logs := &bytes.Buffer{}
			tc.promoter.Audit = audit
			tc.promoter.Logger = slog.New(slog.NewJSONHandler(logs, nil))
			_, _ = tc.promoter.PromoteBeforePop(ctx, time.Now())
			if len(audit.events) != 0 {
				t.Fatalf("unexpected audit events: %#v", audit.events)
			}
			if tc.name == "error" {
				assertSanitizedLog(t, logs.String(), "error")
			}
		})
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

func auditedPromoter(queue QueueBackend, registry *ResultRegistry) (*SLAPromoter, *recordingAuditRepo, *bytes.Buffer) {
	audit := &recordingAuditRepo{}
	logs := &bytes.Buffer{}
	promoter := testSLAPromoter(queue, registry)
	promoter.Audit = audit
	promoter.Logger = slog.New(slog.NewJSONHandler(logs, nil))
	return promoter, audit, logs
}

type recordingAuditRepo struct {
	events []*controlstate.AuditEvent
}

func (r *recordingAuditRepo) Log(_ context.Context, event *controlstate.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *recordingAuditRepo) List(context.Context, string) ([]*controlstate.AuditEvent, error) {
	return nil, nil
}

func (r *recordingAuditRepo) PurgeOld(context.Context, string) (int64, error) {
	return 0, nil
}

type errorQueue struct{}

func (errorQueue) Push(context.Context, QueueItem) error { return context.Canceled }
func (errorQueue) PeekMin(context.Context, int) ([]QueueItem, error) {
	return nil, context.Canceled
}
func (errorQueue) PopMin(context.Context) (QueueItem, error) { return QueueItem{}, context.Canceled }
func (errorQueue) Remove(context.Context, string) error      { return context.Canceled }
func (errorQueue) Len(context.Context) (int64, error)        { return 0, context.Canceled }

func assertAuditEvent(t *testing.T, audit *recordingAuditRepo, policyID string, outcome string) {
	t.Helper()
	if len(audit.events) != 1 {
		t.Fatalf("audit event count=%d, want 1", len(audit.events))
	}
	event := audit.events[0]
	if event.Action != "scheduler.sla_promotion" || event.TargetID != policyID || event.Outcome != outcome || event.Actor != "system" {
		t.Fatalf("unexpected audit event: %#v", event)
	}
	metadata := map[string]any{}
	if err := json.Unmarshal(event.Metadata, &metadata); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	assertAllowedEvidenceKeys(t, metadata)
	assertNoSensitiveEvidence(t, string(event.Metadata))
}

func assertSanitizedLog(t *testing.T, line string, outcome string) {
	t.Helper()
	if !strings.Contains(line, `"outcome":"`+outcome+`"`) {
		t.Fatalf("log missing outcome %q: %s", outcome, line)
	}
	record := map[string]any{}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("log json: %v", err)
	}
	for key := range record {
		if key == "time" || key == "level" || key == "msg" {
			continue
		}
		if !allowedEvidenceKey(key) {
			t.Fatalf("unexpected log key %q in %s", key, line)
		}
	}
	assertNoSensitiveEvidence(t, line)
}

func assertAllowedEvidenceKeys(t *testing.T, metadata map[string]any) {
	t.Helper()
	for key := range metadata {
		if !allowedEvidenceKey(key) {
			t.Fatalf("unexpected metadata key %q in %#v", key, metadata)
		}
	}
	for _, key := range []string{"policy_id", "tenant_id", "tenant_class", "model_class", "request_kind", "priority", "outcome"} {
		if _, ok := metadata[key]; !ok {
			t.Fatalf("missing metadata key %q in %#v", key, metadata)
		}
	}
}

func allowedEvidenceKey(key string) bool {
	switch key {
	case "policy_id", "tenant_id", "tenant_class", "model_class", "request_kind", "priority", "outcome":
		return true
	default:
		return false
	}
}

func assertNoSensitiveEvidence(t *testing.T, content string) {
	t.Helper()
	for _, forbidden := range []string{"prompt", "message", "api_key", "authorization", "secret", "provider_payload", "embedding", "semantic_cache_payload", "raw_task_text"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("sensitive evidence %q found in %s", forbidden, content)
		}
	}
}
