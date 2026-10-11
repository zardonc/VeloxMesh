//go:build phase29preflight

package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
)

func TestLiveRawGreetingStreamShape(t *testing.T) {
	env := liveEnvironment(t)
	payload := llm.ChatCompletionRequest{Model: env["SANS_PRIMARY_DEFAULT_MODEL"], Stream: true,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}}
	liveRawStreamShape(t, payload, env)
}

func TestLiveRawToolStreamShape(t *testing.T) {
	env := liveEnvironment(t)
	payload := liveToolRequest(liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	payload.Stream = true
	liveRawStreamShape(t, payload, env)
}

func liveRawStreamShape(t *testing.T, payload llm.ChatCompletionRequest, env map[string]string) {
	t.Helper()
	response := liveRawToolHTTP(t, payload, env)
	defer response.Body.Close()
	liveReadStreamShapes(t, response.Body)
}

func liveReadStreamShapes(t *testing.T, body io.Reader) {
	t.Helper()
	scanner := bufio.NewScanner(body)
	var frames, done int
	for scanner.Scan() {
		line := scanner.Text()
		if line == "data: [DONE]" {
			done++
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		frames++
		var chunk map[string]json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			t.Fatal("actual upstream emitted a non-JSON SSE frame")
		}
		shipLogJSON(t, liveStreamShape(chunk, frames))
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	shipLogJSON(t, map[string]any{"type": "stream_shape_terminal", "frames": frames, "done": done})
	if done != 1 {
		t.Fatal("actual upstream stream missing its unique DONE marker")
	}
}

func liveStreamShape(chunk map[string]json.RawMessage, frame int) map[string]any {
	var choices []struct {
		Delta        map[string]json.RawMessage `json:"delta"`
		FinishReason *string                    `json:"finish_reason"`
	}
	_ = json.Unmarshal(chunk["choices"], &choices)
	var usage *llm.Usage
	_ = json.Unmarshal(chunk["usage"], &usage)
	shape := map[string]any{"type": "stream_shape", "frame": frame, "choices": len(choices), "usage": usage}
	for index, choice := range choices {
		keys := make([]string, 0, len(choice.Delta))
		for key := range choice.Delta {
			keys = append(keys, key)
		}
		var content, reasoning string
		var toolCalls []json.RawMessage
		_ = json.Unmarshal(choice.Delta["content"], &content)
		_ = json.Unmarshal(choice.Delta["reasoning"], &reasoning)
		_ = json.Unmarshal(choice.Delta["tool_calls"], &toolCalls)
		shape[fmt.Sprint("choice_", index)] = map[string]any{"delta_keys": keys, "finish_reason": choice.FinishReason,
			"content_length": len(content), "reasoning_length": len(reasoning), "tool_calls": len(toolCalls)}
	}
	return shape
}

func TestLiveRawRequiredToolContinuation(t *testing.T) {
	env := liveEnvironment(t)
	payload := liveToolRequest(liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	result := liveDecodeChat(t, liveRawToolHTTP(t, payload, env))
	message := result.Choices[0].Message
	if len(message.ToolCalls) != 1 {
		t.Fatal("actual upstream did not return one required tool call")
	}
	output := liveExecuteTool(t, message.ToolCalls[0])
	followup := llm.ChatCompletionRequest{Model: payload.Model, Tools: payload.Tools,
		ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceNone},
		Messages:   append(append([]llm.Message{}, payload.Messages...), message, output)}
	answer := liveDecodeChat(t, liveRawToolHTTP(t, followup, env))
	if !strings.Contains(answer.Choices[0].Message.Content, fmt.Sprint(liveExpectedSum)) {
		t.Fatal("actual upstream tool continuation returned the wrong result")
	}
	t.Log("direct actual upstream required tool continuation passed")
}
