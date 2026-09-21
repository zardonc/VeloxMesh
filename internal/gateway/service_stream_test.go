package gateway_test

import (
	"context"
	stdlib_errors "errors"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/scheduler"
)

type countingAdmissionController struct {
	releases atomic.Int32
}

func (c *countingAdmissionController) Admit(context.Context, *llm.LLMRequest, routing.RoutingDecision) (admission.ReleaseFunc, admission.AdmissionDecision, error) {
	return func() { c.releases.Add(1) }, admission.AdmissionDecision{}, nil
}

type controlledStreamAdapter struct {
	mockAdapter
	events <-chan llm.StreamEvent
}

const (
	benchmarkStreamChunkCount = 64
	benchmarkStreamPayload    = "stream benchmark payload"
)

func (a *controlledStreamAdapter) Stream(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	return a.events, nil
}

func TestService_HandleChatCompletionStream_WithSchedulerRunner(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)

	p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}}
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{p1}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	svc.SetSchedulerRunner(newTestSchedulerRunner())

	ch, meta, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{Model: "gpt-4o", RequestID: "req-1", Stream: true})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	if meta.Provider != "p1" || meta.Model != "gpt-4o" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	seenDone := false
	for event := range ch {
		if event.Done {
			seenDone = true
		}
	}
	if !seenDone {
		t.Fatalf("expected stream done event")
	}
}

func TestService_HandleChatCompletionStream_UsesLastUsageChunk(t *testing.T) {
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	repo := &mockRepoWithUsage{}
	p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: []llm.StreamEvent{
		{DeltaContent: "a", Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}},
		{DeltaContent: "b", Usage: &llm.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}},
		{Done: true},
	}}
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{p1}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, repo, nil, pipeline.DefaultRegistry(), nil, nil)

	ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{Model: "gpt-4o", RequestID: "req-1", Stream: true})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	for range ch {
	}
	if repo.lastRecord == nil || repo.lastRecord.TotalTokens != 5 {
		t.Fatalf("expected final usage chunk to settle, got %#v", repo.lastRecord)
	}
}

func TestService_HandleChatCompletionStreamTerminalFirstCandidateWins(t *testing.T) {
	tests := []struct {
		name              string
		events            []llm.StreamEvent
		expectedSuccesses int
		expectedFailures  int
	}{
		{
			name: "done then late provider error",
			events: []llm.StreamEvent{
				{Done: true},
				{Error: errors.NewGatewayError(errors.ProviderUnavailable, "late", 503)},
			},
			expectedSuccesses: 1,
		},
		{
			name: "provider error then channel close",
			events: []llm.StreamEvent{
				{Error: errors.NewGatewayError(errors.ProviderUnavailable, "failed", 503)},
			},
			expectedFailures: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &countingAdmissionController{}
			store := health.NewInMemoryStore()
			store.EnsureProvider("p1", 3, 1)
			adapter := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: tt.events}
			svc := newStreamTerminalTestService(t, adapter, store, controller)

			ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{Model: "gpt-4o", RequestID: "req-1", Stream: true})
			if err != nil {
				t.Fatalf("HandleChatCompletionStream: %v", err)
			}
			for range ch {
			}

			snapshot := store.Snapshot("p1")
			if controller.releases.Load() != 1 {
				t.Fatalf("release calls=%d, want 1", controller.releases.Load())
			}
			if snapshot.TotalSuccesses != tt.expectedSuccesses || snapshot.TotalFailures != tt.expectedFailures {
				t.Fatalf("snapshot=%#v, want successes=%d failures=%d", snapshot, tt.expectedSuccesses, tt.expectedFailures)
			}
		})
	}
}

func TestService_HandleChatCompletionStreamTerminalCancellationReleasesOnce(t *testing.T) {
	controller := &countingAdmissionController{}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	events := make(chan llm.StreamEvent)
	adapter := &controlledStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: events}
	svc := newStreamTerminalTestService(t, adapter, store, controller)

	ctx, cancel := context.WithCancel(context.Background())
	ch, _, err := svc.HandleChatCompletionStream(ctx, &llm.LLMRequest{Model: "gpt-4o", RequestID: "req-1", Stream: true})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	cancel()
	close(events)
	for range ch {
	}

	snapshot := store.Snapshot("p1")
	if controller.releases.Load() != 1 || snapshot.TotalFailures != 0 || snapshot.TotalSuccesses != 1 {
		t.Fatalf("release=%d snapshot=%#v", controller.releases.Load(), snapshot)
	}
}

func BenchmarkServiceHandleChatCompletionStream(b *testing.B) {
	events := make([]llm.StreamEvent, benchmarkStreamChunkCount+1)
	for i := range benchmarkStreamChunkCount {
		events[i] = llm.StreamEvent{DeltaContent: benchmarkStreamPayload}
	}
	events[benchmarkStreamChunkCount] = llm.StreamEvent{Done: true}

	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	adapter := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: events}
	svc := newStreamTerminalTestService(b, adapter, store, admission.NewPassThroughController())
	req := &llm.LLMRequest{Model: "gpt-4o", RequestID: "benchmark-stream", Stream: true}

	b.ReportAllocs()
	b.SetBytes(int64(benchmarkStreamChunkCount * len(benchmarkStreamPayload)))
	b.ResetTimer()
	for range b.N {
		ch, _, err := svc.HandleChatCompletionStream(context.Background(), req)
		if err != nil {
			b.Fatal(err)
		}
		for range ch {
		}
	}
}

