package anthropic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"net/http"
	"os"
	"strings"
	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type Adapter struct {
	id                 string
	client             *anthropic.Client
	models             []string
	defaultModel       string
	generateToolCallID func() string
}

type adapterConfig struct {
	id        string
	baseURL   string
	apiKey    string
	modelsStr string
}

func NewAdapter(values ...string) providers.ProviderAdapter {
	config := adapterConfigFrom(values)
	opts := []option.RequestOption{option.WithAPIKey(config.apiKey)}
	if config.baseURL != "" {
		opts = append(opts, option.WithBaseURL(config.baseURL))
	}
	client := anthropic.NewClient(opts...)
	models := configuredModels(config.modelsStr)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}
	return &Adapter{id: config.id, client: &client, models: models, defaultModel: defaultModel, generateToolCallID: opaqueToolCallID}
}

func adapterConfigFrom(values []string) adapterConfig {
	return adapterConfig{
		id:        adapterArgument(values, 0),
		baseURL:   adapterArgument(values, 1),
		apiKey:    adapterArgument(values, 2),
		modelsStr: adapterArgument(values, 3),
	}
}

func adapterArgument(values []string, index int) string {
	if index >= len(values) {
		return ""
	}
	return values[index]
}
func configuredModels(modelsStr string) []string {
	models := make([]string, 0)
	for _, model := range strings.Split(modelsStr, ",") {
		if model = strings.TrimSpace(model); model != "" {
			models = append(models, model)
		}
	}
	return models
}
func opaqueToolCallID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return "call_" + hex.EncodeToString(bytes)
}
func (a *Adapter) ID() string       { return a.id }
func (a *Adapter) Models() []string { return append([]string(nil), a.models...) }
func (a *Adapter) HealthCheck(context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true, Message: "Anthropic native health check not implemented"}
}
func (a *Adapter) Capabilities() providers.CapabilitySet {
	return providers.CapabilitySet{
		ProviderType:        providers.ProviderTypeAnthropic,
		SupportedOperations: []providers.Operation{providers.OperationChatCompletions},
		InputModalities:     []providers.Modality{providers.ModalityText},
		OutputModalities:    []providers.Modality{providers.ModalityText},
		Streaming:           true, ToolCalling: true,
		ToolProtocol: providers.ToolProtocolCapability{Definitions: true, ChoiceModes: map[providers.ToolChoiceCapabilityMode]bool{
			providers.ToolChoiceCapabilityOmitted: true, providers.ToolChoiceCapabilityAuto: true,
			providers.ToolChoiceCapabilityNone: true, providers.ToolChoiceCapabilityRequired: true,
			providers.ToolChoiceCapabilityNamed: true,
		}},
		GenerationParameters: []providers.GenerationParameter{providers.GenerationParameterTemperature, providers.GenerationParameterMaxTokens},
	}
}
func (a *Adapter) buildParams(req *llm.LLMRequest) (anthropic.MessageNewParams, error) {
	messages, system, err := buildAnthropicMessages(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	tools, err := buildAnthropicTools(req.Tools)
	if err != nil {
		return anthropic.MessageNewParams{}, err
	}
	params := anthropic.MessageNewParams{Model: anthropic.Model(a.requestModel(req.Model)), MaxTokens: maxTokens(req), Messages: messages, Tools: tools}
	if len(system) > 0 {
		params.System = system
	}
	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}
	if req.ToolChoice != nil {
		choice, err := anthropicToolChoice(*req.ToolChoice)
		if err != nil {
			return anthropic.MessageNewParams{}, err
		}
		params.ToolChoice = choice
	}
	return params, nil
}
func (a *Adapter) requestModel(model string) string {
	if model != "" {
		return model
	}
	return a.defaultModel
}
func maxTokens(req *llm.LLMRequest) int64 {
	if req.MaxTokens != nil {
		return int64(*req.MaxTokens)
	}
	return 4096
}
func buildAnthropicMessages(messages []llm.Message) ([]anthropic.MessageParam, []anthropic.TextBlockParam, error) {
	result := make([]anthropic.MessageParam, 0, len(messages))
	system := make([]anthropic.TextBlockParam, 0)
	for _, message := range messages {
		switch message.Role {
		case llm.RoleSystem:
			system = append(system, anthropic.TextBlockParam{Text: message.Content})
		case llm.RoleAssistant:
			content, err := assistantContent(message)
			if err != nil {
				return nil, nil, err
			}
			if len(content) > 0 {
				result = append(result, anthropic.NewAssistantMessage(content...))
			}
		case llm.RoleTool:
			if strings.TrimSpace(message.ToolCallID) == "" {
				return nil, nil, invalidRequest()
			}
			result = append(result, anthropic.NewUserMessage(anthropic.NewToolResultBlock(message.ToolCallID, message.Content, false)))
		case llm.RoleUser, "":
			if content := userContent(message); len(content) > 0 {
				result = append(result, anthropic.NewUserMessage(content...))
			}
		}
	}
	return result, system, nil
}
func assistantContent(message llm.Message) ([]anthropic.ContentBlockParamUnion, error) {
	content := make([]anthropic.ContentBlockParamUnion, 0, len(message.ToolCalls)+1)
	if message.Content != "" {
		content = append(content, anthropic.NewTextBlock(message.Content))
	}
	for _, call := range message.ToolCalls {
		input, err := jsonObject(call.Function.Arguments)
		if err != nil || strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Function.Name) == "" {
			return nil, invalidRequest()
		}
		content = append(content, anthropic.NewToolUseBlock(call.ID, input, call.Function.Name))
	}
	return content, nil
}
func userContent(message llm.Message) []anthropic.ContentBlockParamUnion {
	content := make([]anthropic.ContentBlockParamUnion, 0, len(message.MultiContent))
	if len(message.MultiContent) == 0 {
		if message.Content != "" {
			content = append(content, anthropic.NewTextBlock(message.Content))
		}
		return content
	}
	for _, part := range message.MultiContent {
		if part.Type == llm.ContentTypeText && part.Text != "" {
			content = append(content, anthropic.NewTextBlock(part.Text))
		}
		if part.Type == llm.ContentTypeImageURL && part.ImageURL != nil {
			content = appendImageBlock(content, part.ImageURL.URL)
		}
	}
	return content
}
func appendImageBlock(content []anthropic.ContentBlockParamUnion, url string) []anthropic.ContentBlockParamUnion {
	parts := strings.SplitN(url, ";base64,", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "data:image/") {
		return content
	}
	return append(content, anthropic.NewImageBlockBase64(strings.TrimPrefix(parts[0], "data:"), parts[1]))
}
func buildAnthropicTools(tools []llm.Tool) ([]anthropic.ToolUnionParam, error) {
	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
			continue
		}
		schema, err := anthropicInputSchema(tool.Function.Parameters)
		if err != nil || strings.TrimSpace(tool.Function.Name) == "" {
			return nil, invalidRequest()
		}
		parameter := anthropic.ToolParam{Name: tool.Function.Name, InputSchema: schema}
		if tool.Function.Description != "" {
			parameter.Description = anthropic.String(tool.Function.Description)
		}
		result = append(result, anthropic.ToolUnionParam{OfTool: &parameter})
	}
	return result, nil
}
func anthropicInputSchema(raw json.RawMessage) (anthropic.ToolInputSchemaParam, error) {
	if len(raw) == 0 {
		return anthropic.ToolInputSchemaParam{}, invalidRequest()
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil || schema == nil {
		return anthropic.ToolInputSchemaParam{}, invalidRequest()
	}
	properties := schema["properties"]
	required, err := stringSlice(schema["required"])
	if err != nil {
		return anthropic.ToolInputSchemaParam{}, invalidRequest()
	}
	delete(schema, "properties")
	delete(schema, "required")
	delete(schema, "type")
	return anthropic.ToolInputSchemaParam{Properties: properties, Required: required, ExtraFields: schema}, nil
}
func stringSlice(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil, invalidRequest()
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		text, ok := item.(string)
		if !ok {
			return nil, invalidRequest()
		}
		result = append(result, text)
	}
	return result, nil
}
func anthropicToolChoice(choice llm.ToolChoice) (anthropic.ToolChoiceUnionParam, error) {
	switch choice.Mode {
	case llm.ToolChoiceAuto:
		return anthropic.ToolChoiceUnionParam{OfAuto: &anthropic.ToolChoiceAutoParam{}}, nil
	case llm.ToolChoiceNone:
		none := anthropic.NewToolChoiceNoneParam()
		return anthropic.ToolChoiceUnionParam{OfNone: &none}, nil
	case llm.ToolChoiceRequired:
		return anthropic.ToolChoiceUnionParam{OfAny: &anthropic.ToolChoiceAnyParam{}}, nil
	case llm.ToolChoiceNamed:
		if strings.TrimSpace(choice.FunctionName) == "" {
			return anthropic.ToolChoiceUnionParam{}, invalidRequest()
		}
		return anthropic.ToolChoiceParamOfTool(choice.FunctionName), nil
	default:
		return anthropic.ToolChoiceUnionParam{}, invalidRequest()
	}
}
func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}
	response, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, a.mapError(err)
	}
	content, calls, err := a.completeContent(response.Content)
	if err != nil || (content == "" && len(calls) == 0) {
		return nil, providerBadResponse()
	}
	finish, err := normalizedFinishReason(string(response.StopReason), len(calls) > 0)
	if err != nil {
		return nil, providerBadResponse()
	}
	return &llm.LLMResponse{Model: response.Model, Provider: a.id, Choices: []llm.Choice{{Index: 0, Message: llm.Message{Role: llm.RoleAssistant, Content: content, ToolCalls: calls}, FinishReason: finish}}, Usage: usageFromAnthropic(response.Usage)}, nil
}
func (a *Adapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}
	stream := a.client.Messages.NewStreaming(ctx, params)
	events := make(chan llm.StreamEvent)
	go a.runStream(ctx, streamRun{stream: stream, events: events, model: req.Model})
	return events, nil
}
func (a *Adapter) runStream(ctx context.Context, run streamRun) {
	defer close(run.events)
	state := newStreamState(a.generateToolCallID)
	for run.stream.Next() {
		next, output, err := state.apply(run.stream.Current().RawJSON())
		if err != nil {
			a.sendStreamEvent(ctx, run.events, streamError(a.id, run.model))
			return
		}
		state = next
		for _, event := range output {
			event.Provider = a.id
			event.Model = run.model
			if !a.sendStreamEvent(ctx, run.events, event) {
				return
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	if run.stream.Err() != nil || !state.isComplete() {
		a.sendStreamEvent(ctx, run.events, streamError(a.id, run.model))
		return
	}
	a.sendStreamEvent(ctx, run.events, llm.StreamEvent{Done: true, Provider: a.id, Model: run.model})
}
func (a *Adapter) sendStreamEvent(ctx context.Context, events chan<- llm.StreamEvent, event llm.StreamEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
func providerBadResponse() error {
	return gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "invalid provider tool protocol response", http.StatusBadGateway)
}
func invalidRequest() error {
	return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "invalid request for Anthropic", http.StatusBadRequest)
}
func (a *Adapter) mapError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderAuthError, "Anthropic authentication failed", http.StatusBadGateway)
		case http.StatusTooManyRequests:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderRateLimit, "Anthropic rate limit exceeded", http.StatusBadGateway)
		case http.StatusNotFound:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidModel, "Anthropic model not found", http.StatusBadRequest)
		case http.StatusBadRequest:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderInvalidRequest, "Invalid request to Anthropic", http.StatusBadRequest)
		case http.StatusRequestTimeout:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Anthropic request timeout", http.StatusGatewayTimeout)
		default:
			return gatewayErr.NewGatewayError(gatewayErr.ProviderError, "Anthropic API error", http.StatusBadGateway)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return gatewayErr.NewGatewayError(gatewayErr.ProviderTimeout, "Provider request timed out", http.StatusGatewayTimeout)
	}
	return gatewayErr.NewGatewayError(gatewayErr.ProviderUnavailable, "Failed to communicate with Anthropic", http.StatusBadGateway)
}
