package anthropic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"net/http"
	"os"
	"strings"
	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/toolstream"
)

const maxToolArgumentBytes = 1024 * 1024

type Adapter struct {
	id                 string
	client             *anthropic.Client
	models             []string
	defaultModel       string
	generateToolCallID func() string
}
type streamState struct {
	tools        toolstream.State
	toolIndexes  map[int]struct{}
	closed       map[int]struct{}
	usage        llm.Usage
	started      bool
	stopped      bool
	finishReason string
}

func NewAdapter(id, baseURL, apiKey, modelsStr string) providers.ProviderAdapter {
	opts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)
	models := configuredModels(modelsStr)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}
	return &Adapter{id: id, client: &client, models: models, defaultModel: defaultModel, generateToolCallID: opaqueToolCallID}
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
func (a *Adapter) completeContent(blocks []anthropic.ContentBlockUnion) (string, []llm.ToolCall, error) {
	state := toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID, MaxArgumentBytes: maxToolArgumentBytes})
	var content strings.Builder
	toolIndex := 0
	for _, block := range blocks {
		decoded, err := completeBlock(block)
		if err != nil {
			return "", nil, err
		}
		switch decoded["type"] {
		case "text":
			text, ok := decoded["text"].(string)
			if !ok {
				return "", nil, providerBadResponse()
			}
			content.WriteString(text)
		case "tool_use":
			chunk, err := completeToolChunk(toolIndex, decoded)
			if err != nil {
				return "", nil, err
			}
			state, _, err = state.Apply(chunk)
			if err != nil {
				return "", nil, err
			}
			state, err = state.CompleteCall(toolIndex)
			if err != nil {
				return "", nil, err
			}
			toolIndex++
		}
	}
	if toolIndex == 0 {
		return content.String(), nil, nil
	}
	_, completion, err := state.Finish("tool_calls")
	if err != nil || !toolArgumentsAreObjects(completion.Calls) {
		return "", nil, providerBadResponse()
	}
	return content.String(), completion.Calls, nil
}
func completeBlock(block anthropic.ContentBlockUnion) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(block.RawJSON()), &decoded); err != nil || decoded == nil {
		return nil, providerBadResponse()
	}
	return decoded, nil
}
func completeToolChunk(index int, block map[string]any) (llm.ToolCallChunk, error) {
	input, inputOK := block["input"].(map[string]any)
	name, nameOK := block["name"].(string)
	if !inputOK || !nameOK || strings.TrimSpace(name) == "" {
		return llm.ToolCallChunk{}, providerBadResponse()
	}
	arguments, err := json.Marshal(input)
	if err != nil {
		return llm.ToolCallChunk{}, providerBadResponse()
	}
	id, hasID := block["id"].(string)
	if _, exists := block["id"]; exists && (!hasID || strings.TrimSpace(id) == "") {
		return llm.ToolCallChunk{}, providerBadResponse()
	}
	return toolChunk(index, optionalString(id, hasID), &name, string(arguments)), nil
}
func toolChunk(index int, id *string, name *string, arguments string) llm.ToolCallChunk {
	toolType := llm.ToolTypeFunction
	return llm.ToolCallChunk{Index: &index, ID: id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: name, Arguments: &arguments}}
}
func optionalString(value string, present bool) *string {
	if !present {
		return nil
	}
	copy := value
	return &copy
}
func jsonObject(raw string) (map[string]any, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return nil, invalidRequest()
	}
	return value, nil
}
func toolArgumentsAreObjects(calls []llm.ToolCall) bool {
	for _, call := range calls {
		if _, err := jsonObject(call.Function.Arguments); err != nil {
			return false
		}
	}
	return true
}
func normalizedFinishReason(reason string, hasTools bool) (string, error) {
	switch reason {
	case "end_turn", "stop_sequence":
		if hasTools {
			return "", providerBadResponse()
		}
		return "stop", nil
	case "max_tokens":
		if hasTools {
			return "", providerBadResponse()
		}
		return "length", nil
	case "tool_use":
		if !hasTools {
			return "", providerBadResponse()
		}
		return "tool_calls", nil
	default:
		return "", providerBadResponse()
	}
}
func usageFromAnthropic(usage anthropic.Usage) *llm.Usage {
	return normalizedUsage(usage.InputTokens, usage.CacheCreationInputTokens, usage.CacheReadInputTokens, usage.OutputTokens)
}
func normalizedUsage(input, cacheCreate, cacheRead, output int64) *llm.Usage {
	prompt := int(input + cacheCreate + cacheRead)
	completion := int(output)
	return &llm.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
}
func (a *Adapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}
	stream := a.client.Messages.NewStreaming(ctx, params)
	events := make(chan llm.StreamEvent)
	go a.runStream(ctx, stream, events, req.Model)
	return events, nil
}
func (a *Adapter) runStream(ctx context.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion], events chan<- llm.StreamEvent, model string) {
	defer close(events)
	state := streamState{tools: toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID, MaxArgumentBytes: maxToolArgumentBytes}), toolIndexes: map[int]struct{}{}, closed: map[int]struct{}{}}
	for stream.Next() {
		output, err := state.apply(stream.Current().RawJSON())
		if err != nil {
			a.sendStreamEvent(ctx, events, streamError(a.id, model))
			return
		}
		for _, event := range output {
			event.Provider = a.id
			event.Model = model
			if !a.sendStreamEvent(ctx, events, event) {
				return
			}
		}
	}
	if ctx.Err() != nil {
		return
	}
	if stream.Err() != nil || !state.isComplete() {
		a.sendStreamEvent(ctx, events, streamError(a.id, model))
		return
	}
	a.sendStreamEvent(ctx, events, llm.StreamEvent{Done: true, Provider: a.id, Model: model})
}
func (a *Adapter) sendStreamEvent(ctx context.Context, events chan<- llm.StreamEvent, event llm.StreamEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
func (state *streamState) apply(raw string) ([]llm.StreamEvent, error) {
	var event map[string]any
	if err := json.Unmarshal([]byte(raw), &event); err != nil || event == nil {
		return nil, providerBadResponse()
	}
	typeName, ok := event["type"].(string)
	if !ok || state.stopped || (!state.started && typeName != "message_start") {
		return nil, providerBadResponse()
	}
	switch typeName {
	case "message_start":
		return state.messageStart(event)
	case "content_block_start":
		return state.blockStart(event)
	case "content_block_delta":
		return state.blockDelta(event)
	case "content_block_stop":
		return state.blockStop(event)
	case "message_delta":
		return state.messageDelta(event)
	case "message_stop":
		return state.messageStop()
	default:
		return nil, providerBadResponse()
	}
}
func (state *streamState) messageStart(event map[string]any) ([]llm.StreamEvent, error) {
	if state.started {
		return nil, providerBadResponse()
	}
	message, ok := event["message"].(map[string]any)
	if !ok {
		return nil, providerBadResponse()
	}
	state.started = true
	state.usage = usageFromMap(message["usage"], state.usage)
	usage := state.usage
	return []llm.StreamEvent{{Usage: &usage}}, nil
}
func (state *streamState) blockStart(event map[string]any) ([]llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	block, ok := event["content_block"].(map[string]any)
	if err != nil || !ok {
		return nil, providerBadResponse()
	}
	if block["type"] != "tool_use" {
		return nil, nil
	}
	if _, exists := state.toolIndexes[index]; exists {
		return nil, providerBadResponse()
	}
	chunk, err := streamToolStart(index, block)
	if err != nil {
		return nil, err
	}
	next, normalized, err := state.tools.Apply(chunk)
	if err != nil {
		return nil, err
	}
	state.tools = next
	state.toolIndexes[index] = struct{}{}
	return []llm.StreamEvent{{ToolCalls: []llm.ToolCallChunk{normalized}}}, nil
}
func streamToolStart(index int, block map[string]any) (llm.ToolCallChunk, error) {
	name, ok := block["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return llm.ToolCallChunk{}, providerBadResponse()
	}
	id, hasID := block["id"].(string)
	if _, exists := block["id"]; exists && (!hasID || strings.TrimSpace(id) == "") {
		return llm.ToolCallChunk{}, providerBadResponse()
	}
	return toolChunk(index, optionalString(id, hasID), &name, ""), nil
}
func (state *streamState) blockDelta(event map[string]any) ([]llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	delta, ok := event["delta"].(map[string]any)
	if err != nil || !ok {
		return nil, providerBadResponse()
	}
	if delta["type"] == "text_delta" {
		text, ok := delta["text"].(string)
		if !ok {
			return nil, providerBadResponse()
		}
		return []llm.StreamEvent{{DeltaContent: text}}, nil
	}
	if delta["type"] != "input_json_delta" {
		return nil, providerBadResponse()
	}
	if _, exists := state.toolIndexes[index]; !exists {
		return nil, providerBadResponse()
	}
	fragment, ok := delta["partial_json"].(string)
	if !ok {
		return nil, providerBadResponse()
	}
	next, normalized, err := state.tools.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	if err != nil {
		return nil, err
	}
	state.tools = next
	return []llm.StreamEvent{{ToolCalls: []llm.ToolCallChunk{normalized}}}, nil
}
func (state *streamState) blockStop(event map[string]any) ([]llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	if err != nil {
		return nil, providerBadResponse()
	}
	if _, exists := state.toolIndexes[index]; !exists {
		return nil, nil
	}
	if _, exists := state.closed[index]; exists {
		return nil, providerBadResponse()
	}
	next, err := state.tools.CompleteCall(index)
	if err != nil {
		return nil, err
	}
	state.tools = next
	state.closed[index] = struct{}{}
	return nil, nil
}
func (state *streamState) messageDelta(event map[string]any) ([]llm.StreamEvent, error) {
	if state.finishReason != "" {
		return nil, providerBadResponse()
	}
	delta, ok := event["delta"].(map[string]any)
	reason, okReason := delta["stop_reason"].(string)
	if !ok || !okReason {
		return nil, providerBadResponse()
	}
	finish, err := normalizedFinishReason(reason, len(state.toolIndexes) > 0)
	if err != nil || (finish == "tool_calls" && !state.finishTools()) {
		return nil, providerBadResponse()
	}
	state.finishReason = finish
	state.usage = usageFromMap(event["usage"], state.usage)
	usage := state.usage
	return []llm.StreamEvent{{FinishReason: finish, Usage: &usage}}, nil
}
func (state *streamState) finishTools() bool {
	if len(state.closed) != len(state.toolIndexes) || len(state.toolIndexes) == 0 {
		return false
	}
	next, completion, err := state.tools.Finish("tool_calls")
	if err != nil || !toolArgumentsAreObjects(completion.Calls) {
		return false
	}
	state.tools = next
	return true
}
func (state *streamState) messageStop() ([]llm.StreamEvent, error) {
	if state.finishReason == "" {
		return nil, providerBadResponse()
	}
	state.stopped = true
	return nil, nil
}
func (state streamState) isComplete() bool {
	return state.started && state.stopped && state.finishReason != ""
}
func usageFromMap(value any, current llm.Usage) llm.Usage {
	usage, ok := value.(map[string]any)
	if !ok {
		return current
	}
	input := numericField(usage, "input_tokens")
	cacheCreate := numericField(usage, "cache_creation_input_tokens")
	cacheRead := numericField(usage, "cache_read_input_tokens")
	if input != 0 || cacheCreate != 0 || cacheRead != 0 {
		current.PromptTokens = input + cacheCreate + cacheRead
	}
	if _, exists := usage["output_tokens"]; exists {
		current.CompletionTokens = numericField(usage, "output_tokens")
	}
	current.TotalTokens = current.PromptTokens + current.CompletionTokens
	return current
}
func numericField(values map[string]any, name string) int {
	value, ok := values[name].(float64)
	if !ok || value < 0 || value != float64(int(value)) {
		return 0
	}
	return int(value)
}
func streamIndex(value any) (int, error) {
	index, ok := value.(float64)
	if !ok || index < 0 || index != float64(int(index)) {
		return 0, providerBadResponse()
	}
	return int(index), nil
}
func streamError(provider, model string) llm.StreamEvent {
	return llm.StreamEvent{Error: providerBadResponse(), Provider: provider, Model: model}
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
