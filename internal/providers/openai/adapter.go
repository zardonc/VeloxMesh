// Package openai implements the OpenAI-compatible provider adapter.
//
// Decision Record (Phase 2):
// We reviewed the official OpenAI Go SDK for request shape, auth header, error
// mapping, and response model assumptions. However, we decided to keep this
// minimal local `net/http` adapter instead of importing the full official SDK.
// The official SDK would hide transport details that we want to observe and
// add unnecessary abstraction for the hot-path (we only need /chat/completions
// and a simple JSON request/response format). The minimal adapter approach
// is sufficient for our current multi-provider routing needs.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/toolstream"
)

type Adapter struct {
	id                 string
	baseURL            string
	apiKey             string
	models             []string
	client             *http.Client
	generateToolCallID func() string
}

func NewAdapter(id, baseURL, apiKey, modelsCSV string) *Adapter {
	modelList := strings.Split(modelsCSV, ",")
	for i := range modelList {
		modelList[i] = strings.TrimSpace(modelList[i])
	}
	return &Adapter{
		id:      id,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		models:  modelList,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		generateToolCallID: func() string { return "call_" + uuid.NewString() },
	}
}

func (a *Adapter) ID() string {
	return a.id
}

func (a *Adapter) Models() []string {
	models := make([]string, len(a.models))
	copy(models, a.models)
	return models
}

func (a *Adapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeOpenAICompatible,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
		Streaming:           true,
		ToolCalling:         true,
		ToolProtocol: providers.ToolProtocolCapability{
			Definitions:        true,
			AssistantToolCalls: true,
			ToolResults:        true,
			StreamingDeltas:    true,
			ChoiceModes: map[providers.ToolChoiceCapabilityMode]bool{
				providers.ToolChoiceCapabilityOmitted:  true,
				providers.ToolChoiceCapabilityAuto:     true,
				providers.ToolChoiceCapabilityNone:     true,
				providers.ToolChoiceCapabilityRequired: true,
				providers.ToolChoiceCapabilityNamed:    true,
			},
		},
		GenerationParameters: []providers.GenerationParameter{providers.GenerationParameterTemperature, providers.GenerationParameterMaxTokens},
	}
}

type chatRequestPayload struct {
	Model       string           `json:"model"`
	Messages    []map[string]any `json:"messages"`
	Stream      bool             `json:"stream,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	Tools       []llm.Tool       `json:"tools,omitempty"`
	ToolChoice  *llm.ToolChoice  `json:"tool_choice,omitempty"`
}

func newChatRequestPayload(req *llm.LLMRequest, stream bool) chatRequestPayload {
	payload := chatRequestPayload{
		Model: req.Model, Messages: mapMessages(req.Messages), Stream: stream,
		Temperature: req.Temperature, MaxTokens: req.MaxTokens, Tools: req.Tools,
	}
	if req.ToolChoice != nil {
		choice := *req.ToolChoice
		payload.ToolChoice = &choice
	}
	return payload
}

func mapMessages(msgs []llm.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, message := range msgs {
		mapped := map[string]any{"role": message.Role}
		if len(message.ToolCalls) > 0 {
			mapped["tool_calls"] = message.ToolCalls
		}
		if len(message.MultiContent) > 0 {
			mapped["content"] = message.MultiContent
		} else {
			mapped["content"] = message.Content
		}
		if message.ToolCallID != "" {
			mapped["tool_call_id"] = message.ToolCallID
		}
		out = append(out, mapped)
	}
	return out
}

func (a *Adapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	// A basic health check. Ideally, we would do a models list request, but this is fine for Phase 1.
	if a.apiKey == "" {
		return providers.HealthStatus{Available: false, Message: "missing API key"}
	}
	return providers.HealthStatus{Available: true, Message: "configured"}
}

func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	body, err := json.Marshal(newChatRequestPayload(req, false))
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "failed to marshal request", http.StatusBadRequest)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderError, "failed to create request", http.StatusInternalServerError)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
		}
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Provider unavailable", http.StatusBadGateway)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, mapChatError(resp)
	}

	var openAIResp llm.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Malformed JSON from provider", http.StatusBadGateway)
	}

	choices, err := a.normalizeCompleteChoices(openAIResp.Choices)
	if err != nil {
		return nil, err
	}

	return &llm.LLMResponse{
		GatewayID: req.RequestID,
		Model:     openAIResp.Model,
		Provider:  a.id,
		Choices:   choices,
		Usage:     openAIResp.Usage,
	}, nil
}

type streamChunk struct {
	Model   string        `json:"model"`
	Choices []chunkChoice `json:"choices"`
	Usage   *llm.Usage    `json:"usage,omitempty"`
}

type chunkChoice struct {
	Delta struct {
		Content   string              `json:"content"`
		ToolCalls []llm.ToolCallChunk `json:"tool_calls,omitempty"`
	} `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

func (a *Adapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	body, err := json.Marshal(newChatRequestPayload(req, true))
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "failed to marshal request", http.StatusBadRequest)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderError, "failed to create request", http.StatusInternalServerError)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
		}
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Provider unavailable", http.StatusBadGateway)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, mapChatError(resp)
	}

	ch := make(chan llm.StreamEvent)
	go a.readStream(ctx, req, resp, ch)
	return ch, nil
}

