package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type Adapter struct {
	id         string
	baseURL    string
	apiKey     string
	models     []string
	client     *http.Client
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
	// Map to OpenAI request format
	openAIReq := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	var openAIResp llm.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &llm.LLMResponse{
		GatewayID: req.RequestID,
		Model:     openAIResp.Model,
		Choices:   openAIResp.Choices,
	}, nil
}
