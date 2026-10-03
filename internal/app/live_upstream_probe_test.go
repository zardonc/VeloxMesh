//go:build phase29preflight

package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
)

func liveRawToolHTTP(t *testing.T, payload llm.ChatCompletionRequest, env map[string]string) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(env["SANS_BASE_URL"], "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+env["SANS_PRIMARY_API_KEY"])
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatalf("direct upstream transport: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("direct tool status=%d read_error=%v body=%s", response.StatusCode, err, data)
	}
	return response
}

func TestLiveRawNamedTool(t *testing.T) {
	env := liveEnvironment(t)
	chain := liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}
	payload := liveToolRequest(chain, &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "add"})
	response := liveRawToolHTTP(t, payload, env)
	result := liveDecodeChat(t, response)
	if len(result.Choices[0].Message.ToolCalls) != 1 {
		t.Fatal("direct upstream named tool missing")
	}
	liveExecuteTool(t, result.Choices[0].Message.ToolCalls[0])
}

func TestLiveRawToolStream(t *testing.T) {
	env := liveEnvironment(t)
	payload := liveToolRequest(liveChain{model: env["SANS_PRIMARY_DEFAULT_MODEL"]}, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	payload.Stream = true
	response := liveRawToolHTTP(t, payload, env)
	defer response.Body.Close()
	call, done, finish := liveStreamTool(t, response.Body)
	t.Logf("direct tool SSE done=%d finish=%s name=%s arguments_length=%d", done, finish, call.Function.Name, len(call.Function.Arguments))
	if done != 1 || finish != "tool_calls" {
		t.Fatal("direct upstream invalid tool stream terminal")
	}
	liveExecuteTool(t, call)
}
