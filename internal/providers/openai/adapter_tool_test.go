package openai

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

const testToolName = "lookup_weather"

func TestToolRequestPreservesDefinitionsChoicesAndHistory(t *testing.T) {
	choices := []*llm.ToolChoice{
		nil,
		{Mode: llm.ToolChoiceAuto},
		{Mode: llm.ToolChoiceNone},
		{Mode: llm.ToolChoiceRequired},
		{Mode: llm.ToolChoiceNamed, FunctionName: testToolName},
	}
	for _, choice := range choices {
		t.Run(toolChoiceName(choice), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				assertToolWireRequest(t, body, choice)
				_, _ = w.Write([]byte(toolCompletion("call-provider", "tool_calls", "{\"city\":\"Paris\"}", 1)))
			}))
			defer server.Close()

			response, err := NewAdapter("openai-test", server.URL, "", "gpt-4").Complete(context.Background(), toolRequest(choice))
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}
			if response.Choices[0].Message.ToolCalls[0].ID != "call-provider" {
				t.Fatalf("provider call ID was not preserved: %#v", response.Choices[0].Message.ToolCalls)
			}
		})
	}
}

func TestToolContractStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(streamData(toolDelta("call-provider", testToolName, "{\"city\":\"Paris\"}", "tool_calls")) + "data: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := NewAdapter("openai-test", server.URL, "", "gpt-4")
	err := adaptertest.RunToolContract(context.Background(), adaptertest.ToolContractFixture{
		Stream: func(ctx context.Context, request *llm.LLMRequest) ([]llm.StreamEvent, error) {
			stream, err := adapter.Stream(ctx, request)
			if err != nil {
				return nil, err
			}
			events := make([]llm.StreamEvent, 0)
			for event := range stream {
				events = append(events, event)
			}
			return events, nil
		},
	}, adaptertest.ToolContractCase{Name: "stream tool call", Request: toolRequest(nil), Stream: true, ExpectedFinishReason: "tool_calls"}, func(adaptertest.ToolContractCompletion) {})
	if err != nil {
		t.Fatalf("RunToolContract() error = %v", err)
	}
}

func TestToolContractComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(toolCompletion("call-provider", "tool_calls", "{\"city\":\"Paris\"}", 1)))
	}))
	defer server.Close()

	adapter := NewAdapter("openai-test", server.URL, "", "gpt-4")
	calls := 0
	err := adaptertest.RunToolContract(context.Background(), adaptertest.ToolContractFixture{
		Complete: adapter.Complete,
	}, adaptertest.ToolContractCase{
		Name: "complete tool call", Request: toolRequest(&llm.ToolChoice{Mode: llm.ToolChoiceRequired}), ExpectedFinishReason: "tool_calls",
	}, func(adaptertest.ToolContractCompletion) { calls++ })
	if err != nil {
		t.Fatalf("RunToolContract() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("completion observer calls = %d, want 1", calls)
	}
}

func TestToolCompleteGeneratesMissingProviderID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(toolCompletion("", "tool_calls", "{\"city\":\"Paris\"}", 1)))
	}))
	defer server.Close()

	adapter := NewAdapter("openai-test", server.URL, "", "gpt-4")
	adapter.generateToolCallID = func() string { return "call-generated" }
	response, err := adapter.Complete(context.Background(), toolRequest(nil))
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got := response.Choices[0].Message.ToolCalls[0].ID; got != "call-generated" {
		t.Fatalf("generated tool call ID = %q", got)
	}
}

func TestToolCompleteRejectsMalformedProviderCalls(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "duplicate ID", body: toolCompletion("call-1", "tool_calls", "{\"city\":\"Paris\"}", 2)},
		{name: "invalid arguments", body: toolCompletion("call-1", "tool_calls", "{not-json}", 1)},
		{name: "finish conflict", body: toolCompletion("call-1", "stop", "{\"city\":\"Paris\"}", 1)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			_, err := NewAdapter("openai-test", server.URL, "", "gpt-4").Complete(context.Background(), toolRequest(nil))
			assertProviderBadResponse(t, err)
		})
	}
}

