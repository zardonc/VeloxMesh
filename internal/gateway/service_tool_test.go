package gateway_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type semanticCacheSpy struct {
	lookups atomic.Int32
	stores  atomic.Int32
}

func (s *semanticCacheSpy) Store(context.Context, *controlstate.SemanticCacheEntry) error {
	s.stores.Add(1)
	return nil
}

func (s *semanticCacheSpy) ListCandidates(context.Context, string, string) ([]*controlstate.SemanticCacheEntry, error) {
	s.lookups.Add(1)
	return nil, nil
}

func (s *semanticCacheSpy) GetCandidate(context.Context, string, string, string) (*controlstate.SemanticCacheEntry, error) {
	return nil, nil
}

func (s *semanticCacheSpy) RecordHit(context.Context, string) error { return nil }
func (s *semanticCacheSpy) Disable(context.Context, string) error   { return nil }

type embeddingSpy struct{ calls atomic.Int32 }

func (s *embeddingSpy) ID() string       { return "embeddings" }
func (s *embeddingSpy) Models() []string { return []string{"text-embedding-3-small"} }
func (s *embeddingSpy) Complete(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}
func (s *embeddingSpy) HealthCheck(context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}
func (s *embeddingSpy) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{SupportedOperations: []providers.Operation{providers.OperationEmbeddings}}
}

func (s *embeddingSpy) Embed(context.Context, *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	s.calls.Add(1)
	return &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1}}}}, nil
}

type toolProtocolAdapter struct{ mockAdapter }

func (a *toolProtocolAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeOpenAICompatible,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
		Streaming:           true,
		ToolCalling:         true,
		ToolProtocol:        fullToolProtocolCapability(),
	}
}

func fullToolProtocolCapability() providers.ToolProtocolCapability {
	return providers.ToolProtocolCapability{
		Definitions: true, AssistantToolCalls: true, ToolResults: true, StreamingDeltas: true,
		ChoiceModes: map[providers.ToolChoiceCapabilityMode]bool{
			providers.ToolChoiceCapabilityOmitted:  true,
			providers.ToolChoiceCapabilityAuto:     true,
			providers.ToolChoiceCapabilityNone:     true,
			providers.ToolChoiceCapabilityRequired: true,
			providers.ToolChoiceCapabilityNamed:    true,
		},
	}
}

func TestToolProtocolTracerBypassesSemanticCache(t *testing.T) {
	cacheSpy, embedder := &semanticCacheSpy{}, &embeddingSpy{}
	service := newSemanticCacheTracerService(cacheSpy, embedder)
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, &middleware.AuthIdentity{ID: "cache-user"})

	_, err := service.HandleChatCompletion(ctx, toolProtocolTracerRequest())
	if err != nil {
		t.Fatalf("HandleChatCompletion: %v", err)
	}
	if got := embedder.calls.Load(); got != 0 {
		t.Fatalf("embedding calls=%d, want 0", got)
	}
	if got := cacheSpy.lookups.Load(); got != 0 {
		t.Fatalf("cache lookups=%d, want 0", got)
	}
	if got := cacheSpy.stores.Load(); got != 0 {
		t.Fatalf("cache stores=%d, want 0", got)
	}
}

func TestNoToolsWithoutTrustedProfileBypassesSemanticCache(t *testing.T) {
	cacheSpy, embedder := &semanticCacheSpy{}, &embeddingSpy{}
	service := newSemanticCacheTracerService(cacheSpy, embedder)
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, &middleware.AuthIdentity{ID: "cache-user"})
	request := toolProtocolTracerRequest()
	request.ToolRequirements = llm.ToolProtocolRequirements{}

	_, err := service.HandleChatCompletion(ctx, request)
	if err != nil {
		t.Fatalf("HandleChatCompletion: %v", err)
	}
	if got := embedder.calls.Load(); got != 0 {
		t.Fatalf("embedding calls=%d, want 0", got)
	}
	if got := cacheSpy.lookups.Load(); got != 0 {
		t.Fatalf("cache lookups=%d, want 0", got)
	}
	if got := cacheSpy.stores.Load(); got != 0 {
		t.Fatalf("cache stores=%d, want 0", got)
	}
}

func newSemanticCacheTracerService(repo controlstate.SemanticCacheRepository, embedder providers.EmbedAdapter) *gateway.Service {
	adapter := &toolProtocolAdapter{mockAdapter: mockAdapter{id: "tool-provider", models: []string{"gpt-4o"}, resp: textResponse("ok")}}
	store := health.NewInMemoryStore()
	store.EnsureProvider(adapter.ID(), 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	semanticCache := cache.NewSemanticCacheService(cache.SemanticCacheConfig{Enabled: true, MaxCandidates: 1, TTL: time.Minute}, repo, nil, embedder)
	return gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, semanticCache, pipeline.DefaultRegistry(), nil, nil)
}

func toolProtocolTracerRequest() *llm.LLMRequest {
	return &llm.LLMRequest{
		RequestID: "tool-cache-tracer", Model: "gpt-4o",
		Messages:         []llm.Message{{Role: llm.RoleUser, Content: "call lookup"}},
		ToolRequirements: llm.ToolProtocolRequirements{HasDefinitions: true},
	}
}
