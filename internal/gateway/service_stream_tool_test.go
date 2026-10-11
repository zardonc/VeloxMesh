package gateway_test

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

const toolStreamImmediateTimeout = 200 * time.Millisecond

type pendingToolStreamAdapter struct {
	toolProtocolAdapter
	events <-chan llm.StreamEvent
}

func (a *pendingToolStreamAdapter) Stream(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	return a.events, nil
}

func TestToolProtocolTracerBypassesResponseRuleBuffer(t *testing.T) {
	events := make(chan llm.StreamEvent, 1)
	index, callID, toolType, name, arguments := 0, "call-1", llm.ToolTypeFunction, "lookup", `{}`
	events <- llm.StreamEvent{ToolCalls: []llm.ToolCallChunk{{
		Index: &index, ID: &callID, Type: &toolType,
		Function: &llm.FunctionCallChunk{Name: &name, Arguments: &arguments},
	}}}
	adapter := &pendingToolStreamAdapter{toolProtocolAdapter: toolProtocolAdapter{mockAdapter: mockAdapter{id: "tool-provider", models: []string{"gpt-4o"}}}, events: events}
	service := newToolProtocolStreamService(adapter)

	result := make(chan error, 1)
	var stream <-chan llm.StreamEvent
	go func() {
		var err error
		stream, _, err = service.HandleChatCompletionStream(context.Background(), &llm.LLMRequest{
			RequestID: "tool-stream-tracer", Model: "gpt-4o", Stream: true,
			ToolRequirements: llm.ToolProtocolRequirements{HasDefinitions: true},
		})
		result <- err
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("HandleChatCompletionStream: %v", err)
		}
	case <-time.After(toolStreamImmediateTimeout):
		t.Fatal("tool stream was buffered while response rules were enabled")
	}
	first := <-stream
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID == nil || *first.ToolCalls[0].ID != callID {
		t.Fatalf("first stream event=%#v, want initial tool-call delta", first)
	}
	close(events)
	for range stream {
	}
}

func newToolProtocolStreamService(adapter providers.ProviderAdapter) *gateway.Service {
	store := health.NewInMemoryStore()
	store.EnsureProvider(adapter.ID(), 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RulePII] = pipeline.RuleConfig{Enabled: true}
	return gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), staticRuleResolver{cfg: cfg}, nil)
}