func TestToolStreamEmitsFirstFragmentBeforeBodyCloses(t *testing.T) {
	firstWritten := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(streamData(toolDelta("call-provider", testToolName, "{\"city\":", ""))))
		flusher.Flush()
		close(firstWritten)
		<-release
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	stream, err := NewAdapter("openai-test", server.URL, "", "gpt-4").Stream(context.Background(), toolRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	<-firstWritten
	select {
	case event := <-stream:
		if len(event.ToolCalls) != 1 || event.ToolCalls[0].ID == nil {
			t.Fatalf("first event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("first tool fragment was buffered until the provider body closed")
	}
	close(release)
	for range stream {
	}
}

func TestToolStreamRejectsIncompleteOrUnsafeTerminal(t *testing.T) {
	validArguments := "{\"city\":\"Paris\"}"
	cases := []struct {
		name string
		body string
	}{
		{name: "premature EOF", body: streamData(toolDelta("call-1", testToolName, "{", ""))},
		{name: "malformed SSE", body: "data: {not-json}\n\n"},
		{name: "finish conflict", body: streamData(toolDelta("call-1", testToolName, validArguments, "stop"))},
		{name: "identity drift", body: streamData(toolDelta("call-1", testToolName, "{", "")) + streamData(toolDelta("call-2", testToolName, validArguments, ""))},
		{name: "post completion fragment", body: streamData(toolDelta("call-1", testToolName, validArguments, "tool_calls")) + streamData(toolDelta("call-1", testToolName, "{}", ""))},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			stream, err := NewAdapter("openai-test", server.URL, "", "gpt-4").Stream(context.Background(), toolRequest(nil))
			if err != nil {
				t.Fatal(err)
			}
			var events []llm.StreamEvent
			for event := range stream {
				events = append(events, event)
			}
			if len(events) == 0 || events[len(events)-1].Done {
				t.Fatalf("invalid stream events = %#v", events)
			}
			assertProviderBadResponse(t, events[len(events)-1].Error)
		})
	}
}

func toolRequest(choice *llm.ToolChoice) *llm.LLMRequest {
	return &llm.LLMRequest{
		Model: "gpt-4",
		Tools: []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{
			Name: testToolName, Description: "lookup weather",
			Parameters: json.RawMessage("{\"type\":\"object\",\"properties\":{\"city\":{\"type\":\"string\",\"x-nested\":{\"type\":\"array\",\"items\":{\"type\":\"string\"}}}},\"required\":[\"city\"],\"additionalProperties\":false}"),
		}}},
		ToolChoice: choice,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "weather"},
			{Role: llm.RoleAssistant, Content: "checking", ToolCalls: []llm.ToolCall{{ID: "call-history-1", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: testToolName, Arguments: "{\"city\":\"Paris\"}"}}, {ID: "call-history-2", Type: llm.ToolTypeFunction, Function: llm.FunctionCall{Name: testToolName, Arguments: "{\"city\":\"Lyon\"}"}}}},
			{Role: llm.RoleTool, ToolCallID: "call-history-2", Content: "{\"temp\":20}"},
			{Role: llm.RoleTool, ToolCallID: "call-history-1", Content: "{\"temp\":18}"},
		},
	}
}

func toolChoiceName(choice *llm.ToolChoice) string {
	if choice == nil {
		return "omitted"
	}
	if choice.Mode == llm.ToolChoiceNamed {
		return "named"
	}
	return string(choice.Mode)
}

func assertToolWireRequest(t *testing.T, body map[string]json.RawMessage, choice *llm.ToolChoice) {
	t.Helper()
	if choice == nil {
		if _, ok := body["tool_choice"]; ok {
			t.Fatal("omitted tool_choice was sent")
		}
	} else if string(body["tool_choice"]) != expectedChoiceJSON(choice) {
		t.Fatalf("tool_choice = %s", body["tool_choice"])
	}
	if !strings.Contains(string(body["tools"]), "\"x-nested\"") {
		t.Fatalf("nested schema was downgraded: %s", body["tools"])
	}
	if !strings.Contains(string(body["messages"]), "\"tool_call_id\":\"call-history-2\"") || !strings.Contains(string(body["messages"]), "\"tool_calls\"") {
		t.Fatalf("tool history was downgraded: %s", body["messages"])
	}
}

func expectedChoiceJSON(choice *llm.ToolChoice) string {
	if choice.Mode == llm.ToolChoiceNamed {
		return "{\"type\":\"function\",\"function\":{\"name\":\"lookup_weather\"}}"
	}
	return "\"" + string(choice.Mode) + "\""
}

func toolCompletion(id, finish, arguments string, count int) string {
	calls := make([]map[string]any, count)
	for index := range calls {
		call := map[string]any{"type": "function", "function": map[string]any{"name": testToolName, "arguments": arguments}}
		if id != "" {
			call["id"] = id
		}
		calls[index] = call
	}
	return stringJSON(map[string]any{"model": "gpt-4", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "checking", "tool_calls": calls}, "finish_reason": finish}}})
}

func toolDelta(id, name, arguments, finish string) map[string]any {
	call := map[string]any{
		"index": 0, "id": id, "type": "function",
		"function": map[string]any{"name": name, "arguments": arguments},
	}
	delta := map[string]any{"tool_calls": []any{call}}
	choice := map[string]any{"index": 0, "delta": delta}
	if finish != "" {
		choice["finish_reason"] = finish
	}
	return map[string]any{"model": "gpt-4", "choices": []any{choice}}
}

func streamData(value any) string {
	return "data: " + stringJSON(value) + "\n\n"
}

func stringJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func assertProviderBadResponse(t *testing.T, err error) {
	t.Helper()
	gatewayErr, ok := err.(*gatewayerrors.GatewayError)
	if !ok || gatewayErr.Code != gatewayerrors.ProviderBadResponse {
		t.Fatalf("error = %v, want provider_bad_response", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "arguments") {
		t.Fatalf("error leaked provider payload: %v", err)
	}
}