type streamReadState struct {
	protocol    toolstream.State
	indexes     map[int]struct{}
	sawEvent    bool
	sawToolCall bool
	terminal    bool
}

func (a *Adapter) readStream(ctx context.Context, req *llm.LLMRequest, resp *http.Response, ch chan<- llm.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = resp.Body.Close() })
	defer stopClose()
	state := streamReadState{protocol: toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID}), indexes: map[int]struct{}{}}
	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadBytes('\n')
		data := strings.TrimSpace(string(line))
		if strings.HasPrefix(data, "data:") && !a.handleStreamData(ctx, ch, &state, strings.TrimSpace(strings.TrimPrefix(data, "data:")), req.Model) {
			return
		}
		if readErr == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if state.sawToolCall || !state.sawEvent || !errors.Is(readErr, io.EOF) {
			a.sendStreamError(ctx, ch)
			return
		}
		sendStreamEvent(ctx, ch, llm.StreamEvent{Done: true, Provider: a.id, Model: req.Model})
		return
	}
}

func (a *Adapter) handleStreamData(ctx context.Context, ch chan<- llm.StreamEvent, state *streamReadState, data, model string) bool {
	if data == "[DONE]" {
		if state.sawToolCall && !state.terminal {
			a.sendStreamError(ctx, ch)
			return false
		}
		sendStreamEvent(ctx, ch, llm.StreamEvent{Done: true, Provider: a.id, Model: model})
		return false
	}
	if state.terminal {
		a.sendStreamError(ctx, ch)
		return false
	}
	var chunk streamChunk
	if json.Unmarshal([]byte(data), &chunk) != nil {
		a.sendStreamError(ctx, ch)
		return false
	}
	event, next, indexes, hasToolCall, err := a.normalizeStreamChunk(state.protocol, chunk, model)
	if err != nil || !a.completeStreamEvent(state, event, next, indexes, hasToolCall) {
		a.sendStreamError(ctx, ch)
		return false
	}
	if (event.DeltaContent != "" || event.FinishReason != "" || event.Usage != nil || len(event.ToolCalls) > 0) && !sendStreamEvent(ctx, ch, event) {
		return false
	}
	state.sawEvent = true
	return true
}

func (a *Adapter) completeStreamEvent(state *streamReadState, event llm.StreamEvent, next toolstream.State, indexes map[int]struct{}, hasToolCall bool) bool {
	state.protocol = next
	for index := range indexes {
		state.indexes[index] = struct{}{}
	}
	state.sawToolCall = state.sawToolCall || hasToolCall
	if event.FinishReason == "" {
		return true
	}
	if !state.sawToolCall {
		state.terminal = event.FinishReason != "tool_calls"
		return state.terminal
	}
	if event.FinishReason != "tool_calls" {
		return false
	}
	completed, err := finishToolStream(state.protocol, state.indexes)
	if err != nil {
		return false
	}
	state.protocol, state.terminal = completed, true
	return true
}

func (a *Adapter) normalizeStreamChunk(state toolstream.State, chunk streamChunk, fallbackModel string) (llm.StreamEvent, toolstream.State, map[int]struct{}, bool, error) {
	event := llm.StreamEvent{Provider: a.id, Model: chunk.Model, Usage: chunk.Usage}
	if event.Model == "" {
		event.Model = fallbackModel
	}
	indexes := map[int]struct{}{}
	if len(chunk.Choices) == 0 {
		return event, state, indexes, false, nil
	}
	choice := chunk.Choices[0]
	event.DeltaContent = choice.Delta.Content
	if choice.FinishReason != nil {
		event.FinishReason = *choice.FinishReason
	}
	if len(choice.Delta.ToolCalls) == 0 {
		return event, state, indexes, false, nil
	}
	next := state
	chunks := make([]llm.ToolCallChunk, 0, len(choice.Delta.ToolCalls))
	for _, fragment := range choice.Delta.ToolCalls {
		updated, normalized, err := next.Apply(fragment)
		if err != nil || normalized.Index == nil {
			return llm.StreamEvent{}, state, indexes, false, providerToolError()
		}
		next = updated
		indexes[*normalized.Index] = struct{}{}
		chunks = append(chunks, normalized)
	}
	event.ToolCalls = chunks
	return event, next, indexes, true, nil
}

func finishToolStream(state toolstream.State, indexes map[int]struct{}) (toolstream.State, error) {
	next := state
	for index := range indexes {
		updated, err := next.CompleteCall(index)
		if err != nil {
			return state, providerToolError()
		}
		next = updated
	}
	finished, _, err := next.Finish("tool_calls")
	if err != nil {
		return state, providerToolError()
	}
	return finished, nil
}

