package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/adaptertest"
)

const anthropicToolName = "lookup_weather"

func TestToolRequestPreservesDefinitionsChoicesAndHistory(t *testing.T) {
	choices := []*llm.ToolChoice{
		nil,
		{Mode: llm.ToolChoiceAuto},
		{Mode: llm.ToolChoiceNone},
		{Mode: llm.ToolChoiceRequired},
		{Mode: llm.ToolChoiceNamed, FunctionName: anthropicToolName},
	}
	for _, choice := range choices {
		t.Run(toolChoiceName(choice), func(t *testing.T) {
			var requestBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer r.Body.Close()
				if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				writeAnthropicJSON(w, toolCompletion("call-provider", "tool_use", "{\"city\":\"Paris\"}"))
			}))
			defer server.Close()

			response, err := newToolAdapter(server).Complete(context.Background(), anthropicToolRequest(choice))
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if got := response.Choices[0].FinishReason; got != "tool_calls" {
				t.Fatalf("finish reason = %q", got)
			}
			assertAnthropicRequest(t, requestBody, choice)
		})
	}
}

func TestToolCompleteNormalizesCallsUsageAndFailures(t *testing.T) {
	cases := []struct {
		name string
		body any
		code string
	}{
		{name: "multiple calls preserve IDs and usage", body: map[string]any{
			"id": "msg-tool", "type": "message", "role": "assistant", "model": "claude-test", "stop_reason": "tool_use",
			"content": []map[string]any{
				{"type": "text", "text": "Checking."},
				{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{"city": "Paris"}},
				{"type": "tool_use", "id": "call-2", "name": "lookup_time", "input": map[string]any{"city": "Tokyo"}},
			},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_creation_input_tokens": 2, "cache_read_input_tokens": 3},
		}},
		{name: "duplicate IDs fail closed", body: toolCompletionWithBlocks("tool_use", []map[string]any{
			{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}},
			{"type": "tool_use", "id": "call-1", "name": "lookup_time", "input": map[string]any{}},
		}), code: gatewayerrors.ProviderBadResponse},
		{name: "missing ID is generated", body: toolCompletion("", "tool_use", "{\"city\":\"Paris\"}")},
		{name: "non-object input fails closed", body: toolCompletionWithRawInput([]any{"not-an-object"}), code: gatewayerrors.ProviderBadResponse},
		{name: "stop conflict fails closed", body: toolCompletion("call-1", "end_turn", "{\"city\":\"Paris\"}"), code: gatewayerrors.ProviderBadResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeAnthropicJSON(w, tc.body)
			}))
			defer server.Close()
			adapter := newToolAdapter(server)

			response, err := adapter.Complete(context.Background(), anthropicToolRequest(nil))
			if tc.code != "" {
				assertAnthropicProviderBadResponse(t, err)
				return
			}
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if tc.name == "missing ID is generated" && response.Choices[0].Message.ToolCalls[0].ID == "" {
				t.Fatalf("missing generated ID")
			}
			if tc.name == "multiple calls preserve IDs and usage" {
				if got := response.Choices[0].Message.ToolCalls; len(got) != 2 || got[0].ID != "call-1" || got[1].ID != "call-2" {
					t.Fatalf("tool calls = %#v", got)
				}
				if response.Usage == nil || response.Usage.PromptTokens != 15 || response.Usage.CompletionTokens != 5 || response.Usage.TotalTokens != 20 {
					t.Fatalf("usage = %#v", response.Usage)
				}
			}
		})
	}
}

func TestToolStreamEmitsPartialJSONAndRejectsUnsafeLifecycle(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantFinish string
		wantError  bool
	}{
		{name: "valid interleaved blocks", body: anthropicStream(
			anthropicEvent("message_start", map[string]any{"message": map[string]any{"usage": map[string]any{"input_tokens": 4}}}),
			anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}}),
			anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{\"city\":"}}),
			anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "\"Paris\"}"}}),
			anthropicEvent("content_block_stop", map[string]any{"index": 0}),
			anthropicEvent("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "tool_use"}, "usage": map[string]any{"output_tokens": 2}}),
			anthropicEvent("message_stop", map[string]any{}),
		), wantFinish: "tool_calls"},
		{name: "premature EOF", body: anthropicStream(
			anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}}),
			anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{"}}),
		), wantError: true},
		{name: "invalid final JSON", body: anthropicStream(
			anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}}),
			anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{"}}),
			anthropicEvent("content_block_stop", map[string]any{"index": 0}),
			anthropicEvent("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "tool_use"}}),
			anthropicEvent("message_stop", map[string]any{}),
		), wantError: true},
		{name: "identity drift", body: anthropicStream(
			anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}}),
			anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-2", "name": anthropicToolName, "input": map[string]any{}}}),
		), wantError: true},
		{name: "malformed SSE", body: "event: content_block_start\ndata: {not-json}\n\n", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			stream, err := newToolAdapter(server).Stream(context.Background(), anthropicToolRequest(nil))
			if err != nil {
				t.Fatalf("Stream() error = %v", err)
			}
			events := collectAnthropicEvents(stream)
			if tc.wantError {
				assertAnthropicStreamFailure(t, events)
				return
			}
			assertAnthropicPartialStream(t, events, tc.wantFinish)
		})
	}
}

