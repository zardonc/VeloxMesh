package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	gatewayerrors "veloxmesh/internal/errors"
)

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "function"
)

type ToolChoice struct {
	Mode         ToolChoiceMode
	FunctionName string
}

type ToolProtocolRequirements struct {
	HasDefinitions       bool
	HasExplicitChoice    bool
	ChoiceMode           ToolChoiceMode
	NamedFunction        string
	HasAssistantToolCall bool
	HasToolResult        bool
}

func (choice ToolChoice) MarshalJSON() ([]byte, error) {
	switch choice.Mode {
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		return json.Marshal(string(choice.Mode))
	case ToolChoiceNamed:
		return json.Marshal(struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}{Type: string(ToolChoiceNamed), Function: struct {
			Name string `json:"name"`
		}{Name: choice.FunctionName}})
	default:
		return nil, fmt.Errorf("unknown tool choice mode")
	}
}

func ParseToolChoice(raw json.RawMessage) (*ToolChoice, *gatewayerrors.GatewayError) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return nil, nil
	}
	if value[0] == '"' {
		return parseToolChoiceMode(value)
	}
	if value[0] != '{' {
		return nil, invalidToolProtocol()
	}
	return parseNamedToolChoice(value)
}

func parseToolChoiceMode(raw json.RawMessage) (*ToolChoice, *gatewayerrors.GatewayError) {
	var mode ToolChoiceMode
	if err := json.Unmarshal(raw, &mode); err != nil {
		return nil, invalidToolProtocol()
	}
	switch mode {
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		return &ToolChoice{Mode: mode}, nil
	default:
		return nil, invalidToolProtocol()
	}
}

func parseNamedToolChoice(raw json.RawMessage) (*ToolChoice, *gatewayerrors.GatewayError) {
	var choice map[string]json.RawMessage
	if err := json.Unmarshal(raw, &choice); err != nil || len(choice) != 2 {
		return nil, invalidToolProtocol()
	}
	var choiceType string
	if err := json.Unmarshal(choice["type"], &choiceType); err != nil || choiceType != string(ToolChoiceNamed) {
		return nil, invalidToolProtocol()
	}
	var function map[string]json.RawMessage
	if err := json.Unmarshal(choice["function"], &function); err != nil || len(function) != 1 {
		return nil, invalidToolProtocol()
	}
	var name string
	if err := json.Unmarshal(function["name"], &name); err != nil {
		return nil, invalidToolProtocol()
	}
	return &ToolChoice{Mode: ToolChoiceNamed, FunctionName: name}, nil
}

func NormalizeToolProtocol(request ChatCompletionRequest) (ChatCompletionRequest, ToolProtocolRequirements, *gatewayerrors.GatewayError) {
	tools, names, err := normalizeTools(request.Tools)
	if err != nil {
		return ChatCompletionRequest{}, ToolProtocolRequirements{}, err
	}
	choice, err := normalizeToolChoice(request.ToolChoice, names)
	if err != nil {
		return ChatCompletionRequest{}, ToolProtocolRequirements{}, err
	}
	messages, requirements, err := normalizeMessages(request.Messages, names)
	if err != nil {
		return ChatCompletionRequest{}, ToolProtocolRequirements{}, err
	}
	requirements.HasDefinitions = len(tools) > 0
	if choice != nil {
		requirements.HasExplicitChoice = true
		requirements.ChoiceMode = choice.Mode
		requirements.NamedFunction = choice.FunctionName
	}
	return ChatCompletionRequest{
		Model: request.Model, Messages: messages, Temperature: request.Temperature,
		MaxTokens: request.MaxTokens, Stream: request.Stream, Tools: tools, ToolChoice: choice,
	}, requirements, nil
}

func normalizeTools(tools []Tool) ([]Tool, map[string]struct{}, *gatewayerrors.GatewayError) {
	normalized := make([]Tool, 0, len(tools))
	names := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.Type != ToolTypeFunction || tool.Function == nil || blank(tool.Function.Name) {
			return nil, nil, invalidToolProtocol()
		}
		if _, exists := names[tool.Function.Name]; exists || !validJSONObject(tool.Function.Parameters) {
			return nil, nil, invalidToolProtocol()
		}
		names[tool.Function.Name] = struct{}{}
		normalized = append(normalized, Tool{Type: ToolTypeFunction, Function: &Function{
			Name: tool.Function.Name, Description: tool.Function.Description,
			Parameters: append(json.RawMessage(nil), tool.Function.Parameters...),
		}})
	}
	return normalized, names, nil
}

func validJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func normalizeToolChoice(choice *ToolChoice, names map[string]struct{}) (*ToolChoice, *gatewayerrors.GatewayError) {
	if choice == nil {
		return nil, nil
	}
	if len(names) == 0 {
		return nil, invalidToolProtocol()
	}
	switch choice.Mode {
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired:
		return &ToolChoice{Mode: choice.Mode}, nil
	case ToolChoiceNamed:
	default:
		return nil, invalidToolProtocol()
	}
	if blank(choice.FunctionName) {
		return nil, invalidToolProtocol()
	}
	if _, declared := names[choice.FunctionName]; !declared {
		return nil, invalidToolProtocol()
	}
	return &ToolChoice{Mode: ToolChoiceNamed, FunctionName: choice.FunctionName}, nil
}

func normalizeMessages(messages []Message, names map[string]struct{}) ([]Message, ToolProtocolRequirements, *gatewayerrors.GatewayError) {
	normalized := make([]Message, 0, len(messages))
	ledger := newToolLedger(names)
	for _, message := range messages {
		if err := ledger.accept(message); err != nil {
			return nil, ToolProtocolRequirements{}, err
		}
		normalized = append(normalized, cloneMessage(message))
	}
	return normalized, ledger.requirements, nil
}

func cloneMessage(message Message) Message {
	copy := message
	copy.MultiContent = append([]ContentPart(nil), message.MultiContent...)
	copy.ToolCalls = append([]ToolCall(nil), message.ToolCalls...)
	return copy
}

type toolLedger struct {
	names        map[string]struct{}
	seen         map[string]struct{}
	pending      map[string]struct{}
	requirements ToolProtocolRequirements
}

func newToolLedger(names map[string]struct{}) toolLedger {
	return toolLedger{names: names, seen: map[string]struct{}{}, pending: map[string]struct{}{}}
}

func (ledger *toolLedger) accept(message Message) *gatewayerrors.GatewayError {
	if message.Role == RoleTool {
		return ledger.acceptResult(message)
	}
	if !validMessageRole(message.Role) || len(ledger.pending) > 0 {
		return invalidToolProtocol()
	}
	if message.Role == RoleAssistant {
		return ledger.acceptCalls(message.ToolCalls)
	}
	return nil
}

func (ledger *toolLedger) acceptCalls(calls []ToolCall) *gatewayerrors.GatewayError {
	for _, call := range calls {
		if blank(call.ID) || call.Type != ToolTypeFunction || blank(call.Function.Name) || !json.Valid([]byte(call.Function.Arguments)) {
			return invalidToolProtocol()
		}
		if _, declared := ledger.names[call.Function.Name]; !declared {
			return invalidToolProtocol()
		}
		if _, exists := ledger.seen[call.ID]; exists {
			return invalidToolProtocol()
		}
		ledger.seen[call.ID] = struct{}{}
		ledger.pending[call.ID] = struct{}{}
		ledger.requirements.HasAssistantToolCall = true
	}
	return nil
}

func (ledger *toolLedger) acceptResult(message Message) *gatewayerrors.GatewayError {
	if blank(message.ToolCallID) {
		return invalidToolProtocol()
	}
	if _, exists := ledger.pending[message.ToolCallID]; !exists {
		return invalidToolProtocol()
	}
	delete(ledger.pending, message.ToolCallID)
	ledger.requirements.HasToolResult = true
	return nil
}

func validMessageRole(role Role) bool {
	return role == RoleSystem || role == RoleUser || role == RoleAssistant
}

func blank(value string) bool {
	return strings.TrimSpace(value) == ""
}

func invalidToolProtocol() *gatewayerrors.GatewayError {
	return gatewayerrors.NewInvalidToolProtocolRequest()
}
