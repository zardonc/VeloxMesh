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
	last         *llm.LLMRequest
	calls        int
	providerType providers.ProviderType
	response     *llm.LLMResponse
}

func (a *captureChatAdapter) ID() string       { return "p1" }
func (a *captureChatAdapter) Models() []string { return []string{"gpt-4o"} }
func (a *captureChatAdapter) Capabilities() providers.CapabilitySet {
	providerType := a.providerType
	if providerType == "" {
		providerType = providers.ProviderTypeOpenAICompatible
	}
	return providers.CapabilitySet{
		ProviderType:        providerType,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
		Streaming:           true,
		ToolCalling:         true,
		ToolProtocol: providers.ToolProtocolCapability{
			Definitions: true, AssistantToolCalls: true, ToolResults: true, StreamingDeltas: true,
			ChoiceModes: map[providers.ToolChoiceCapabilityMode]bool{
				providers.ToolChoiceCapabilityOmitted:  true,
				providers.ToolChoiceCapabilityAuto:     true,
				providers.ToolChoiceCapabilityNone:     true,
				providers.ToolChoiceCapabilityRequired: true,
				providers.ToolChoiceCapabilityNamed:    true,
			},
		},
	}
}
func (a *captureChatAdapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	copied := *req
	a.last = &copied
	a.calls++
	if a.response != nil {
		return a.response, nil
	}
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
			{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},
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
	if got := adapter.last.Messages[2].ToolCallID; got != "call_1" {
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

func TestChatCompletionsStreamDoesNotAddDoneWhenChannelCloses(t *testing.T) {
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
	if bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("unexpected done frame after bare channel close: %s", rec.Body.String())
	}
}

func TestChatCompletionsStreamSuppressesCancelledTerminalOutput(t *testing.T) {
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
	if bytes.Contains(rec.Body.Bytes(), []byte("event: error")) || bytes.Contains(rec.Body.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("unexpected terminal output for cancellation: %s", rec.Body.String())
	}
}

func TestPublicToolMatrix(t *testing.T) {
	for _, providerType := range []providers.ProviderType{
		providers.ProviderTypeOpenAICompatible,
		providers.ProviderTypeAnthropic,
		providers.ProviderTypeGemini,
	} {
		for _, choice := range publicToolChoices() {
			t.Run(string(providerType)+"/"+choice.name, func(t *testing.T) {
				adapter := &captureChatAdapter{providerType: providerType, response: publicToolResponse()}
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(publicToolRequestBody(t, choice.value)))
				newToolProtocolHandler(adapter).ServeHTTP(recorder, request)

				if recorder.Code != http.StatusOK || adapter.calls != 1 || adapter.last == nil {
					t.Fatalf("status=%d calls=%d body=%s", recorder.Code, adapter.calls, recorder.Body.String())
				}
				if !adapter.last.ToolRequirements.HasDefinitions || !adapter.last.ToolRequirements.HasAssistantToolCall || !adapter.last.ToolRequirements.HasToolResult {
					t.Fatalf("tool requirements were not preserved: %#v", adapter.last.ToolRequirements)
				}
				assertPublicToolResponse(t, recorder)
			})
		}
	}
}

func TestNoToolsRegression(t *testing.T) {
	adapter := &captureChatAdapter{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	newToolProtocolHandler(adapter).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || adapter.calls != 1 || adapter.last.ToolRequirements.UsesProtocol() {
		t.Fatalf("status=%d calls=%d requirements=%#v", recorder.Code, adapter.calls, adapter.last.ToolRequirements)
	}
	var response llm.ChatCompletionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil || response.Usage == nil || response.Choices[0].FinishReason != "stop" {
		t.Fatalf("no-tools response=%#v err=%v", response, err)
	}
}

func TestOptOutNoUpstreamIO(t *testing.T) {
	for _, body := range []string{
		`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"functions":[]}`,
		`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"function_call":"auto"}`,
	} {
		adapter := &captureChatAdapter{}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
		newToolProtocolHandler(adapter).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || adapter.calls != 0 || adapter.last != nil {
			t.Fatalf("OPT-OUT status=%d calls=%d body=%s", recorder.Code, adapter.calls, recorder.Body.String())
		}
	}
}

type publicToolChoice struct {
	name  string
	value *llm.ToolChoice
}

func publicToolChoices() []publicToolChoice {
	return []publicToolChoice{
		{name: "omitted"},
		{name: "auto", value: &llm.ToolChoice{Mode: llm.ToolChoiceAuto}},
		{name: "none", value: &llm.ToolChoice{Mode: llm.ToolChoiceNone}},
		{name: "required", value: &llm.ToolChoice{Mode: llm.ToolChoiceRequired}},
		{name: "named", value: &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "lookup"}},
	}
}

func publicToolRequestBody(t *testing.T, choice *llm.ToolChoice) []byte {
	t.Helper()
	request := llm.ChatCompletionRequest{
		Model: "gpt-4o", ToolChoice: choice,
		Tools: []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "lookup"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "history-call", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
			{Role: llm.RoleTool, ToolCallID: "history-call", Content: `{"ok":true}`},
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal public tool request: %v", err)
	}
	return body
}

func publicToolResponse() *llm.LLMResponse {
	return &llm.LLMResponse{
		Model: "gpt-4o", Usage: &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		Choices: []llm.Choice{{Index: 0, FinishReason: "tool_calls", Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}}}}},
	}
}

func assertPublicToolResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var response llm.ChatCompletionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choice := response.Choices[0]
	if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) != 1 || response.Usage == nil || response.Usage.TotalTokens != 5 {
		t.Fatalf("public tool response=%#v", response)
	}
}
