package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type captureChatAdapter struct {
	last *llm.LLMRequest
}

func (a *captureChatAdapter) ID() string       { return "p1" }
func (a *captureChatAdapter) Models() []string { return []string{"gpt-4o"} }
func (a *captureChatAdapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeOpenAICompatible,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
	}
}
func (a *captureChatAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	copied := *req
	a.last = &copied
	return &llm.LLMResponse{
		Model: req.Model,
		Choices: []llm.Choice{{
			Index:        0,
			Message:      llm.Message{Role: llm.RoleAssistant, Content: "ok"},
			FinishReason: "stop",
		}},
		Usage: &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
	}, nil
}
func (a *captureChatAdapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}

type closeOnlyStreamAdapter struct {
	captureChatAdapter
}

func (a *closeOnlyStreamAdapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{DeltaContent: "ok"}
	close(ch)
	return ch, nil
}

type errorStreamAdapter struct {
	captureChatAdapter
}

func (a *errorStreamAdapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{Error: context.Canceled}
	close(ch)
	return ch, nil
}

func TestChatCompletionsPassesToolFieldsAndReturnsUsage(t *testing.T) {
	adapter := &captureChatAdapter{}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	handler := middleware.RequestID(http.HandlerFunc(NewChatHandler(svc).ChatCompletions))

	body := bytes.NewBufferString(`{
		"model":"gpt-4o",
		"messages":[
			{"role":"user","content":"call a tool"},
			{"role":"tool","tool_call_id":"call_1","content":"{\"ok\":true}"}
		],
		"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],
		"tool_choice":{"type":"function","function":{"name":"lookup"}}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set(middleware.RequestIDHeader, "req-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if adapter.last == nil || len(adapter.last.Tools) != 1 || adapter.last.ToolChoice == nil {
		t.Fatalf("tool fields were not forwarded: %#v", adapter.last)
	}
	if got := adapter.last.Messages[1].ToolCallID; got != "call_1" {
		t.Fatalf("tool_call_id not forwarded: %q", got)
	}

	var resp llm.ChatCompletionResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 5 {
		t.Fatalf("usage not returned: %#v", resp.Usage)
	}
}

func TestChatCompletionsStreamAddsDoneWhenChannelCloses(t *testing.T) {
	adapter := &closeOnlyStreamAdapter{}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	handler := middleware.RequestID(http.HandlerFunc(NewChatHandler(svc).ChatCompletions))

	body := bytes.NewBufferString(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set(middleware.RequestIDHeader, "req-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("missing done frame: %s", rec.Body.String())
	}
}

func TestChatCompletionsStreamReportsErrorThenDone(t *testing.T) {
	adapter := &errorStreamAdapter{}
	store := health.NewInMemoryStore()
	store.EnsureProvider("p1", 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	svc := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	handler := middleware.RequestID(http.HandlerFunc(NewChatHandler(svc).ChatCompletions))

	body := bytes.NewBufferString(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set(middleware.RequestIDHeader, "req-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("event: error")) || !bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("missing error or done frame: %s", rec.Body.String())
	}
}
