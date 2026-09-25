package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/adaptertest"
)

const geminiToolSchema = `{"type":"object","additionalProperties":false,"properties":{"city":{"type":"string"}},"required":["city"]}`

func TestToolRequestPreservesDeclarationsChoicesHistoryAndResults(t *testing.T) {
	choices := []struct {
		name       string
		choice     *llm.ToolChoice
		wantMode   string
		wantNames  []string
		wantAbsent bool
	}{
		{name: "default", wantAbsent: true},
		{name: "auto", choice: &llm.ToolChoice{Mode: llm.ToolChoiceAuto}, wantMode: "AUTO"},
		{name: "none", choice: &llm.ToolChoice{Mode: llm.ToolChoiceNone}, wantMode: "NONE"},
		{name: "required", choice: &llm.ToolChoice{Mode: llm.ToolChoiceRequired}, wantMode: "ANY"},
		{name: "named", choice: &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "weather"}, wantMode: "ANY", wantNames: []string{"weather"}},
	}

	for _, tc := range choices {
		t.Run(tc.name, func(t *testing.T) {
			var request map[string]any
			server := geminiToolServer(t, func(body map[string]any, writer http.ResponseWriter) {
				request = body
				writeGeminiJSON(t, writer, geminiCompletion("call-provider", "weather", map[string]any{"city": "Paris"}, "STOP", geminiUsage()))
			})
			defer server.Close()

			response, err := newGeminiToolAdapter(server).Complete(context.Background(), geminiToolRequest(tc.choice))
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 1 {
				t.Fatalf("tool response choices = %#v", response.Choices)
			}
			if got := response.Choices[0].Message.ToolCalls[0].ID; got != "call-provider" {
				t.Errorf("tool call ID = %q, want provider ID", got)
			}
			if got := response.Choices[0].FinishReason; got != "tool_calls" {
				t.Errorf("finish reason = %q, want tool_calls", got)
			}
			assertGeminiRequest(t, request, tc.wantMode, tc.wantNames, tc.wantAbsent)
		})
	}
}

func TestToolCompleteNormalizesUsageAndSafeFailures(t *testing.T) {
	cases := []struct {
		name      string
		response  map[string]any
		wantError bool
	}{
		{
			name: "provider ids and combined usage",
			response: geminiCompletion("call-a", "weather", map[string]any{"city": "Paris"}, "STOP", map[string]any{
				"promptTokenCount":        3,
				"toolUsePromptTokenCount": 2,
				"candidatesTokenCount":    5,
				"thoughtsTokenCount":      7,
				"totalTokenCount":         17,
			}),
		},
		{
			name:      "malformed function finish is safe provider error",
			response:  geminiCompletion("call-secret", "weather", map[string]any{"token": "secret-value"}, "MALFORMED_FUNCTION_CALL", geminiUsage()),
			wantError: true,
		},
		{
			name:     "missing function id receives opaque identity",
			response: geminiCompletion("", "weather", map[string]any{"city": "Paris"}, "STOP", geminiUsage()),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := geminiToolServer(t, func(_ map[string]any, writer http.ResponseWriter) {
				writeGeminiJSON(t, writer, tc.response)
			})
			defer server.Close()

			response, err := newGeminiToolAdapter(server).Complete(context.Background(), geminiToolRequest(nil))
			if tc.wantError {
				if err == nil {
					t.Fatal("Complete() error = nil, want safe provider error")
				}
				if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "call-secret") {
					t.Errorf("error leaked provider data: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if len(response.Choices) != 1 || len(response.Choices[0].Message.ToolCalls) != 1 {
				t.Fatalf("tool response choices = %#v", response.Choices)
			}
			call := response.Choices[0].Message.ToolCalls[0]
			if tc.name == "missing function id receives opaque identity" {
				if call.ID == "" || call.ID == "call_1" {
					t.Errorf("generated ID = %q, want non-ordinal opaque ID", call.ID)
				}
				return
			}
			if call.ID != "call-a" {
				t.Errorf("tool call ID = %q, want call-a", call.ID)
			}
			if response.Choices[0].FinishReason != "tool_calls" {
				t.Errorf("finish reason = %q, want tool_calls", response.Choices[0].FinishReason)
			}
			if response.Usage.PromptTokens != 5 || response.Usage.CompletionTokens != 12 || response.Usage.TotalTokens != 17 {
				t.Errorf("usage = %#v, want prompt=5 completion=12 total=17", response.Usage)
			}
		})
	}
}

