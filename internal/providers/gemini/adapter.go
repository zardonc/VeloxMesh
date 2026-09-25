package gemini

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"os"
	"strings"

	"google.golang.org/genai"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/toolstream"
)

type Adapter struct {
	id                 string
	client             *genai.Client
	models             []string
	defaultModel       string
	generateToolCallID func() string
}

type AdapterConfig struct {
	ID        string
	BaseURL   string
	APIKey    string
	ModelsCSV string
}

func NewAdapter(config AdapterConfig) providers.ProviderAdapter {
	clientConfig := &genai.ClientConfig{APIKey: config.APIKey, Backend: genai.BackendGeminiAPI}
	if config.BaseURL != "" {
		clientConfig.HTTPOptions = genai.HTTPOptions{BaseURL: config.BaseURL}
	}
	client, _ := genai.NewClient(context.Background(), clientConfig)
	models := configuredModels(config.ModelsCSV)
	return &Adapter{
		id:                 config.ID,
		client:             client,
		models:             models,
		defaultModel:       firstModel(models),
		generateToolCallID: opaqueToolCallID,
	}
}

func configuredModels(modelsCSV string) []string {
	models := make([]string, 0)
	for _, model := range strings.Split(modelsCSV, ",") {
		if model = strings.TrimSpace(model); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func firstModel(models []string) string {
	if len(models) == 0 {
		return ""
	}
	return models[0]
}

func (a *Adapter) ID() string { return a.id }

func (a *Adapter) Models() []string { return append([]string(nil), a.models...) }

func (a *Adapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeGemini,
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
		GenerationParameters: []providers.GenerationParameter{
			providers.GenerationParameterTemperature,
			providers.GenerationParameterMaxTokens,
		},
	}
}

func (a *Adapter) HealthCheck(context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true, Message: "Gemini native health check not implemented"}
}

func (a *Adapter) buildParams(req *llm.LLMRequest) (string, []*genai.Content, *genai.GenerateContentConfig, error) {
	contents, system, err := geminiContents(req.Messages)
	if err != nil {
		return "", nil, nil, err
	}
	functions := geminiFunctionDeclarations(req.Tools)
	toolConfig, err := geminiToolConfig(req.ToolChoice, functions)
	if err != nil {
		return "", nil, nil, err
	}
	config := geminiGenerateConfig(req, system, toolConfig, functions)
	return a.requestModel(req.Model), contents, config, nil
}

func geminiGenerateConfig(req *llm.LLMRequest, system *genai.Content, toolConfig *genai.ToolConfig, functions []*genai.FunctionDeclaration) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{SystemInstruction: system, ToolConfig: toolConfig}
	if len(functions) > 0 {
		config.Tools = []*genai.Tool{{FunctionDeclarations: functions}}
	}
	if req.Temperature != nil {
		temperature := float32(*req.Temperature)
		config.Temperature = &temperature
	}
	if req.MaxTokens != nil {
		config.MaxOutputTokens = int32(*req.MaxTokens)
	}
	return config
}

func (a *Adapter) requestModel(model string) string {
	if model != "" {
		return model
	}
	return a.defaultModel
}

func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	model, contents, config, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}
	response, err := a.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, a.mapError(err)
	}
	if len(response.Candidates) == 0 {
		return nil, invalidGeminiToolResponse()
	}
	choice, err := geminiChoice(response.Candidates[geminiFirstCandidateIndex], a.generateToolCallID)
	if err != nil {
		return nil, err
	}
	return &llm.LLMResponse{
		Model:    model,
		Provider: a.id,
		Choices:  []llm.Choice{choice},
		Usage:    usageFromGemini(response.UsageMetadata),
	}, nil
}

func (a *Adapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	model, contents, config, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}
	stream := a.client.Models.GenerateContentStream(ctx, model, contents, config)
	output := make(chan llm.StreamEvent)
	go a.forwardStream(stream, output)
	return output, nil
}

func (a *Adapter) forwardStream(stream iter.Seq2[*genai.GenerateContentResponse, error], output chan<- llm.StreamEvent) {
	defer close(output)
	state := geminiStreamState{tools: toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID})}
	for response, err := range stream {
		if err != nil {
			output <- llm.StreamEvent{Error: a.mapError(err)}
			return
		}
		next, events, eventErr := geminiStreamEvents(state, response)
		if eventErr != nil {
			output <- llm.StreamEvent{Error: invalidGeminiToolResponse()}
			return
		}
		state = next
		for _, event := range events {
			output <- event
		}
	}
	if state.toolCount > 0 && !state.terminal {
		output <- llm.StreamEvent{Error: invalidGeminiToolResponse()}
		return
	}
	output <- llm.StreamEvent{Done: true}
}

func (a *Adapter) mapError(err error) error {
	if code, found := geminiAPIStatus(err); found {
		return geminiStatusError(code)
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
	}
	return gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Failed to communicate with Gemini", http.StatusBadGateway)
}

func geminiAPIStatus(err error) (int, bool) {
	var pointer *genai.APIError
	if errors.As(err, &pointer) {
		return pointer.Code, true
	}
	var value genai.APIError
	if errors.As(err, &value) {
		return value.Code, true
	}
	return 0, false
}

func geminiStatusError(code int) error {
	switch code {
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
