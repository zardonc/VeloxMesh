package handlers

import (
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

func NewChatHandler(svc *gateway.Service) *ChatHandler {
	return &ChatHandler{service: svc}
}

func (h *ChatHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req llm.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, "invalid_request", "Failed to parse JSON body", http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		sendError(w, "invalid_request", "Messages array is required", http.StatusBadRequest)
		return
	}

	if req.Stream {
		sendError(w, "not_supported", "Streaming is not yet supported in Phase 1", http.StatusBadRequest)
		return
	}

	for _, msg := range req.Messages {
		if msg.Role != llm.RoleSystem && msg.Role != llm.RoleUser && msg.Role != llm.RoleAssistant {
			sendError(w, "invalid_request", "Invalid message role", http.StatusBadRequest)
			return
		}
	}

	reqID := middleware.GetReqID(r.Context())
	priority := r.Header.Get("X-Priority")
	routeOverride := r.Header.Get("X-Route-To")

	llmReq := &llm.LLMRequest{
		RequestID:     reqID,
		Model:         req.Model,
		Messages:      req.Messages,
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		Stream:        req.Stream,
		PriorityClass: priority,
		RouteOverride: routeOverride,
	}

	resp, err := h.service.HandleChatCompletion(r.Context(), llmReq)
	if err != nil {
		if gwErr, ok := err.(*errors.GatewayError); ok {
			sendError(w, gwErr.Code, gwErr.Message, gwErr.HTTPStatus)
		} else {
			sendError(w, "provider_error", fmt.Sprintf("Upstream error: %v", err), http.StatusBadGateway)
		}
		return
	}

	duration := time.Since(start)

	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("X-Provider", resp.Provider)
	w.Header().Set("X-Model", resp.Model)
	w.Header().Set("X-Cache-Hit", "false")
	w.Header().Set("X-Cache-Level", "none")
	w.Header().Set("X-Latency-E2E-Ms", fmt.Sprintf("%d", duration.Milliseconds()))
	w.Header().Set("X-Queue-Wait-Ms", "0")
	if resp.Strategy != "" {
		w.Header().Set("X-Routing-Strategy", resp.Strategy)
	}
	if resp.AttemptCount > 0 {
		w.Header().Set("X-Provider-Attempts", fmt.Sprintf("%d", resp.AttemptCount))
	}
	if resp.FallbackUsed {
		w.Header().Set("X-Fallback-Used", "true")
	} else {
		w.Header().Set("X-Fallback-Used", "false")
	}

	openAIResp := llm.ChatCompletionResponse{
		ID:      reqID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: resp.Choices,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(openAIResp)
}

func sendError(w http.ResponseWriter, code, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errors.NewGatewayError(code, message, status))
}
