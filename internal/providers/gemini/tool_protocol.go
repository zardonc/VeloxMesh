package gemini

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/genai"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/toolstream"
)

const (
	geminiFirstCandidateIndex   = 0
	geminiToolCallsFinishReason = "tool_calls"
	geminiToolCallIDPrefix      = "call_"
)

type geminiStreamState struct {
	tools     toolstream.State
	toolCount int
	terminal  bool
}

func geminiContents(messages []llm.Message) ([]*genai.Content, *genai.Content, error) {
	contents := make([]*genai.Content, 0, len(messages))
	callNames := make(map[string]string)
	var system *genai.Content
	for _, message := range messages {
		if message.Role == llm.RoleSystem {
			system = &genai.Content{Role: "system", Parts: geminiContentParts(message)}
			continue
		}
		content, names, err := geminiMessageContent(message, callNames)
		if err != nil {
			return nil, nil, err
		}
		for id, name := range names {
			callNames[id] = name
		}
		contents = append(contents, content)
	}
	return contents, system, nil
}

func geminiMessageContent(message llm.Message, callNames map[string]string) (*genai.Content, map[string]string, error) {
	if message.Role == llm.RoleTool {
		part, err := geminiFunctionResponse(message, callNames)
		if err != nil {
			return nil, nil, err
		}
		return &genai.Content{Role: "user", Parts: []*genai.Part{part}}, nil, nil
	}
	parts := geminiContentParts(message)
	names, toolParts, err := geminiFunctionCallParts(message)
	if err != nil {
		return nil, nil, err
	}
	parts = append(parts, toolParts...)
	return &genai.Content{Role: geminiRole(message.Role), Parts: parts}, names, nil
}

func geminiRole(role llm.Role) string {
	if role == llm.RoleAssistant {
		return "model"
	}
	return "user"
}

func geminiContentParts(message llm.Message) []*genai.Part {
	if len(message.MultiContent) == 0 {
		if message.Content == "" {
			return nil
		}
		return []*genai.Part{{Text: message.Content}}
	}
	parts := make([]*genai.Part, 0, len(message.MultiContent))
	for _, content := range message.MultiContent {
		if part := geminiContentPart(content); part != nil {
			parts = append(parts, part)
		}
	}
	return parts
}

func geminiContentPart(content llm.ContentPart) *genai.Part {
	if content.Type == llm.ContentTypeText {
		return &genai.Part{Text: content.Text}
	}
	if content.Type != llm.ContentTypeImageURL || content.ImageURL == nil {
		return nil
	}
	return geminiImagePart(content.ImageURL.URL)
}

func geminiImagePart(imageURL string) *genai.Part {
	if !strings.HasPrefix(imageURL, "data:image/") {
		return nil
	}
	encoded := strings.SplitN(imageURL, ";base64,", 2)
	if len(encoded) != 2 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded[1])
	if err != nil {
		return nil
	}
	return &genai.Part{InlineData: &genai.Blob{MIMEType: strings.TrimPrefix(encoded[0], "data:"), Data: data}}
}

func geminiFunctionCallParts(message llm.Message) (map[string]string, []*genai.Part, error) {
	if len(message.ToolCalls) == 0 {
		return nil, nil, nil
	}
	names := make(map[string]string, len(message.ToolCalls))
	parts := make([]*genai.Part, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		args, err := geminiArguments(call.Function.Arguments)
		if err != nil || !validGeminiFunctionCall(call) {
			return nil, nil, invalidGeminiToolRequest()
		}
		if _, exists := names[call.ID]; exists {
			return nil, nil, invalidGeminiToolRequest()
		}
		names[call.ID] = call.Function.Name
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{ID: call.ID, Name: call.Function.Name, Args: args}})
	}
	return names, parts, nil
}

func validGeminiFunctionCall(call llm.ToolCall) bool {
	return call.Type == llm.ToolTypeFunction && strings.TrimSpace(call.ID) != "" && strings.TrimSpace(call.Function.Name) != ""
}

func geminiFunctionResponse(message llm.Message, callNames map[string]string) (*genai.Part, error) {
	name, found := callNames[message.ToolCallID]
	response, err := geminiArguments(message.Content)
	if !found || err != nil {
		return nil, invalidGeminiToolRequest()
	}
	return &genai.Part{FunctionResponse: &genai.FunctionResponse{ID: message.ToolCallID, Name: name, Response: response}}, nil
}

func geminiArguments(raw string) (map[string]any, error) {
	arguments := make(map[string]any)
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &arguments) != nil {
		return nil, invalidGeminiToolRequest()
	}
	return arguments, nil
}