func TestToolStreamDeliversFirstFragmentBeforeProviderCloses(t *testing.T) {
	firstWritten := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}})))
		flusher.Flush()
		close(firstWritten)
		<-release
		_, _ = w.Write([]byte(anthropicStream(
			anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"}}),
			anthropicEvent("content_block_stop", map[string]any{"index": 0}),
			anthropicEvent("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "tool_use"}}),
			anthropicEvent("message_stop", map[string]any{}),
		)))
	}))
	defer server.Close()

	stream, err := newToolAdapter(server).Stream(context.Background(), anthropicToolRequest(nil))
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	<-firstWritten
	select {
	case event := <-stream:
		if len(event.ToolCalls) != 1 || event.ToolCalls[0].ID == nil || *event.ToolCalls[0].ID != "call-1" {
			t.Fatalf("first event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("first tool fragment was buffered until the provider body closed")
	}
	close(release)
	for range stream {
	}
}

func TestToolContractCompleteAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(anthropicStream(
				anthropicEvent("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": map[string]any{}}}),
				anthropicEvent("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": "{}"}}),
				anthropicEvent("content_block_stop", map[string]any{"index": 0}),
				anthropicEvent("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "tool_use"}}),
				anthropicEvent("message_stop", map[string]any{}),
			)))
			return
		}
		writeAnthropicJSON(w, toolCompletion("call-1", "tool_use", "{}"))
	}))
	defer server.Close()

	adapter := newToolAdapter(server)
	fixture := adaptertest.ToolContractFixture{Complete: adapter.Complete, Stream: func(ctx context.Context, request *llm.LLMRequest) ([]llm.StreamEvent, error) {
		stream, err := adapter.Stream(ctx, request)
		if err != nil {
			return nil, err
		}
		return collectAnthropicEvents(stream), nil
	}}
	for _, contract := range []adaptertest.ToolContractCase{
		{Name: "complete", Request: anthropicToolRequest(&llm.ToolChoice{Mode: llm.ToolChoiceRequired}), ExpectedFinishReason: "tool_calls"},
		{Name: "stream", Request: anthropicToolRequest(nil), Stream: true, ExpectedFinishReason: "tool_calls"},
	} {
		calls := 0
		if err := adaptertest.RunToolContract(context.Background(), fixture, contract, func(adaptertest.ToolContractCompletion) { calls++ }); err != nil {
			t.Fatalf("%s contract error = %v", contract.Name, err)
		}
		if calls != 1 {
			t.Fatalf("%s observer calls = %d", contract.Name, calls)
		}
	}
}

func newToolAdapter(server *httptest.Server) *Adapter {
	return NewAdapter("anthropic-test", server.URL+"/", "test-key", "claude-test").(*Adapter)
}

func anthropicToolRequest(choice *llm.ToolChoice) *llm.LLMRequest {
	schema := json.RawMessage(`{"type":"object","properties":{"city":{"type":"string","minLength":2}},"required":["city"],"additionalProperties":false}`)
	return &llm.LLMRequest{
		Model:      "claude-test",
		Tools:      []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: anthropicToolName, Description: "Look up weather", Parameters: schema}}},
		ToolChoice: choice,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "What is the weather?"},
			{Role: llm.RoleAssistant, Content: "Checking.", ToolCalls: []llm.ToolCall{{ID: "call-history", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: anthropicToolName, Arguments: "{\"city\":\"Paris\"}"}}}},
			{Role: llm.RoleTool, ToolCallID: "call-history", Content: "sunny"},
			{Role: llm.RoleUser, Content: "Continue."},
		},
	}
}

