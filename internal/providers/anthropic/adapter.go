package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type Adapter struct {
	id           string
	client       *anthropic.Client
	models       []string
	defaultModel string
}

func NewAdapter(id, baseURL, apiKey, modelsStr string) providers.ProviderAdapter {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := anthropic.NewClient(opts...)

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
		client:       &client,
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
		ProviderType:         providers.ProviderTypeAnthropic,
		SupportedOperations:  []providers.Operation{providers.OperationChatCompletions},
		InputModalities:      []providers.Modality{providers.ModalityText},
		OutputModalities:     []providers.Modality{providers.ModalityText},
		Streaming:            true,
		ToolCalling:          true,
		GenerationParameters: []providers.GenerationParameter{providers.GenerationParameterTemperature, providers.GenerationParameterMaxTokens},
	}
}

func (a *Adapter) HealthCheck(ctx context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true, Message: "Anthropic native health check not implemented"}
}

func (a *Adapter) buildParams(req *llm.LLMRequest) (anthropic.MessageNewParams, error) {
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}

	var systemBlocks []anthropic.TextBlockParam
	var anthropicMessages []anthropic.MessageParam

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
				Text: msg.Content,
			})
		case llm.RoleUser:
			if len(msg.MultiContent) > 0 {
				var blocks []anthropic.ContentBlockParamUnion
				for _, part := range msg.MultiContent {
					if part.Type == llm.ContentTypeText {
						blocks = append(blocks, anthropic.NewTextBlock(part.Text))
					} else if part.Type == llm.ContentTypeImageURL && part.ImageURL != nil {
						if strings.HasPrefix(part.ImageURL.URL, "data:image/") {
							parts := strings.SplitN(part.ImageURL.URL, ";base64,", 2)
							if len(parts) == 2 {
								mediaType := strings.TrimPrefix(parts[0], "data:")
								blocks = append(blocks, anthropic.NewImageBlockBase64(mediaType, parts[1]))
							}
						}
					}
				}
				anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(blocks...))
			} else {
				anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
			}
		case llm.RoleAssistant:
			anthropicMessages = append(anthropicMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		default:
			anthropicMessages = append(anthropicMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		}
	}

	var anthropicTools []anthropic.ToolUnionParam
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			if t.Type == llm.ToolTypeFunction && t.Function != nil {
				var schemaMap struct {
					Properties any      `json:"properties"`
					Required   []string `json:"required"`
				}
				if t.Function.Parameters != nil {
					b, _ := json.Marshal(t.Function.Parameters)
					_ = json.Unmarshal(b, &schemaMap)
				}
				tool := anthropic.ToolParam{
					Name:        t.Function.Name,
					Description: anthropic.String(t.Function.Description),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: schemaMap.Properties,
						Required:   schemaMap.Required,
					},
				}
				anthropicTools = append(anthropicTools, anthropic.ToolUnionParam{OfTool: &tool})
			}
		}
	}

	maxTokens := int64(4096)
	if req.MaxTokens != nil {
		maxTokens = int64(*req.MaxTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages:  anthropicMessages,
	}

	if len(systemBlocks) > 0 {
		params.System = systemBlocks
	}

	if len(anthropicTools) > 0 {
		params.Tools = anthropicTools
	}

	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	return params, nil
}

func (a *Adapter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Messages.New(ctx, params)
	if err != nil {
		return nil, a.mapError(err)
	}

	if len(resp.Content) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned empty content", http.StatusBadGateway)
	}

	content := ""
	var toolCalls []llm.ToolCall

	for _, blockUnion := range resp.Content {
		b, _ := json.Marshal(blockUnion)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if m["type"] == "text" {
			if txt, ok := m["text"].(string); ok {
				content += txt
			}
		} else if m["type"] == "tool_use" {
			id, _ := m["id"].(string)
			name, _ := m["name"].(string)
			input, _ := m["input"]
			inputBytes, _ := json.Marshal(input)
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   id,
				Type: llm.ToolTypeFunction,
				Function: llm.FunctionCall{
					Name:      name,
					Arguments: string(inputBytes),
				},
			})
		}
	}

	if content == "" && len(toolCalls) == 0 {
		return nil, gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "Provider returned no text content or tool calls", http.StatusBadGateway)
	}

	finishReason := "stop"
	switch resp.StopReason {
	case "end_turn":
		finishReason = "stop"
	case "max_tokens":
		finishReason = "length"
	case "stop_sequence":
		finishReason = "stop"
	case "tool_use":
		finishReason = "tool_calls"
	}

	return &llm.LLMResponse{
		Model:    resp.Model,
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

func (a *Adapter) Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	params, err := a.buildParams(req)
	if err != nil {
		return nil, err
	}

	respStream := a.client.Messages.NewStreaming(ctx, params)
	ch := make(chan llm.StreamEvent)

	go func() {
		defer close(ch)
		var usage llm.Usage

		for respStream.Next() {
			event := respStream.Current()
			
			b, _ := json.Marshal(event)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			
			typ, _ := m["type"].(string)

			switch typ {
			case "message_start":
				if msg, ok := m["message"].(map[string]any); ok {
					if usg, ok := msg["usage"].(map[string]any); ok {
						if in, ok := usg["input_tokens"].(float64); ok {
							usage.PromptTokens = int(in)
						}
					}
				}
				ch <- llm.StreamEvent{Usage: &usage}
			
			case "content_block_start":
				if block, ok := m["content_block"].(map[string]any); ok {
					if bType, _ := block["type"].(string); bType == "tool_use" {
						id, _ := block["id"].(string)
						name, _ := block["name"].(string)
						idx, _ := m["index"].(float64)
						
						idxInt := int(idx)
						toolType := llm.ToolTypeFunction
						ch <- llm.StreamEvent{
							ToolCalls: []llm.ToolCallChunk{
								{
									Index: &idxInt,
									ID:    &id,
									Type:  &toolType,
									Function: &llm.FunctionCallChunk{
										Name: &name,
									},
								},
							},
						}
					}
				}
			
			case "content_block_delta":
				if delta, ok := m["delta"].(map[string]any); ok {
					dType, _ := delta["type"].(string)
					if dType == "text_delta" {
						if txt, ok := delta["text"].(string); ok {
							ch <- llm.StreamEvent{DeltaContent: txt}
						}
					} else if dType == "input_json_delta" {
						if partial, ok := delta["partial_json"].(string); ok {
							idx, _ := m["index"].(float64)
							idxInt := int(idx)
							ch <- llm.StreamEvent{
								ToolCalls: []llm.ToolCallChunk{
									{
										Index: &idxInt,
										Function: &llm.FunctionCallChunk{
											Arguments: &partial,
										},
									},
								},
							}
						}
					}
				}
			
			case "message_delta":
				if delta, ok := m["delta"].(map[string]any); ok {
					if stopR, ok := delta["stop_reason"].(string); ok {
						fr := "stop"
						switch stopR {
						case "end_turn":
							fr = "stop"
						case "max_tokens":
							fr = "length"
						case "tool_use":
							fr = "tool_calls"
						}
						ch <- llm.StreamEvent{FinishReason: fr}
					}
				}
				if usg, ok := m["usage"].(map[string]any); ok {
					if out, ok := usg["output_tokens"].(float64); ok {
						usage.CompletionTokens += int(out)
						usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
						ch <- llm.StreamEvent{Usage: &usage}
					}
				}
			}
		}

		if err := respStream.Err(); err != nil {
			ch <- llm.StreamEvent{Error: a.mapError(err)}
		} else {
			ch <- llm.StreamEvent{Done: true}
		}
	}()

	return ch, nil
}