func geminiFunctionDeclarations(tools []llm.Tool) []*genai.FunctionDeclaration {
	functions := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
			continue
		}
		functions = append(functions, &genai.FunctionDeclaration{
			Name:                 tool.Function.Name,
			Description:          tool.Function.Description,
			ParametersJsonSchema: append(json.RawMessage(nil), tool.Function.Parameters...),
		})
	}
	return functions
}

func geminiToolConfig(choice *llm.ToolChoice, functions []*genai.FunctionDeclaration) (*genai.ToolConfig, error) {
	if choice == nil {
		return nil, nil
	}
	config := &genai.FunctionCallingConfig{}
	switch choice.Mode {
	case llm.ToolChoiceAuto:
		config.Mode = genai.FunctionCallingConfigModeAuto
	case llm.ToolChoiceNone:
		config.Mode = genai.FunctionCallingConfigModeNone
	case llm.ToolChoiceRequired:
		config.Mode = genai.FunctionCallingConfigModeAny
	case llm.ToolChoiceNamed:
		if !geminiFunctionExists(choice.FunctionName, functions) {
			return nil, invalidGeminiToolRequest()
		}
		config.Mode = genai.FunctionCallingConfigModeAny
		config.AllowedFunctionNames = []string{choice.FunctionName}
	default:
		return nil, invalidGeminiToolRequest()
	}
	return &genai.ToolConfig{FunctionCallingConfig: config}, nil
}

func geminiFunctionExists(name string, functions []*genai.FunctionDeclaration) bool {
	for _, function := range functions {
		if function.Name == name {
			return true
		}
	}
	return false
}

func geminiChoice(candidate *genai.Candidate, generateID func() string) (llm.Choice, error) {
	content, calls, err := geminiCandidateContent(candidate, generateID)
	if err != nil {
		return llm.Choice{}, err
	}
	finish, err := geminiFinish(candidate.FinishReason, len(calls) > 0)
	if err != nil || (content == "" && len(calls) == 0 && !geminiBlocked(candidate.FinishReason)) {
		return llm.Choice{}, invalidGeminiToolResponse()
	}
	return llm.Choice{
		Index:        geminiFirstCandidateIndex,
		Message:      llm.Message{Role: llm.RoleAssistant, Content: content, ToolCalls: calls},
		FinishReason: finish,
	}, nil
}

func geminiCandidateContent(candidate *genai.Candidate, generateID func() string) (string, []llm.ToolCall, error) {
	if candidate.Content == nil {
		return "", nil, nil
	}
	state := toolstream.New(toolstream.Config{GenerateID: generateID})
	content := ""
	callCount := 0
	for _, part := range candidate.Content.Parts {
		content += part.Text
		if part.FunctionCall == nil {
			continue
		}
		next, _, err := geminiApplyCall(state, callCount, part.FunctionCall)
		if err != nil {
			return "", nil, invalidGeminiToolResponse()
		}
		state = next
		callCount++
	}
	if callCount == 0 {
		return content, nil, nil
	}
	return geminiCompletedCalls(state, content)
}

func geminiCompletedCalls(state toolstream.State, content string) (string, []llm.ToolCall, error) {
	_, completion, err := state.Finish(geminiToolCallsFinishReason)
	if err != nil {
		return "", nil, invalidGeminiToolResponse()
	}
	return content, completion.Calls, nil
}

func geminiApplyCall(state toolstream.State, index int, call *genai.FunctionCall) (toolstream.State, llm.ToolCallChunk, error) {
	arguments, err := json.Marshal(call.Args)
	if err != nil || call.Args == nil || strings.TrimSpace(call.Name) == "" {
		return state, llm.ToolCallChunk{}, invalidGeminiToolResponse()
	}
	indexCopy := index
	typeCopy := llm.ToolTypeFunction
	nameCopy := call.Name
	argumentCopy := string(arguments)
	chunk := llm.ToolCallChunk{
		Index: &indexCopy,
		Type:  &typeCopy,
		Function: &llm.FunctionCallChunk{
			Name:      &nameCopy,
			Arguments: &argumentCopy,
		},
	}
	if strings.TrimSpace(call.ID) != "" {
		idCopy := call.ID
		chunk.ID = &idCopy
	}
	next, normalized, err := state.Apply(chunk)
	if err != nil {
		return state, llm.ToolCallChunk{}, err
	}
	next, err = next.CompleteCall(index)
	return next, normalized, err
}