func assertAnthropicRequest(t *testing.T, body map[string]any, choice *llm.ToolChoice) {
	t.Helper()
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v", body["tools"])
	}
	inputSchema, ok := tools[0].(map[string]any)["input_schema"].(map[string]any)
	if !ok || inputSchema["additionalProperties"] != false {
		t.Fatalf("input schema = %#v", tools[0])
	}
	if choice == nil {
		if _, exists := body["tool_choice"]; exists {
			t.Fatalf("omitted tool choice serialized as %#v", body["tool_choice"])
		}
	} else if got := body["tool_choice"]; !anthropicChoiceMatches(got, choice) {
		t.Fatalf("tool choice = %#v for %#v", got, choice)
	}
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 4 {
		t.Fatalf("messages = %#v", body["messages"])
	}
	assistant := messages[1].(map[string]any)
	assistantBlocks := assistant["content"].([]any)
	if len(assistantBlocks) != 2 || assistantBlocks[1].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("assistant history = %#v", assistant)
	}
	result := messages[2].(map[string]any)["content"].([]any)
	if len(result) != 1 || result[0].(map[string]any)["tool_use_id"] != "call-history" {
		t.Fatalf("tool result history = %#v", messages[2])
	}
}

func anthropicChoiceMatches(value any, choice *llm.ToolChoice) bool {
	mapped, ok := value.(map[string]any)
	if !ok {
		return false
	}
	switch choice.Mode {
	case llm.ToolChoiceAuto:
		return mapped["type"] == "auto"
	case llm.ToolChoiceNone:
		return mapped["type"] == "none"
	case llm.ToolChoiceRequired:
		return mapped["type"] == "any"
	case llm.ToolChoiceNamed:
		return mapped["type"] == "tool" && mapped["name"] == choice.FunctionName
	default:
		return false
	}
}

func toolChoiceName(choice *llm.ToolChoice) string {
	if choice == nil {
		return "omitted"
	}
	return string(choice.Mode)
}

func toolCompletion(id, stopReason, arguments string) map[string]any {
	var input any
	_ = json.Unmarshal([]byte(arguments), &input)
	return toolCompletionWithBlocks(stopReason, []map[string]any{{"type": "tool_use", "id": id, "name": anthropicToolName, "input": input}})
}

func toolCompletionWithBlocks(stopReason string, blocks []map[string]any) map[string]any {
	return map[string]any{"id": "msg-tool", "type": "message", "role": "assistant", "model": "claude-test", "content": blocks, "stop_reason": stopReason, "usage": map[string]any{"input_tokens": 1, "output_tokens": 1}}
}

func toolCompletionWithRawInput(input any) map[string]any {
	return toolCompletionWithBlocks("tool_use", []map[string]any{{"type": "tool_use", "id": "call-1", "name": anthropicToolName, "input": input}})
}

func writeAnthropicJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func anthropicEvent(event string, body map[string]any) string {
	body["type"] = event
	encoded, _ := json.Marshal(body)
	return "event: " + event + "\ndata: " + string(encoded) + "\n\n"
}

func anthropicStream(events ...string) string {
	return strings.Join(events, "")
}

func collectAnthropicEvents(stream <-chan llm.StreamEvent) []llm.StreamEvent {
	events := make([]llm.StreamEvent, 0)
	for event := range stream {
		events = append(events, event)
	}
	return events
}

func assertAnthropicPartialStream(t *testing.T, events []llm.StreamEvent, finishReason string) {
	t.Helper()
	if len(events) < 4 {
		t.Fatalf("events = %#v", events)
	}
	first := events[1]
	if len(first.ToolCalls) != 1 || first.ToolCalls[0].ID == nil || *first.ToolCalls[0].ID != "call-1" {
		t.Fatalf("first tool event = %#v", first)
	}
	if len(events[2].ToolCalls) != 1 || events[2].ToolCalls[0].Function == nil || events[2].ToolCalls[0].Function.Arguments == nil {
		t.Fatalf("first partial event = %#v", events[2])
	}
	foundFinish := false
	foundDone := false
	for _, event := range events {
		foundFinish = foundFinish || event.FinishReason == finishReason
		foundDone = foundDone || event.Done
	}
	if !foundFinish || !foundDone {
		t.Fatalf("finish/done = %#v", events)
	}
}

func assertAnthropicStreamFailure(t *testing.T, events []llm.StreamEvent) {
	t.Helper()
	for _, event := range events {
		if event.Done {
			t.Fatalf("unexpected Done event: %#v", events)
		}
		if event.Error != nil {
			assertAnthropicProviderBadResponse(t, event.Error)
			return
		}
	}
	t.Fatalf("missing provider_bad_response: %#v", events)
}

func assertAnthropicProviderBadResponse(t *testing.T, err error) {
	t.Helper()
	gatewayError, ok := err.(*gatewayerrors.GatewayError)
	if !ok || gatewayError.Code != gatewayerrors.ProviderBadResponse {
		t.Fatalf("error = %T %v", err, err)
	}
	if strings.Contains(err.Error(), "test-key") || strings.Contains(err.Error(), "partial_json") || strings.Contains(err.Error(), "city") {
		t.Fatalf("unsafe provider error: %v", err)
	}
}
