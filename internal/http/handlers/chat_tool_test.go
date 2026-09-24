package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

const toolProtocolSecret = "tool-secret-should-not-appear"

type toolProtocolCase struct {
	name string
	body string
}

// Failure matrix: every violation must be invalid_request before service/provider work.
var invalidToolProtocolCases = []toolProtocolCase{
	{"unsupported tool type", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"retrieval"}]}`},
	{"missing function", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function"}]}`},
	{"blank function name", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"\u2003"}}]}`},
	{"duplicate function names", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}},{"type":"function","function":{"name":"lookup"}}]}`},
	{"malformed parameters", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":[]}}]}`},
	{"legacy functions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"functions":[]}`},
	{"legacy function call", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"function_call":"auto"}`},
	{"malformed named choice", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"function"}}`},
	{"undeclared named choice", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"function","function":{"name":"other"}}}`},
	{"choice without definitions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"tool_choice":"auto"}`},
	{"malformed content", `{"model":"gpt-4o","messages":[{"role":"user","content":{"secret":"tool-secret-should-not-appear"}}]}`},
	{"missing call ID", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"{}"}}]}]}`},
	{"duplicate call ID", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}},{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]}]}`},
	{"call invalid type", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"other","function":{"name":"lookup","arguments":"{}"}}]}]}`},
	{"call blank name", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"","arguments":"{}"}}]}]}`},
	{"call invalid arguments", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"not-json"}}]}]}`},
	{"unknown result", `{"model":"gpt-4o","messages":[{"role":"tool","tool_call_id":"call_1","content":"tool-secret-should-not-appear"}]}`},
	{"duplicate result", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"{}"},{"role":"tool","tool_call_id":"call_1","content":"{}"}]}`},
	{"unsettled before user", `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"user","content":"continue"}]}`},
}

func newToolProtocolHandler(adapter providers.ProviderAdapter) http.Handler {
	store := health.NewInMemoryStore()
	store.EnsureProvider(adapter.ID(), 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	service := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	return middleware.RequestID(http.HandlerFunc(NewChatHandler(service).ChatCompletions))
}

func TestChatToolValidationRejectsInvalidRequestsBeforeService(t *testing.T) {
	for _, tc := range invalidToolProtocolCases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &captureChatAdapter{}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(tc.body))
			newToolProtocolHandler(adapter).ServeHTTP(recorder, request)
			assertToolProtocolError(t, recorder, adapter)
		})
	}
}

func assertToolProtocolError(t *testing.T, recorder *httptest.ResponseRecorder, adapter *captureChatAdapter) {
	t.Helper()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %q", response.Code)
	}
	if adapter.last != nil {
		t.Fatal("invalid request reached service boundary")
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(toolProtocolSecret)) {
		t.Fatal("error response leaked tool payload")
	}
}

func TestChatToolValidationPreservesCompatibleRequests(t *testing.T) {
	adapter := &captureChatAdapter{}
	handler := newToolProtocolHandler(adapter)
	for _, body := range []string{validParallelToolRequest, validTextOnlyRequest, validMixedContentToolRequest} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || adapter.last == nil {
			t.Fatalf("expected compatible request to reach service: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}
}

const validParallelToolRequest = `{"model":"gpt-4o","messages":[{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{}"}},{"id":"call_2","type":"function","function":{"name":"lookup","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_2","content":"{}"},{"role":"tool","tool_call_id":"call_1","content":"{}"},{"role":"user","content":"continue"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":"auto"}`

const validTextOnlyRequest = `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`

const validMixedContentToolRequest = `{"model":"gpt-4o","messages":[{"role":"assistant","content":"calling now","tool_calls":[{"id":"call_1","type":"function","function":{"name":"caf\u00e9","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_1","content":"{}"},{"role":"assistant","content":"done"}],"tools":[{"type":"function","function":{"name":"caf\u00e9"}},{"type":"function","function":{"name":"cafe\u0301"}}]}`