func TestService_HandleChatCompletionStream_BuffersResponseRules(t *testing.T) {
	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RulePII] = pipeline.RuleConfig{Enabled: true}
	p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: []llm.StreamEvent{
		{DeltaContent: "Stored {{PII_EMAIL_0}}", Usage: &llm.Usage{CompletionTokens: 2}},
		{Done: true},
	}}
	svc := newStreamRuleTestService(t, p1, cfg)

	ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{
		Model: "gpt-4o", RequestID: "req-1", Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "email me at test@example.com"}},
	})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	if got := collectStreamText(ch); got != "Stored test@example.com" {
		t.Fatalf("expected restored stream text, got %q", got)
	}
}

func TestService_HandleChatCompletionStream_SkipsResponseRulesForToolCalls(t *testing.T) {
	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RulePII] = pipeline.RuleConfig{Enabled: true}
	p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: []llm.StreamEvent{
		{DeltaContent: "{{PII_EMAIL_0}}", ToolCalls: []llm.ToolCallChunk{{}}},
		{Done: true},
	}}
	svc := newStreamRuleTestService(t, p1, cfg)

	ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{
		Model: "gpt-4o", RequestID: "req-1", Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "email me at test@example.com"}},
	})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	text := ""
	toolCalls := 0
	for event := range ch {
		text += event.DeltaContent
		toolCalls += len(event.ToolCalls)
	}
	if text != "{{PII_EMAIL_0}}" || toolCalls != 1 {
		t.Fatalf("expected original tool-call stream, text=%q toolCalls=%d", text, toolCalls)
	}
}

func TestService_HandleChatCompletionStream_ResponseRuleBlockDoesNotAffectProviderHealth(t *testing.T) {
	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RuleFilter] = pipeline.RuleConfig{Enabled: true, Options: map[string]interface{}{"response_action": "block"}}
	p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{p1}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), staticRuleResolver{cfg: cfg}, nil)

	_, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{Model: "gpt-4o", RequestID: "req-1", Stream: true})
	var gwErr *errors.GatewayError
	if !stdlib_errors.As(err, &gwErr) || gwErr.Code != "policy_blocked" {
		t.Fatalf("expected policy_blocked, got %v", err)
	}
	if failures := store.Snapshot("p1").ConsecutiveFailures; failures != 0 {
		t.Fatalf("policy block affected provider health: failures=%d", failures)
	}
}

func TestService_HandleChatCompletionStream_FusionBuffersResponseRules(t *testing.T) {
	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RulePII] = pipeline.RuleConfig{Enabled: true}
	store := health.NewInMemoryStore()
	for _, providerID := range []string{"p1", "p2", "judge"} {
		store.EnsureProvider(providerID, 3, 1)
	}

	p1 := &mockAdapter{id: "p1", models: []string{"member-a"}, resp: textResponse("member a")}
	p2 := &mockAdapter{id: "p2", models: []string{"member-b"}, resp: textResponse("member b")}
	judge := &mockStreamAdapter{
		mockAdapter: mockAdapter{id: "judge", models: []string{"judge-model"}},
		events: []llm.StreamEvent{
			{DeltaContent: "Stream {{PII_EMAIL_0}}", Usage: &llm.Usage{CompletionTokens: 2}},
			{Done: true},
		},
	}
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{p1, p2, judge}, []providers.Combo{
		{ID: "fusion-combo", Name: "fusion-combo", Strategy: "fusion", Members: []string{"member-a", "member-b"}, Judge: "judge-model"},
	})
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), staticRuleResolver{cfg: cfg}, nil)

	ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{
		Model: "fusion-combo", RequestID: "req-1", Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "email me at test@example.com"}},
	})
	if err != nil {
		t.Fatalf("HandleChatCompletionStream: %v", err)
	}
	if got := collectStreamText(ch); got != "Stream test@example.com" {
		t.Fatalf("expected restored fusion stream, got %q", got)
	}
}