func TestToolStreamEmitsCompleteCallsAndRequiresTerminalEvent(t *testing.T) {
	cases := []struct {
		name      string
		events    []map[string]any
		wantError bool
	}{
		{
			name: "complete call before provider closes with final usage",
			events: []map[string]any{
				geminiCompletion("call-stream", "weather", map[string]any{"city": "Paris"}, "", nil),
				geminiCompletion("", "", nil, "STOP", map[string]any{
					"promptTokenCount":        2,
					"toolUsePromptTokenCount": 3,
					"candidatesTokenCount":    5,
					"thoughtsTokenCount":      7,
					"totalTokenCount":         17,
				}),
			},
		},
		{
			name: "premature end after tool call is a safe error",
			events: []map[string]any{
				geminiCompletion("call-stream", "weather", map[string]any{"city": "Paris"}, "", nil),
			},
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := geminiToolStreamServer(t, tc.events)
			defer server.Close()

			stream, err := newGeminiToolAdapter(server).Stream(context.Background(), geminiToolRequest(nil))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			events := collectGeminiEvents(stream)
			if tc.wantError {
				if len(events) == 0 || events[len(events)-1].Error == nil {
					t.Fatalf("stream events = %#v, want safe terminal error", events)
				}
				if strings.Contains(events[len(events)-1].Error.Error(), "Paris") {
					t.Errorf("stream error leaked provider arguments: %v", events[len(events)-1].Error)
				}
				return
			}

			calls := streamToolCalls(events)
			if len(calls) != 1 {
				t.Fatalf("stream tool calls = %#v, want one complete call", calls)
			}
			if calls[0].ID == nil || *calls[0].ID != "call-stream" {
				t.Errorf("stream ID = %#v, want call-stream", calls[0].ID)
			}
			if calls[0].Function == nil || calls[0].Function.Arguments == nil || *calls[0].Function.Arguments != `{"city":"Paris"}` {
				t.Errorf("stream arguments = %#v, want complete JSON arguments", calls[0].Function)
			}
			finish := streamFinish(events)
			if finish == nil || finish.FinishReason != "tool_calls" {
				t.Errorf("stream finish = %#v, want tool_calls", finish)
				return
			}
			if finish.Usage == nil || finish.Usage.PromptTokens != 5 || finish.Usage.CompletionTokens != 12 || finish.Usage.TotalTokens != 17 {
				t.Errorf("stream usage = %#v, want prompt=5 completion=12 total=17", finish.Usage)
			}
		})
	}
}

func TestToolContractHarness(t *testing.T) {
	server := geminiToolServer(t, func(_ map[string]any, writer http.ResponseWriter) {
		writeGeminiJSON(t, writer, geminiCompletion("call-contract", "weather", map[string]any{"city": "Paris"}, "STOP", geminiUsage()))
	})
	defer server.Close()

	adapter := newGeminiToolAdapter(server)
	completed := adaptertest.ToolContractCompletion{}
	err := adaptertest.RunToolContract(
		context.Background(),
		adaptertest.ToolContractFixture{Complete: adapter.Complete, Stream: collectGeminiStream(adapter)},
		adaptertest.ToolContractCase{Name: "gemini", Request: geminiToolRequest(nil), ExpectedFinishReason: "tool_calls"},
		func(result adaptertest.ToolContractCompletion) { completed = result },
	)
	if err != nil {
		t.Fatalf("RunToolContract() error = %v", err)
	}
	if completed.FinishReason != "tool_calls" {
		t.Errorf("contract finish = %q, want tool_calls", completed.FinishReason)
	}
}

func TestGeminiToolRequestRejectsUncorrelatedResult(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	request := geminiToolRequest(nil)
	request.Messages = append(request.Messages, llm.Message{Role: llm.RoleTool, ToolCallID: "unknown-call", Content: `{"secret":"value"}`})
	_, _, _, err := newGeminiToolAdapter(server).buildParams(request)
	if err == nil {
		t.Fatal("buildParams() error = nil, want safe correlation error")
	}
	if strings.Contains(err.Error(), "unknown-call") || strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaked request data: %v", err)
	}
}

func TestGeminiToolArgumentsRemainLossless(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	input := geminiToolRequest(nil)
	input.Tools[0].Function.Parameters = json.RawMessage(`{"type":"object","properties":{"filter":{"type":"object","additionalProperties":{"type":"string"}}}}`)
	_, _, config, err := newGeminiToolAdapter(server).buildParams(input)
	if err != nil {
		t.Fatalf("buildParams() error = %v", err)
	}
	payload, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !bytes.Contains(payload, []byte(`"additionalProperties":{"type":"string"}`)) {
		t.Errorf("parametersJsonSchema was not preserved: %s", payload)
	}
}

func newGeminiToolAdapter(server *httptest.Server) *Adapter {
	return NewAdapter("gemini", server.URL, "test-key", "gemini-2.5-flash").(*Adapter)
}

func geminiToolRequest(choice *llm.ToolChoice) *llm.LLMRequest {
	return &llm.LLMRequest{
		Model: "gemini-2.5-flash",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "Use the available tools."},
			{Role: llm.RoleUser, Content: "What is the weather in Paris?"},
			{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call-history", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: "weather", Arguments: `{"city":"Paris"}`}}}},
			{Role: llm.RoleTool, ToolCallID: "call-history", Content: `{"forecast":"sunny"}`},
		},
		Tools:      []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: "weather", Description: "Get weather", Parameters: json.RawMessage(geminiToolSchema)}}},
		ToolChoice: choice,
	}
}

func geminiToolServer(t *testing.T, handler func(map[string]any, http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		handler(body, writer)
	}))
}

