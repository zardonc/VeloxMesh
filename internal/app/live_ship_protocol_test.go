//go:build phase29preflight

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

const liveHTTPTimeout = 12 * time.Second
const liveSettleWait = 2 * time.Second
const livePollInterval = 20 * time.Millisecond
const liveTestCreditRate = 1000
const liveExpectedSum = 19 + 23

type liveChain struct {
	app               *App
	repo              *sqlite.Repository
	url, token, model string
	initialBalance    int
}

func newLiveChain(t *testing.T) liveChain {
	t.Helper()
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real components opt-in")
	}
	env := liveEnvironment(t)
	repo, dsn, keyID := liveRepository(t)
	application, token := liveApplicationWithDatabase(t, env, liveDatabase{dsn: dsn, keyID: keyID})
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	model := env["SANS_PRIMARY_DEFAULT_MODEL"]
	err := repo.Rates().Save(context.Background(), &controlstate.ProviderModelRate{
		ProviderID: "sans-primary", Model: model, InputCreditRate: liveTestCreditRate, OutputCreditRate: liveTestCreditRate,
	})
	if err != nil {
		t.Fatal(err)
	}
	var balance int
	if err := repo.DBForTest().QueryRow("SELECT credit_balance FROM api_keys WHERE id = ?", token).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return liveChain{app: application, repo: repo, url: server.URL, token: token, model: model, initialBalance: balance}
}

func liveHTTP(t *testing.T, chain liveChain, payload any) *http.Response {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, chain.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+chain.token)
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		// Retain error categories without credentials or raw request/answer payloads.
		var problem gatewayerrors.GatewayError
		decodeErr := json.Unmarshal(body, &problem)
		t.Fatalf("real gateway status=%d error_code=%s decode_error=%v read_error=%v", response.StatusCode, problem.Code, decodeErr, readErr)
	}
	return response
}

func liveUsageCount(t *testing.T, chain liveChain) int {
	t.Helper()
	var count int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM usage_records WHERE api_key_id = ?", chain.token).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func liveAssertSettlement(t *testing.T, chain liveChain, expected int) {
	t.Helper()
	deadline := time.Now().Add(liveSettleWait)
	for liveUsageCount(t, chain) < expected && time.Now().Before(deadline) {
		time.Sleep(livePollInterval)
	}
	var count, tokens, credits, settled int
	err := chain.repo.DBForTest().QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens),0), COALESCE(SUM(credits_consumed),0), COUNT(CASE WHEN status = 'settled' THEN 1 END) FROM usage_records WHERE api_key_id = ?`, chain.token).Scan(&count, &tokens, &credits, &settled)
	if err != nil {
		t.Fatal(err)
	}
	var allRows int
	if err := chain.repo.DBForTest().QueryRow("SELECT COUNT(*) FROM usage_records").Scan(&allRows); err != nil {
		t.Fatal(err)
	}
	t.Logf("usage_rows_all=%d usage_rows_key=%d", allRows, count)
	if count != expected || settled != expected || tokens <= 0 || credits != tokens {
		t.Fatalf("settlement count=%d expected=%d settled=%d tokens=%d credits=%d", count, expected, settled, tokens, credits)
	}
	liveAssertDebit(t, chain, credits)
	time.Sleep(livePollInterval)
	if liveUsageCount(t, chain) != expected {
		t.Fatal("duplicate settlement")
	}
	t.Logf("settled_records=%d total_tokens=%d credits=%d exactly_once=true", count, tokens, credits)
}

func liveAssertDebit(t *testing.T, chain liveChain, credits int) {
	t.Helper()
	var balance int
	err := chain.repo.DBForTest().QueryRow("SELECT credit_balance FROM api_keys WHERE id = ?", chain.token).Scan(&balance)
	if err != nil || balance != chain.initialBalance-credits {
		t.Fatalf("balance mismatch: error=%v", err)
	}
}

func TestLiveStreamSettlement(t *testing.T) {
	chain := newLiveChain(t)
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with a short greeting."}}})
	defer response.Body.Close()
	var done, fragments int
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		liveRejectSSEError(t, line)
		if line == "data: [DONE]" {
			done++
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk llm.ChatCompletionChunkResponse
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			t.Fatal(err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				fragments++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if done != 1 || fragments == 0 {
		t.Fatalf("SSE done=%d content_fragments=%d", done, fragments)
	}
	t.Logf("real SSE done=%d fragments=%d cache_hit=%s", done, fragments, response.Header.Get("X-Cache-Hit"))
	liveAssertSettlement(t, chain, 1)
}

func TestLiveStreamCancellation(t *testing.T) {
	chain := newLiveChain(t)
	payload := llm.ChatCompletionRequest{Model: chain.model, Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "List the integers from one to one thousand, writing each integer on a separate line."}}}
	response := liveHTTP(t, chain, payload)
	defer response.Body.Close()
	liveReadFirstFragment(t, response.Body)
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(liveSettleWait)
	if liveUsageCount(t, chain) != 0 {
		t.Fatal("cancelled stream was settled")
	}
	if snapshot := chain.app.HealthStore().Snapshot("sans-primary"); snapshot.TotalFailures != 0 || snapshot.PendingRequests != 0 {
		t.Fatalf("disconnect affected provider health: failures=%d pending=%d", snapshot.TotalFailures, snapshot.PendingRequests)
	}
	t.Log("real client disconnected after a fragment; no usage settlement")
}

func liveReadFirstFragment(t *testing.T, body io.Reader) {
	t.Helper()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		liveRejectSSEError(t, line)
		if line == "data: [DONE]" {
			t.Fatal("upstream completed before cancellation; cancellation untested")
		}
		var chunk llm.ChatCompletionChunkResponse
		if strings.HasPrefix(line, "data: ") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
				t.Fatal(err)
			}
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			return
		}
	}
	t.Fatalf("no stream fragment before disconnect: %v", scanner.Err())
}

func liveToolRequest(chain liveChain, choice *llm.ToolChoice) llm.ChatCompletionRequest {
	return llm.ChatCompletionRequest{Model: chain.model, ToolChoice: choice,
		Tools:    []llm.Tool{{Type: llm.ToolTypeFunction, Function: &llm.Function{Name: "add", Description: "Add two integers.", Parameters: json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"},"b":{"type":"integer"}},"required":["a","b"],"additionalProperties":false}`)}}},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Use the add function to compute 19 plus 23. Return only the result after receiving the tool output."}},
	}
}

