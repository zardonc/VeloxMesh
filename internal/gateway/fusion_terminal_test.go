package gateway_test

import (
	"context"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

func TestFusionStreamTerminalAdapterCompletesOnce(t *testing.T) {
	controller := &countingAdmissionController{}
	store := health.NewInMemoryStore()
	for _, providerID := range []string{"p1", "p2", "judge"} {
		store.EnsureProvider(providerID, 3, 1)
	}
	p1 := &mockAdapter{id: "p1", models: []string{"member-a"}, resp: textResponse("member a")}
	p2 := &mockAdapter{id: "p2", models: []string{"member-b"}, resp: textResponse("member b")}
	judge := &mockStreamAdapter{mockAdapter: mockAdapter{id: "judge", models: []string{"judge-model"}}}
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

	if controller.releases.Load() != 3 {
		t.Fatalf("release calls=%d, want 3 for two members and one judge", controller.releases.Load())
	}
	judgeSnapshot := store.Snapshot("judge")
	if judgeSnapshot.TotalSuccesses != 1 || judgeSnapshot.TotalFailures != 0 {
		t.Fatalf("judge snapshot=%#v", judgeSnapshot)
	}
}

var _ admission.Controller = (*countingAdmissionController)(nil)