func geminiToolStreamServer(t *testing.T, events []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := writer.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flush")
			return
		}
		for _, event := range events {
			payload, err := json.Marshal(event)
			if err != nil {
				t.Errorf("marshal stream response: %v", err)
				return
			}
			if _, err = writer.Write(append([]byte("data: "), append(payload, []byte("\n\n")...)...)); err != nil {
				t.Errorf("write stream response: %v", err)
				return
			}
			flusher.Flush()
		}
	}))
}

func geminiCompletion(id, name string, arguments map[string]any, finish string, usage map[string]any) map[string]any {
	parts := []any{}
	if name != "" {
		parts = append(parts, map[string]any{"functionCall": map[string]any{"id": id, "name": name, "args": arguments}})
	}
	candidate := map[string]any{"content": map[string]any{"role": "model", "parts": parts}}
	if finish != "" {
		candidate["finishReason"] = finish
	}
	response := map[string]any{"candidates": []any{candidate}}
	if usage != nil {
		response["usageMetadata"] = usage
	}
	return response
}

func geminiUsage() map[string]any {
	return map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 3, "totalTokenCount": 5}
}

func writeGeminiJSON(t *testing.T, writer http.ResponseWriter, body map[string]any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(body); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func assertGeminiRequest(t *testing.T, request map[string]any, wantMode string, wantNames []string, wantAbsent bool) {
	t.Helper()
	declarations := mapSlice(t, request["tools"], "tools")
	schema := mapValue(t, mapValue(t, declarations[0], "functionDeclarations"), "parametersJsonSchema")
	if schema["additionalProperties"] != false {
		t.Errorf("schema additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	properties := mapValue(t, schema, "properties")
	city := mapValue(t, properties, "city")
	if city["type"] != "string" {
		t.Errorf("schema city type = %#v, want string", city["type"])
	}

	contents := mapSlice(t, request["contents"], "contents")
	if len(contents) < 3 {
		t.Fatalf("contents = %#v, want history and result", contents)
	}
	if contents[1]["role"] != "model" {
		t.Errorf("assistant history role = %#v, want model", contents[1]["role"])
	}
	historyParts := mapSlice(t, contents[1]["parts"], "history parts")
	historyCall := mapValue(t, historyParts[0], "functionCall")
	if historyCall["id"] != "call-history" {
		t.Errorf("history call ID = %#v, want call-history", historyCall["id"])
	}
	resultParts := mapSlice(t, contents[2]["parts"], "result parts")
	result := mapValue(t, resultParts[0], "functionResponse")
	if result["id"] != "call-history" || result["name"] != "weather" {
		t.Errorf("tool result = %#v, want correlated weather result", result)
	}

	config, present := request["toolConfig"]
	if wantAbsent {
		if present {
			t.Errorf("toolConfig = %#v, want absent", config)
		}
		return
	}
	functionConfig := mapValue(t, mapValue(t, config, "functionCallingConfig"), "mode")
	if functionConfig["value"] != wantMode {
		t.Errorf("tool mode = %#v, want %q", functionConfig["value"], wantMode)
	}
	if len(wantNames) == 0 {
		if _, found := functionConfig["allowedFunctionNames"]; found {
			t.Errorf("allowed function names = %#v, want absent", functionConfig["allowedFunctionNames"])
		}
		return
	}
	names := stringSlice(t, functionConfig["allowedFunctionNames"], "allowedFunctionNames")
	if len(names) != 1 || names[0] != wantNames[0] {
		t.Errorf("allowed names = %#v, want %#v", names, wantNames)
	}
}

func mapValue(t *testing.T, value any, name string) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", name, value)
	}
	return result
}

func mapSlice(t *testing.T, value any, name string) []map[string]any {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want array", name, value)
	}
	result := make([]map[string]any, len(raw))
	for index := range raw {
		result[index] = mapValue(t, raw[index], name)
	}
	return result
}

func stringSlice(t *testing.T, value any, name string) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %#v, want string array", name, value)
	}
	result := make([]string, len(raw))
	for index := range raw {
		stringValue, ok := raw[index].(string)
		if !ok {
			t.Fatalf("%s[%d] = %#v, want string", name, index, raw[index])
		}
		result[index] = stringValue
	}
	return result
}

func collectGeminiEvents(stream <-chan llm.StreamEvent) []llm.StreamEvent {
	var events []llm.StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func collectGeminiStream(adapter *Adapter) func(context.Context, *llm.LLMRequest) ([]llm.StreamEvent, error) {
	return func(ctx context.Context, request *llm.LLMRequest) ([]llm.StreamEvent, error) {
		stream, err := adapter.Stream(ctx, request)
		if err != nil {
			return nil, err
		}
		return collectGeminiEvents(stream), nil
	}
}
func streamToolCalls(events []llm.StreamEvent) []llm.ToolCallChunk {
	var calls []llm.ToolCallChunk
	for _, event := range events {
		calls = append(calls, event.ToolCalls...)
	}
	return calls
}

func streamFinish(events []llm.StreamEvent) *llm.StreamEvent {
	for index := range events {
		if events[index].FinishReason != "" {
			return &events[index]
		}
	}
	return nil
}
