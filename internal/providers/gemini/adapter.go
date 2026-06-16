package gemini

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"google.golang.org/genai"
	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type Adapter struct {
	id           string
	client       *genai.Client
	models       []string
	defaultModel string
}

func NewAdapter(id, baseURL, apiKey, modelsStr string) providers.ProviderAdapter {
	config := &genai.ClientConfig{
		APIKey: apiKey,
	}
	if baseURL != "" {
		config.HTTPOptions = genai.HTTPOptions{
			BaseURL: baseURL,
		}
	}
	client, _ := genai.NewClient(context.Background(), config)

	var models []string
	for _, m := range strings.Split(modelsStr, ",") {
		m = strings.TrimSpace(m)
		if m != "" {
			models = append(models, m)
		}
	}

	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}

	return &Adapter{
		id:           id,
		client:       client,
		models:       models,
		defaultModel: defaultModel,
	}
}

func (a *Adapter) ID() string {
	return a.id
}

func (a *Adapter) Models() []string {
	return a.models
}

func (a *Adapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true, Message: "Gemini native health check not implemented"}
}

func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}

	var contents []*genai.Content
	var systemInstruction *genai.Content

	for _, msg := range req.Messages {
		if msg.Role == llm.RoleSystem {
			systemInstruction = &genai.Content{
				Role: "system",
				Parts: []*genai.Part{
					{Text: msg.Content},
				},
			}
			continue
		}

		role := "user"
		if msg.Role == llm.RoleAssistant {
			role = "model"
		}

		contents = append(contents, &genai.Content{
			Role: role,
			Parts: []*genai.Part{
				{Text: msg.Content},
			},
		})
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
	}
	if req.Temperature != nil {
		f := float32(*req.Temperature)
		config.Temperature = &f
	}
	if req.MaxTokens != nil {
		config.MaxOutputTokens = int32(*req.MaxTokens)
	}

	resp, err := a.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, a.mapError(err)
	}

	if len(resp.Candidates) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "No candidates returned from Gemini", http.StatusBadGateway)
	}

	candidate := resp.Candidates[0]
	content := ""
	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				content += part.Text
			}
		}
	}
	
	if content == "" && candidate.FinishReason != "SAFETY" && candidate.FinishReason != "RECITATION" && candidate.FinishReason != "OTHER" {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned no text content", http.StatusBadGateway)
	}

	finishReason := "stop"
	if candidate.FinishReason != "" {
		switch candidate.FinishReason {
		case "STOP":
			finishReason = "stop"
		case "MAX_TOKENS":
			finishReason = "length"
		case "SAFETY", "RECITATION", "OTHER":
			finishReason = strings.ToLower(string(candidate.FinishReason))
		}
	}

	return &llm.LLMResponse{
		Model:    model,
		Provider: a.id,
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: content,
				},
				FinishReason: finishReason,
			},
		},
	}, nil
}

func (a *Adapter) mapError(err error) error {
	var apiErr *genai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case http.StatusUnauthorized, http.StatusForbidden:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderAuthError, "Gemini authentication failed", http.StatusBadGateway)
		case http.StatusTooManyRequests:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderRateLimit, "Gemini rate limit exceeded", http.StatusBadGateway)
		case http.StatusNotFound:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Gemini model not found", http.StatusBadRequest)
		case http.StatusBadRequest:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "Invalid request to Gemini", http.StatusBadRequest)
		case http.StatusRequestTimeout:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Gemini request timeout", http.StatusGatewayTimeout)
		default:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderError, "Gemini API error", http.StatusBadGateway)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
	}
	return gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Failed to communicate with Gemini", http.StatusBadGateway)
}