func geminiFinish(reason genai.FinishReason, hasTools bool) (string, error) {
	if hasTools {
		if reason == "" || reason == "STOP" {
			return geminiToolCallsFinishReason, nil
		}
		return "", invalidGeminiToolResponse()
	}
	switch reason {
	case "", "STOP":
		return "stop", nil
	case "MAX_TOKENS":
		return "length", nil
	case "MALFORMED_FUNCTION_CALL", "UNEXPECTED_TOOL_CALL":
		return "", invalidGeminiToolResponse()
	default:
		return strings.ToLower(string(reason)), nil
	}
}

func geminiBlocked(reason genai.FinishReason) bool {
	switch reason {
	case "SAFETY", "RECITATION", "OTHER", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return true
	default:
		return false
	}
}

func usageFromGemini(usage *genai.GenerateContentResponseUsageMetadata) *llm.Usage {
	if usage == nil {
		return nil
	}
	prompt := int(usage.PromptTokenCount + usage.ToolUsePromptTokenCount)
	completion := int(usage.CandidatesTokenCount + usage.ThoughtsTokenCount)
	total := int(usage.TotalTokenCount)
	if total == 0 {
		total = prompt + completion
	}
	return &llm.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
}

func geminiStreamEvents(state geminiStreamState, response *genai.GenerateContentResponse) (geminiStreamState, []llm.StreamEvent, error) {
	usage := usageFromGemini(response.UsageMetadata)
	if len(response.Candidates) == 0 {
		return state, geminiUsageEvents(nil, usage, -1), nil
	}
	return geminiCandidateStreamEvents(state, response.Candidates[geminiFirstCandidateIndex], usage)
}

func geminiCandidateStreamEvents(state geminiStreamState, candidate *genai.Candidate, usage *llm.Usage) (geminiStreamState, []llm.StreamEvent, error) {
	events, next, err := geminiContentStreamEvents(state, candidate.Content)
	if err != nil || candidate.FinishReason == "" {
		return next, geminiUsageEvents(events, usage, -1), err
	}
	next, finish, err := geminiStreamFinish(next, candidate.FinishReason)
	if err != nil {
		return state, nil, err
	}
	finishIndex := len(events)
	events = append(events, llm.StreamEvent{FinishReason: finish})
	return next, geminiUsageEvents(events, usage, finishIndex), nil
}

func geminiContentStreamEvents(state geminiStreamState, content *genai.Content) ([]llm.StreamEvent, geminiStreamState, error) {
	if content == nil {
		return nil, state, nil
	}
	events := make([]llm.StreamEvent, 0, len(content.Parts))
	for _, part := range content.Parts {
		if part.Text != "" {
			events = append(events, llm.StreamEvent{DeltaContent: part.Text})
		}
		if part.FunctionCall == nil {
			continue
		}
		next, chunk, err := geminiStreamCall(state, part.FunctionCall)
		if err != nil {
			return nil, state, err
		}
		state = next
		events = append(events, llm.StreamEvent{ToolCalls: []llm.ToolCallChunk{chunk}})
	}
	return events, state, nil
}

func geminiUsageEvents(events []llm.StreamEvent, usage *llm.Usage, finishIndex int) []llm.StreamEvent {
	if usage == nil {
		return events
	}
	if finishIndex >= 0 {
		events[finishIndex].Usage = usage
		return events
	}
	return append(events, llm.StreamEvent{Usage: usage})
}

func geminiStreamCall(state geminiStreamState, call *genai.FunctionCall) (geminiStreamState, llm.ToolCallChunk, error) {
	nextTools, chunk, err := geminiApplyCall(state.tools, state.toolCount, call)
	if err != nil {
		return state, llm.ToolCallChunk{}, err
	}
	return geminiStreamState{tools: nextTools, toolCount: state.toolCount + 1, terminal: state.terminal}, chunk, nil
}

func geminiStreamFinish(state geminiStreamState, reason genai.FinishReason) (geminiStreamState, string, error) {
	finish, err := geminiFinish(reason, state.toolCount > 0)
	if err != nil {
		return state, "", err
	}
	if state.toolCount == 0 {
		return geminiStreamState{tools: state.tools, toolCount: state.toolCount, terminal: true}, finish, nil
	}
	nextTools, _, err := state.tools.Finish(geminiToolCallsFinishReason)
	if err != nil {
		return state, "", err
	}
	return geminiStreamState{tools: nextTools, toolCount: state.toolCount, terminal: true}, finish, nil
}

func opaqueToolCallID() string { return geminiToolCallIDPrefix + uuid.NewString() }

func invalidGeminiToolRequest() error { return gatewayErr.NewInvalidToolProtocolRequest() }

func invalidGeminiToolResponse() error {
	return gatewayErr.NewGatewayError(gatewayErr.ProviderBadResponse, "invalid provider tool protocol response", http.StatusBadGateway)
}