func liveDecodeChat(t *testing.T, response *http.Response) llm.ChatCompletionResponse {
	t.Helper()
	defer response.Body.Close()
	var result llm.ChatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("choices=%d", len(result.Choices))
	}
	return result
}

func liveExecuteTool(t *testing.T, call llm.ToolCall) llm.Message {
	t.Helper()
	liveAssertToolMetadata(t, call)
	var args struct{ A, B *int }
	if call.ID == "" || call.Type != llm.ToolTypeFunction || call.Function.Name != "add" {
		t.Fatal("invalid tool identity")
	}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || args.A == nil || args.B == nil {
		t.Fatal("invalid real model tool arguments")
	}
	if *args.A != 19 || *args.B != 23 {
		t.Fatal("unexpected tool inputs")
	}
	output, err := json.Marshal(map[string]int{"result": *args.A + *args.B})
	if err != nil {
		t.Fatal(err)
	}
	return llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: string(output)}
}

func liveAssertToolMetadata(t *testing.T, call llm.ToolCall) {
	t.Helper()
	if os.Getenv("SHIP_PROVIDER_TYPE") != "gemini" {
		return
	}
	encoded, err := json.Marshal(call)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Extra *struct {
			Google struct {
				Signature string `json:"thought_signature"`
			} `json:"google"`
		} `json:"extra_content"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil || wire.Extra == nil || wire.Extra.Google.Signature == "" {
		t.Fatal("real Gemini tool metadata missing from the public HTTP response")
	}
	t.Log("real Gemini opaque tool signature preserved in public response")
}

func liveToolRoundTrip(t *testing.T, choice *llm.ToolChoice) {
	t.Helper()
	chain := newLiveChain(t)
	request := liveToolRequest(chain, choice)
	result := liveDecodeChat(t, liveHTTP(t, chain, request))
	message := result.Choices[0].Message
	if result.Choices[0].FinishReason != "tool_calls" || len(message.ToolCalls) != 1 {
		t.Fatal("real model did not produce one tool call")
	}
	toolOutput := liveExecuteTool(t, message.ToolCalls[0])
	followup := llm.ChatCompletionRequest{Model: chain.model, Tools: request.Tools, ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceNone}, Messages: append(append([]llm.Message{}, request.Messages...), message, toolOutput)}
	answer := liveDecodeChat(t, liveHTTP(t, chain, followup))
	if !strings.Contains(answer.Choices[0].Message.Content, fmt.Sprint(liveExpectedSum)) || len(answer.Choices[0].Message.ToolCalls) != 0 {
		t.Fatal("tool result continuation failed")
	}
	liveAssertSettlement(t, chain, 2)
	t.Log("real model -> tool arguments -> client calculation -> tool_call_id continuation -> final answer passed")
}

func TestLiveToolRequired(t *testing.T) {
	liveToolRoundTrip(t, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
}
func TestLiveToolNamed(t *testing.T) {
	liveToolRoundTrip(t, &llm.ToolChoice{Mode: llm.ToolChoiceNamed, FunctionName: "add"})
}
func TestLiveToolAuto(t *testing.T)    { liveToolRoundTrip(t, &llm.ToolChoice{Mode: llm.ToolChoiceAuto}) }
func TestLiveToolOmitted(t *testing.T) { liveToolRoundTrip(t, nil) }

func TestLiveToolInvalidSignature(t *testing.T) {
	chain := newLiveChain(t)
	if os.Getenv("SHIP_PROVIDER_TYPE") != "gemini" {
		t.Skip("real Gemini signature validation")
	}
	request := liveToolRequest(chain, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	result := liveDecodeChat(t, liveHTTP(t, chain, request))
	message := result.Choices[0].Message
	if len(message.ToolCalls) != 1 {
		t.Fatal("real model did not produce one tool call")
	}
	output := liveExecuteTool(t, message.ToolCalls[0])
	call := message.ToolCalls[0]
	call.ExtraContent = &llm.ToolCallExtraContent{Google: llm.GoogleToolCallExtraContent{ThoughtSignature: "invalid-base64%"}}
	message.ToolCalls = []llm.ToolCall{call}
	followup := llm.ChatCompletionRequest{Model: chain.model, Tools: request.Tools, Messages: append(append([]llm.Message{}, request.Messages...), message, output)}
	body, err := json.Marshal(followup)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, chain.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+chain.token)
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var problem gatewayerrors.GatewayError
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || problem.Code != gatewayerrors.InvalidRequest {
		t.Fatalf("malformed signature status=%d error_code=%s", response.StatusCode, problem.Code)
	}
	liveAssertSettlement(t, chain, 1)
	t.Log("malformed client signature rejected by gateway; no additional settlement")
}

func TestLiveToolNone(t *testing.T) {
	chain := newLiveChain(t)
	request := liveToolRequest(chain, &llm.ToolChoice{Mode: llm.ToolChoiceNone})
	request.Messages = []llm.Message{{Role: llm.RoleUser, Content: "Compute 19 plus 23. Reply in text without calling a tool."}}
	answer := liveDecodeChat(t, liveHTTP(t, chain, request))
	if len(answer.Choices[0].Message.ToolCalls) != 0 || answer.Choices[0].Message.Content == "" {
		t.Fatalf("tool_choice none failed: tool_calls=%d content_length=%d finish=%s", len(answer.Choices[0].Message.ToolCalls), len(answer.Choices[0].Message.Content), answer.Choices[0].FinishReason)
	}
	liveAssertSettlement(t, chain, 1)
}

func TestLiveToolStreaming(t *testing.T) {
	chain := newLiveChain(t)
	request := liveToolRequest(chain, &llm.ToolChoice{Mode: llm.ToolChoiceRequired})
	request.Stream = true
	response := liveHTTP(t, chain, request)
	defer response.Body.Close()
	call, done, finish := liveStreamTool(t, response.Body)
	if done != 1 || finish != "tool_calls" {
		t.Fatalf("tool SSE done=%d finish=%s", done, finish)
	}
	output := liveExecuteTool(t, call)
	message := llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{call}}
	followup := llm.ChatCompletionRequest{Model: chain.model, Tools: request.Tools, ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceNone}, Messages: append(append([]llm.Message{}, request.Messages...), message, output)}
	answer := liveDecodeChat(t, liveHTTP(t, chain, followup))
	if !strings.Contains(answer.Choices[0].Message.Content, fmt.Sprint(liveExpectedSum)) {
		t.Fatal("streamed tool continuation failed")
	}
	liveAssertSettlement(t, chain, 2)
}

func liveStreamTool(t *testing.T, body io.Reader) (llm.ToolCall, int, string) {
	t.Helper()
	call := llm.ToolCall{}
	var done int
	finish := ""
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		liveRejectSSEError(t, line)
		if line == "data: [DONE]" {
			done++
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var chunk llm.ChatCompletionChunkResponse
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk); err != nil {
			t.Fatal(err)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				finish = *choice.FinishReason
			}
			call = liveMergeTool(t, call, choice)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return call, done, finish
}

func liveRejectSSEError(t *testing.T, line string) {
	t.Helper()
	if !strings.HasPrefix(line, "data: ") || line == "data: [DONE]" {
		return
	}
	var problem gatewayerrors.GatewayError
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "" {
		t.Fatalf("gateway SSE error_code=%s", problem.Code)
	}
}

func liveMergeTool(t *testing.T, call llm.ToolCall, choice llm.ChunkChoice) llm.ToolCall {
	t.Helper()
	next := call
	for _, delta := range choice.Delta.ToolCalls {
		if delta.ExtraContent != nil {
			next.ExtraContent = llm.CloneToolCallExtraContent(delta.ExtraContent)
		}
		if delta.Index == nil || *delta.Index != 0 {
			t.Fatal("unexpected tool index")
		}
		if delta.ID != nil {
			next.ID = *delta.ID
		}
		if delta.Type != nil {
			next.Type = *delta.Type
		}
		if delta.Function == nil {
			continue
		}
		if delta.Function.Name != nil {
			next.Function.Name += *delta.Function.Name
		}
		if delta.Function.Arguments != nil {
			next.Function.Arguments += *delta.Function.Arguments
		}
	}
	return next
}