func TestFusionStreamTerminalFirstCandidateWins(t *testing.T) {
	tests := []struct {
		name        string
		judgeEvents []llm.StreamEvent
		successes   int
		failures    int
	}{
		{
			name: "done then late judge error",
			judgeEvents: []llm.StreamEvent{
				{Done: true},
				{Error: errors.NewGatewayError(errors.ProviderUnavailable, "late", 503)},
			},
			successes: 1,
		},
		{
			name: "judge error then channel close",
			judgeEvents: []llm.StreamEvent{
				{Error: errors.NewGatewayError(errors.ProviderUnavailable, "failed", 503)},
			},
			failures: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &countingAdmissionController{}
			store := health.NewInMemoryStore()
			for _, providerID := range []string{"p1", "p2", "judge"} {
				store.EnsureProvider(providerID, 3, 1)
			}
			p1 := &mockAdapter{id: "p1", models: []string{"member-a"}, resp: textResponse("member a")}
			p2 := &mockAdapter{id: "p2", models: []string{"member-b"}, resp: textResponse("member b")}
			judge := &mockStreamAdapter{mockAdapter: mockAdapter{id: "judge", models: []string{"judge-model"}}, events: tt.judgeEvents}
			registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{p1, p2, judge}, []providers.Combo{
				{ID: "fusion-combo", Name: "fusion-combo", Strategy: "fusion", Members: []string{"member-a", "member-b"}, Judge: "judge-model"},
			})
			router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
			svc := gateway.NewService(router, controller, store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)

			ch, _, err := svc.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{Model: "fusion-combo", RequestID: "req-1", Stream: true})
			if err != nil {
				t.Fatalf("HandleChatCompletionStream: %v", err)
			}
			for range ch {
			}

			snapshot := store.Snapshot("judge")
			if controller.releases.Load() != 3 {
				t.Fatalf("release calls=%d, want 3 for two members and one judge", controller.releases.Load())
			}
			if snapshot.TotalSuccesses != tt.successes || snapshot.TotalFailures != tt.failures {
				t.Fatalf("judge snapshot=%#v, want successes=%d failures=%d", snapshot, tt.successes, tt.failures)
			}
		})
	}
}

type mockStreamAdapter struct {
	mockAdapter
	events []llm.StreamEvent
}

func (m *mockStreamAdapter) Stream(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	events := m.events
	if events == nil {
		events = []llm.StreamEvent{{DeltaContent: "ok"}, {Done: true}}
	}
	ch := make(chan llm.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type staticRuleResolver struct {
	cfg *pipeline.SemanticPipelineConfig
}

func (r staticRuleResolver) GetGlobalDefaults(context.Context) (*pipeline.SemanticPipelineConfig, error) {
	return r.cfg, nil
}

func (r staticRuleResolver) GetUserConfig(context.Context, string) (*pipeline.SemanticPipelineConfig, error) {
	return nil, nil
}

func newStreamRuleTestService(t *testing.T, adapter *mockStreamAdapter, cfg *pipeline.SemanticPipelineConfig) *gateway.Service {
	t.Helper()
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	return gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), staticRuleResolver{cfg: cfg}, nil)
}

func newStreamTerminalTestService(t testing.TB, adapter providers.ProviderAdapter, store health.Store, controller admission.Controller) *gateway.Service {
	t.Helper()
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	return gateway.NewService(router, controller, store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
}

func collectStreamText(ch <-chan llm.StreamEvent) string {
	text := ""
	for event := range ch {
		text += event.DeltaContent
	}
	return text
}

func textResponse(content string) *llm.LLMResponse {
	return &llm.LLMResponse{
		Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant, Content: content}}},
		Usage:   &llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}
}

func newTestSchedulerRunner() *scheduler.SynchronousRunner {
	registry := scheduler.NewResultRegistry()
	queue := scheduler.NewMemoryQueue()
	intake := &scheduler.TaskIntake{
		Queue: queue, Scorer: scheduler.FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: scheduler.NewPriorityResolver(nil),
		Policy:   scheduler.PriorityPolicy{Default: scheduler.PriorityNormal, Max: scheduler.PriorityHigh},
		Backend:  "memory",
	}
	executor := &scheduler.Executor{Queue: queue, Registry: registry}
	return scheduler.NewSynchronousRunner(intake, executor, registry)
}

func newQueuedSchedulerRunner(t *testing.T, guard scheduler.QueueGuard, queued int) *scheduler.SynchronousRunner {
	t.Helper()
	registry := scheduler.NewResultRegistry()
	queue := scheduler.NewMemoryQueue()
	for i := 0; i < queued; i++ {
		if err := queue.Push(context.Background(), scheduler.QueueItem{TaskID: "queued-" + string(rune('a'+i)), Score: 1}); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	intake := &scheduler.TaskIntake{
		Queue: queue, Guard: guard, Scorer: scheduler.FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: scheduler.NewPriorityResolver(nil),
		Policy:   scheduler.PriorityPolicy{Default: scheduler.PriorityNormal, Max: scheduler.PriorityHigh},
		Backend:  "memory", ThrottleWait: time.Millisecond,
	}
	executor := &scheduler.Executor{Queue: queue, Registry: registry}
	return scheduler.NewSynchronousRunner(intake, executor, registry)
}

func newLostTaskSchedulerRunner() *scheduler.SynchronousRunner {
	registry := scheduler.NewResultRegistry()
	intakeQueue := scheduler.NewMemoryQueue()
	executorQueue := scheduler.NewMemoryQueue()
	intake := &scheduler.TaskIntake{
		Queue: intakeQueue, Scorer: scheduler.FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: scheduler.NewPriorityResolver(nil),
		Policy:   scheduler.PriorityPolicy{Default: scheduler.PriorityNormal, Max: scheduler.PriorityHigh},
		Backend:  "memory",
	}
	executor := &scheduler.Executor{Queue: executorQueue, Registry: registry}
	return scheduler.NewSynchronousRunner(intake, executor, registry)
}
