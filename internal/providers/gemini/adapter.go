package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	models := make([]string, len(a.models))
	copy(models, a.models)
	return models
}

func (a *Adapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:         providers.ProviderTypeGemini,
		SupportedOperations:  []providers.Operation{providers.OperationChatCompletions},
		InputModalities:      []providers.Modality{providers.ModalityText},
		OutputModalities:     []providers.Modality{providers.ModalityText},
		Streaming:            false,
		ToolCalling:          true,
		GenerationParameters: []providers.GenerationParameter{providers.GenerationParameterTemperature, providers.GenerationParameterMaxTokens},
	}
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

		var parts []*genai.Part
		if len(msg.MultiContent) > 0 {
			for _, p := range msg.MultiContent {
				if p.Type == llm.ContentTypeText {
					parts = append(parts, &genai.Part{Text: p.Text})
				} else if p.Type == llm.ContentTypeImageURL && p.ImageURL != nil {
					if strings.HasPrefix(p.ImageURL.URL, "data:image/") {
						arr := strings.SplitN(p.ImageURL.URL, ";base64,", 2)
						if len(arr) == 2 {
							mimeType := strings.TrimPrefix(arr[0], "data:")
							dataBytes, _ := base64.StdEncoding.DecodeString(arr[1])
							parts = append(parts, &genai.Part{
								InlineData: &genai.Blob{
									MIMEType: mimeType,
									Data:     dataBytes,
								},
							})
						}
					}
				}
			}
		} else {
			parts = append(parts, &genai.Part{Text: msg.Content})
		}

		contents = append(contents, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}

	var funcs []*genai.FunctionDeclaration
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			if t.Type == llm.ToolTypeFunction && t.Function != nil {
				funcs = append(funcs, &genai.FunctionDeclaration{
					Name:                 t.Function.Name,
					Description:          t.Function.Description,
					ParametersJsonSchema: t.Function.Parameters,
				})
			}
		}
	}

	config := &genai.GenerateContentConfig{
		SystemInstruction: systemInstruction,
	}
	if len(funcs) > 0 {
		config.Tools = []*genai.Tool{
			{FunctionDeclarations: funcs},
		}
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
	var toolCalls []llm.ToolCall

	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				content += part.Text
			}
			if part.FunctionCall != nil {
				argsBytes, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:   fmt.Sprintf("call_%d", len(toolCalls)+1),
					Type: llm.ToolTypeFunction,
					Function: llm.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsBytes),
					},
				})
			}
		}
	}

	if content == "" && len(toolCalls) == 0 && candidate.FinishReason != "SAFETY" && candidate.FinishReason != "RECITATION" && candidate.FinishReason != "OTHER" {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned no valid output", http.StatusBadGateway)
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
					Role:      llm.RoleAssistant,
					Content:   content,
					ToolCalls: toolCalls,
				},
				FinishReason: finishReason,
			},
		},
	}, nil
}

func (a *Adapter) mapError(err error) error {
	var apiErr *genai.APIError
	var apiErrVal genai.APIError
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
	} else if errors.As(err, &apiErrVal) {
		switch apiErrVal.Code {
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