func sendStreamEvent(ctx context.Context, ch chan<- llm.StreamEvent, event llm.StreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *Adapter) sendStreamError(ctx context.Context, ch chan<- llm.StreamEvent) {
	sendStreamEvent(ctx, ch, llm.StreamEvent{Error: providerToolError(), Provider: a.id})
}

func (a *Adapter) normalizeCompleteChoices(choices []llm.Choice) ([]llm.Choice, error) {
	if len(choices) == 0 {
		return nil, providerToolError()
	}
	normalized := make([]llm.Choice, 0, len(choices))
	for _, choice := range choices {
		updated, err := a.normalizeCompleteChoice(choice)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, updated)
	}
	return normalized, nil
}

func (a *Adapter) normalizeCompleteChoice(choice llm.Choice) (llm.Choice, error) {
	if len(choice.Message.ToolCalls) == 0 {
		if choice.FinishReason == "tool_calls" {
			return llm.Choice{}, providerToolError()
		}
		return choice, nil
	}
	if choice.FinishReason != "tool_calls" {
		return llm.Choice{}, providerToolError()
	}
	state := toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID})
	for index, call := range choice.Message.ToolCalls {
		chunk := completeToolChunk(index, call)
		updated, _, err := state.Apply(chunk)
		if err != nil {
			return llm.Choice{}, providerToolError()
		}
		state = updated
		state, err = state.CompleteCall(index)
		if err != nil {
			return llm.Choice{}, providerToolError()
		}
	}
	_, completion, err := state.Finish("tool_calls")
	if err != nil {
		return llm.Choice{}, providerToolError()
	}
	choice.Message.ToolCalls = completion.Calls
	choice.FinishReason = completion.FinishReason
	return choice, nil
}

func completeToolChunk(index int, call llm.ToolCall) llm.ToolCallChunk {
	indexCopy := index
	typeCopy := call.Type
	name := call.Function.Name
	arguments := call.Function.Arguments
	chunk := llm.ToolCallChunk{Index: &indexCopy, Type: &typeCopy, Function: &llm.FunctionCallChunk{Name: &name, Arguments: &arguments}}
	if call.ID != "" {
		id := call.ID
		chunk.ID = &id
	}
	return chunk
}

func mapChatError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	bodyStr := strings.ToLower(string(bodyBytes))
	isModelInvalid := resp.StatusCode == http.StatusNotFound || strings.Contains(bodyStr, "model")
	switch resp.StatusCode {
	case http.StatusBadRequest:
		if isModelInvalid {
			return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Invalid model requested", http.StatusBadRequest)
		}
		return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "Invalid request to provider", http.StatusBadRequest)
	case http.StatusUnauthorized, http.StatusForbidden:
		return gatewayErr.NewGatewayError(gatewayErr.ProviderAuthError, "Provider authentication failed", http.StatusBadGateway)
	case http.StatusNotFound:
		return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Invalid model requested", http.StatusBadRequest)
	case http.StatusRequestTimeout:
		return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
	case http.StatusTooManyRequests:
		return gatewayErr.NewGatewayError(gatewayErr.ProviderRateLimit, "Provider rate limit exceeded", http.StatusBadGateway)
	default:
		return gatewayErr.NewGatewayError(gatewayErr.ProviderError, "Provider returned error", http.StatusBadGateway)
	}
}

func providerToolError() error {
	return gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "invalid tool protocol response from provider", http.StatusBadGateway)
}

func (a *Adapter) Embed(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	openAIReq := map[string]interface{}{
		"model": req.Model,
		"input": req.Input,
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "failed to marshal request", http.StatusBadRequest)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderError, "failed to create request", http.StatusInternalServerError)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
		}
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Provider unavailable", http.StatusBadGateway)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		bodyStr := strings.ToLower(string(bodyBytes))
		isModelInvalid := resp.StatusCode == http.StatusNotFound || strings.Contains(bodyStr, "model")

		switch resp.StatusCode {
		case http.StatusBadRequest:
			if isModelInvalid {
				return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Invalid model requested", http.StatusBadRequest)
			}
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "Invalid request to provider", http.StatusBadRequest)
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderAuthError, "Provider authentication failed", http.StatusBadGateway)
		case http.StatusNotFound:
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Invalid model requested", http.StatusBadRequest)
		case http.StatusRequestTimeout:
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
		case http.StatusTooManyRequests:
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderRateLimit, "Provider rate limit exceeded", http.StatusBadGateway)
		default:
			return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderError, "Provider returned error", http.StatusBadGateway)
		}
	}

	var openAIResp llm.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Malformed JSON from provider", http.StatusBadGateway)
	}

	if len(openAIResp.Data) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned no embeddings", http.StatusBadGateway)
	}

	return &openAIResp, nil
}
