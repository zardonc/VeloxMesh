package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
)

type ChatHandler struct {
	service *gateway.Service
}

type chatProxyMessage struct {
	Role       llm.Role        `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []llm.ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type chatProxyRequest struct {
	Model        string             `json:"model"`
	Messages     []chatProxyMessage `json:"messages"`
	Temperature  *float64           `json:"temperature,omitempty"`
	MaxTokens    *int               `json:"max_tokens,omitempty"`
	Stream       bool               `json:"stream,omitempty"`
	Tools        []llm.Tool         `json:"tools,omitempty"`
	ToolChoice   json.RawMessage    `json:"tool_choice,omitempty"`
	Functions    json.RawMessage    `json:"functions,omitempty"`
	FunctionCall json.RawMessage    `json:"function_call,omitempty"`
}

func NewChatHandler(svc *gateway.Service) *ChatHandler {
	return &ChatHandler{service: svc}
}

func (h *ChatHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	req, requirements, gatewayErr := decodeChatRequest(r)
	if gatewayErr != nil {
		sendGatewayError(w, gatewayErr)
		return
	}

	reqID := middleware.GetReqID(r.Context())
	llmReq := newLLMRequest(reqID, r, req, requirements)
	if req.Stream {
		h.streamChatCompletions(w, r, llmReq, reqID, start)
		return
	}

	resp, err := h.service.HandleChatCompletion(r.Context(), llmReq)
	if err != nil {
		sendGatewayError(w, errors.TranslateError(err))
		return
	}
	writeChatCompletionResponse(w, reqID, resp, time.Since(start))
}

func decodeChatRequest(r *http.Request) (llm.ChatCompletionRequest, llm.ToolProtocolRequirements, *errors.GatewayError) {
	var proxyRequest chatProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&proxyRequest); err != nil {
		return llm.ChatCompletionRequest{}, llm.ToolProtocolRequirements{}, errors.NewGatewayError("invalid_request", "Failed to parse JSON body", http.StatusBadRequest)
	}
	if len(proxyRequest.Functions) > 0 || len(proxyRequest.FunctionCall) > 0 {
		return llm.ChatCompletionRequest{}, llm.ToolProtocolRequirements{}, errors.NewInvalidToolProtocolRequest()
	}
	choice, gatewayErr := llm.ParseToolChoice(proxyRequest.ToolChoice)
	if gatewayErr != nil {
		return llm.ChatCompletionRequest{}, llm.ToolProtocolRequirements{}, gatewayErr
	}
	messages, gatewayErr := decodeChatMessages(proxyRequest.Messages)
	if gatewayErr != nil {
		return llm.ChatCompletionRequest{}, llm.ToolProtocolRequirements{}, gatewayErr
	}
	if len(messages) == 0 {
		return llm.ChatCompletionRequest{}, llm.ToolProtocolRequirements{}, errors.NewGatewayError("invalid_request", "Messages array is required", http.StatusBadRequest)
	}
	return llm.NormalizeToolProtocol(llm.ChatCompletionRequest{
		Model: proxyRequest.Model, Messages: messages, Temperature: proxyRequest.Temperature,
		MaxTokens: proxyRequest.MaxTokens, Stream: proxyRequest.Stream, Tools: proxyRequest.Tools, ToolChoice: choice,
	})
}

func newLLMRequest(requestID string, r *http.Request, request llm.ChatCompletionRequest, requirements llm.ToolProtocolRequirements) *llm.LLMRequest {
	return &llm.LLMRequest{
		RequestID: requestID, Model: request.Model, Messages: request.Messages,
		Temperature: request.Temperature, MaxTokens: request.MaxTokens, Stream: request.Stream,
		PriorityClass: r.Header.Get("X-Priority"), RouteOverride: r.Header.Get("X-Route-To"),
		Tools: request.Tools, ToolChoice: request.ToolChoice, ToolRequirements: requirements,
	}
}

func writeChatCompletionResponse(w http.ResponseWriter, requestID string, response *llm.LLMResponse, duration time.Duration) {
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Provider", response.Provider)
	w.Header().Set("X-Model", response.Model)
	w.Header().Set("X-Latency-E2E-Ms", fmt.Sprintf("%d", duration.Milliseconds()))
	w.Header().Set("X-Queue-Wait-Ms", fmt.Sprintf("%d", response.QueueWaitMs))
	w.Header().Set("X-Cache-Hit", fmt.Sprintf("%t", response.CacheHit))
	if response.CacheHit {
		w.Header().Set("X-Cache-Level", response.CacheLevel)
	} else {
		w.Header().Set("X-Cache-Level", "none")
	}
	if response.Strategy != "" {
		w.Header().Set("X-Routing-Strategy", response.Strategy)
	}
	if response.AttemptCount > 0 {
		w.Header().Set("X-Provider-Attempts", fmt.Sprintf("%d", response.AttemptCount))
	}
	w.Header().Set("X-Fallback-Used", fmt.Sprintf("%t", response.FallbackUsed))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(llm.ChatCompletionResponse{
		ID: requestID, Object: "chat.completion", Created: time.Now().Unix(),
		Model: response.Model, Choices: response.Choices, Usage: response.Usage,
	})
}
func decodeChatMessages(proxyMessages []chatProxyMessage) ([]llm.Message, *errors.GatewayError) {
	messages := make([]llm.Message, 0, len(proxyMessages))
	for _, proxyMessage := range proxyMessages {
		content, multiContent, err := decodeChatContent(proxyMessage.Content)
		if err != nil {
			return nil, err
		}
		messages = append(messages, llm.Message{
			Role: proxyMessage.Role, Content: content, MultiContent: multiContent,
			ToolCalls: proxyMessage.ToolCalls, ToolCallID: proxyMessage.ToolCallID,
		})
	}
	return messages, nil
}

func decodeChatContent(raw json.RawMessage) (string, []llm.ContentPart, *errors.GatewayError) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "", nil, nil
	}
	if value[0] == '"' {
		var content string
		if err := json.Unmarshal(value, &content); err == nil {
			return content, nil, nil
		}
		return "", nil, errors.NewInvalidToolProtocolRequest()
	}
	if value[0] == '[' {
		var content []llm.ContentPart
		if err := json.Unmarshal(value, &content); err == nil {
			return "", content, nil
		}
	}
	return "", nil, errors.NewInvalidToolProtocolRequest()
}

func sendError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errors.NewGatewayError(code, message, status))
}

func sendGatewayError(w http.ResponseWriter, gwErr *errors.GatewayError) {
	w.Header().Set("Content-Type", "application/json")
	if gwErr.Headers != nil {
		for k, v := range gwErr.Headers {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(gwErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(gwErr)
}
