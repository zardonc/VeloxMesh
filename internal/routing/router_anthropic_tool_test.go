package routing_test

import (
	"context"
	"testing"

	"veloxmesh/internal/config"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/anthropic"
	"veloxmesh/internal/routing"
)

// Native capability omissions must not make implemented tool flows unroutable.
func TestNativeAnthropicToolFlowsRemainRoutable(t *testing.T) {
	adapter := anthropic.NewAdapter(anthropic.AdapterConfig{ID: "native", ModelsCSV: "test-model"})
	store := health.NewInMemoryStore()
	store.EnsureProvider(adapter.ID(), 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	for _, testCase := range []struct {
		name         string
		stream       bool
		continuation bool
	}{
		{name: "streamed call", stream: true},
		{name: "continuation", continuation: true},
		{name: "streamed continuation", stream: true, continuation: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := &llm.LLMRequest{
				Model: "test-model", Stream: testCase.stream,
				ToolRequirements: llm.ToolProtocolRequirements{
					HasDefinitions: true, HasAssistantToolCall: testCase.continuation, HasToolResult: testCase.continuation,
				},
			}
			selected, _, err := router.Select(context.Background(), request)
			if err != nil {
				t.Fatalf("native Anthropic tool flow was rejected before I/O: %v", err)
			}
			if selected.ID() != adapter.ID() {
				t.Fatalf("selected %q, want native adapter %q", selected.ID(), adapter.ID())
			}
		})
	}
}
