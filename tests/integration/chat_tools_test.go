package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"veloxmesh/internal/app"
	"veloxmesh/internal/llm"
)

func TestToolProtocolFailureMatrix(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(toolProtocolProvider(&calls))
	defer provider.Close()
	application := newToolIntegrationApp(t, provider)

	for _, choice := range integrationToolChoices() {
		t.Run(choice.name, func(t *testing.T) {
			before := calls.Load()
			recorder := serveToolRequest(t, application, false, choice.value)
			if recorder.Code != http.StatusOK || calls.Load() != before+1 {
				t.Fatalf("status=%d calls=%d body=%s", recorder.Code, calls.Load(), recorder.Body.String())
			}
			assertIntegrationToolResponse(t, recorder)
		})
	}
}

func TestToolStreamImmediate(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(toolProtocolProvider(&calls))
	defer provider.Close()
	application := newToolIntegrationApp(t, provider)

	recorder := serveToolRequest(t, application, true, &llm.ToolChoice{Mode: llm.ToolChoiceAuto})
	body := recorder.Body.String()
	toolIndex := bytes.Index([]byte(body), []byte(`"tool_calls"`))
	finishIndex := bytes.Index([]byte(body), []byte(`"finish_reason":"tool_calls"`))
	if recorder.Code != http.StatusOK || toolIndex < 0 || finishIndex <= toolIndex || bytes.Count([]byte(body), []byte("data: [DONE]")) != 1 {
		t.Fatalf("tool SSE status=%d body=%s", recorder.Code, body)
	}
}

func TestNoToolsRegressionIntegration(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(toolProtocolProvider(&calls))
	defer provider.Close()
	application := newToolIntegrationApp(t, provider)

	recorder, response := doChatReq(t, application, "")
	if recorder.Code != http.StatusOK || calls.Load() != 1 || response.Choices[0].FinishReason != "stop" {
		t.Fatalf("status=%d calls=%d response=%#v", recorder.Code, calls.Load(), response)
	}
}

func TestOptOutNoUpstreamIO(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(toolProtocolProvider(&calls))
	defer provider.Close()
	application := newToolIntegrationApp(t, provider)
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"functions":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
	recorder := httptest.NewRecorder()

	application.Router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest || calls.Load() != 0 {
		t.Fatalf("OPT-OUT status=%d calls=%d body=%s", recorder.Code, calls.Load(), recorder.Body.String())
	}
}

type integrationToolChoice struct {
	name  string
	value *llm.ToolChoice
}

func integrationToolChoices() []integrationToolChoice {
	return []integrationToolChoice{
		{name: "omitted"},
		{name: "auto", value: &llm.ToolChoice{Mode: llm.ToolChoiceAuto}},
		{name: "none", value: &llm.ToolChoice{Mode: llm.ToolChoiceNone}},
		{name: "required", value: &llm.ToolChoice{Mode: llm.ToolChoiceRequired}},
		{name: "named", value: &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "lookup"}},
	}
}

func newToolIntegrationApp(t *testing.T, provider *httptest.Server) *app.App {
	t.Helper()
	configPath := writeConfig(t, provider, provider, "round-robin")
	t.Cleanup(func() { _ = os.Remove(configPath) })
	os.Setenv("CONFIG_FILE", configPath)
	t.Cleanup(func() { os.Unsetenv("CONFIG_FILE") })
	application, err := app.New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return application
}

func serveToolRequest(t *testing.T, application *app.App, stream bool, choice *llm.ToolChoice) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(integrationToolRequest(stream, choice))
	if err != nil {
		t.Fatalf("marshal tool request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
	recorder := httptest.NewRecorder()
	application.Router.ServeHTTP(recorder, req)
	return recorder
}

func integrationToolRequest(stream bool, choice *llm.ToolChoice) llm.ChatCompletionRequest {
	return llm.ChatCompletionRequest{
		Model: "gpt-4o", Stream: stream, ToolChoice: choice,
		Tools: []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "lookup"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "history-call", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}}},
			{Role: llm.RoleTool, ToolCallID: "history-call", Content: `{"ok":true}`},
		},
	}
}

func toolProtocolProvider(calls *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Stream bool              `json:"stream"`
			Tools  []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(request.Tools) == 0 {
			writeTextResponse(w)
			return
		}
		if request.Stream {
			writeToolStream(w)
			return
		}
		writeToolResponse(w)
	})
}

func writeTextResponse(w http.ResponseWriter) {
	response := llm.ChatCompletionResponse{
		ID: "text-response", Object: "chat.completion", Created: time.Now().Unix(), Model: "gpt-4o",
		Choices: []llm.Choice{{Index: 0, FinishReason: "stop", Message: llm.Message{Role: llm.RoleAssistant, Content: "ok"}}},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
func writeToolResponse(w http.ResponseWriter) {
	response := llm.ChatCompletionResponse{
		ID: "tool-response", Object: "chat.completion", Created: time.Now().Unix(), Model: "gpt-4o",
		Choices: []llm.Choice{{Index: 0, FinishReason: "tool_calls", Message: llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-1", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "lookup", Arguments: `{}`}}}}}},
		Usage:   &llm.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func writeToolStream(w http.ResponseWriter) {
	index, callID, toolType, name, arguments := 0, "call-1", llm.ToolTypeFunction, "lookup", `{}`
	chunks := []llm.ChatCompletionChunkResponse{
		{ID: "tool-stream", Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: "gpt-4o", Choices: []llm.ChunkChoice{{Index: 0, Delta: llm.Delta{ToolCalls: []llm.ToolCallChunk{{Index: &index, ID: &callID, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name, Arguments: &arguments}}}}}}},
		{ID: "tool-stream", Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: "gpt-4o", Choices: []llm.ChunkChoice{{Index: 0, FinishReason: stringPointer("tool_calls")}}},
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range chunks {
		encoded, _ := json.Marshal(chunk)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

func assertIntegrationToolResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	var response llm.ChatCompletionResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choice := response.Choices[0]
	if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) != 1 || response.Usage == nil || response.Usage.TotalTokens != 5 {
		t.Fatalf("tool response=%#v", response)
	}
}

func stringPointer(value string) *string { return &value }
