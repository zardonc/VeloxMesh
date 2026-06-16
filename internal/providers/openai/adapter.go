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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	
	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type Adapter struct {
	id      string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
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
	}
}

func (a *Adapter) ID() string {
	return a.id
}

func (a *Adapter) Models() []string {
	return a.models
}

func (a *Adapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	// A basic health check. Ideally, we would do a models list request, but this is fine for Phase 1.
	if a.apiKey == "" {
		return providers.HealthStatus{Available: false, Message: "missing API key"}
	}
	return providers.HealthStatus{Available: true, Message: "configured"}
}

func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	openAIReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}
	if req.Temperature != nil {
		openAIReq["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		openAIReq["max_tokens"] = *req.MaxTokens
	}

	body, err := json.Marshal(openAIReq)
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

	var openAIResp llm.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Malformed JSON from provider", http.StatusBadGateway)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned no choices", http.StatusBadGateway)
	}

	return &llm.LLMResponse{
		GatewayID: req.RequestID,
		Model:     openAIResp.Model,
		Choices:   openAIResp.Choices,
	}, nil
}
