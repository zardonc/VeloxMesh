package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/toolstream"
)

const maxToolArgumentBytes = 1024 * 1024

type streamRun struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
	events chan<- llm.StreamEvent
	model  string
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

func newStreamState(generateID func() string) streamState {
	return streamState{
		tools: toolstream.New(toolstream.Config{
			GenerateID:       generateID,
			MaxArgumentBytes: maxToolArgumentBytes,
		}),
		toolIndexes: map[int]struct{}{},
		closed:      map[int]struct{}{},
	}
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
	return toolChunk(toolChunkFields{index: index, id: optionalString(id, hasID), name: &name, arguments: string(arguments)}), nil
}

type toolChunkFields struct {
	index     int
	id        *string
	name      *string
	arguments string
}

func toolChunk(fields toolChunkFields) llm.ToolCallChunk {
	toolType := llm.ToolTypeFunction
	return llm.ToolCallChunk{Index: &fields.index, ID: fields.id, Type: &toolType, Function: &llm.FunctionCallChunk{Name: fields.name, Arguments: &fields.arguments}}
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
	prompt := int(usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens)
	completion := int(usage.OutputTokens)
	return &llm.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion}
}
func (state streamState) apply(raw string) (streamState, []llm.StreamEvent, error) {
	var event map[string]any
	if err := json.Unmarshal([]byte(raw), &event); err != nil || event == nil {
		return state, nil, providerBadResponse()
	}
	typeName, ok := event["type"].(string)
	if !ok || state.stopped || (!state.started && typeName != "message_start") {
		return state, nil, providerBadResponse()
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
		return state, nil, providerBadResponse()
	}
}

func (state streamState) messageStart(event map[string]any) (streamState, []llm.StreamEvent, error) {
	if state.started {
		return state, nil, providerBadResponse()
	}
	message, ok := event["message"].(map[string]any)
	if !ok {
		return state, nil, providerBadResponse()
	}
	next := state
	next.started = true
	next.usage = usageFromMap(message["usage"], state.usage)
	usage := next.usage
	return next, []llm.StreamEvent{{Usage: &usage}}, nil
}

func (state streamState) blockStart(event map[string]any) (streamState, []llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	block, ok := event["content_block"].(map[string]any)
	if err != nil || !ok {
		return state, nil, providerBadResponse()
	}
	if block["type"] != "tool_use" {
		return state, nil, nil
	}
	if _, exists := state.toolIndexes[index]; exists {
		return state, nil, providerBadResponse()
	}
	chunk, err := streamToolStart(index, block)
	if err != nil {
		return state, nil, err
	}
	tools, normalized, err := state.tools.Apply(chunk)
	if err != nil {
		return state, nil, err
	}
	next := state
	next.tools = tools
	next.toolIndexes = cloneIndexes(state.toolIndexes)
	next.toolIndexes[index] = struct{}{}
	return next, []llm.StreamEvent{{ToolCalls: []llm.ToolCallChunk{normalized}}}, nil
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
	return toolChunk(toolChunkFields{index: index, id: optionalString(id, hasID), name: &name}), nil
}

func (state streamState) blockDelta(event map[string]any) (streamState, []llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	delta, ok := event["delta"].(map[string]any)
	if err != nil || !ok {
		return state, nil, providerBadResponse()
	}
	if delta["type"] == "text_delta" {
		text, ok := delta["text"].(string)
		if !ok {
			return state, nil, providerBadResponse()
		}
		return state, []llm.StreamEvent{{DeltaContent: text}}, nil
	}
	if delta["type"] != "input_json_delta" {
		return state, nil, providerBadResponse()
	}
	if _, exists := state.toolIndexes[index]; !exists {
		return state, nil, providerBadResponse()
	}
	fragment, ok := delta["partial_json"].(string)
	if !ok {
		return state, nil, providerBadResponse()
	}
	tools, normalized, err := state.tools.Apply(llm.ToolCallChunk{Index: &index, Function: &llm.FunctionCallChunk{Arguments: &fragment}})
	if err != nil {
		return state, nil, err
	}
	next := state
	next.tools = tools
	return next, []llm.StreamEvent{{ToolCalls: []llm.ToolCallChunk{normalized}}}, nil
}

func (state streamState) blockStop(event map[string]any) (streamState, []llm.StreamEvent, error) {
	index, err := streamIndex(event["index"])
	if err != nil {
		return state, nil, providerBadResponse()
	}
	if _, exists := state.toolIndexes[index]; !exists {
		return state, nil, nil
	}
	if _, exists := state.closed[index]; exists {
		return state, nil, providerBadResponse()
	}
	tools, err := state.tools.CompleteCall(index)
	if err != nil {
		return state, nil, err
	}
	next := state
	next.tools = tools
	next.closed = cloneIndexes(state.closed)
	next.closed[index] = struct{}{}
	return next, nil, nil
}

func (state streamState) messageDelta(event map[string]any) (streamState, []llm.StreamEvent, error) {
	if state.finishReason != "" {
		return state, nil, providerBadResponse()
	}
	delta, ok := event["delta"].(map[string]any)
	reason, okReason := delta["stop_reason"].(string)
	if !ok || !okReason {
		return state, nil, providerBadResponse()
	}
	finish, err := normalizedFinishReason(reason, len(state.toolIndexes) > 0)
	if err != nil {
		return state, nil, providerBadResponse()
	}
	next := state
	if finish == "tool_calls" {
		var finished bool
		next, finished = state.finishTools()
		if !finished {
			return state, nil, providerBadResponse()
		}
	}
	next.finishReason = finish
	next.usage = usageFromMap(event["usage"], state.usage)
	usage := next.usage
	return next, []llm.StreamEvent{{FinishReason: finish, Usage: &usage}}, nil
}

func (state streamState) finishTools() (streamState, bool) {
	if len(state.closed) != len(state.toolIndexes) || len(state.toolIndexes) == 0 {
		return state, false
	}
	tools, completion, err := state.tools.Finish("tool_calls")
	if err != nil || !toolArgumentsAreObjects(completion.Calls) {
		return state, false
	}
	next := state
	next.tools = tools
	return next, true
}

func (state streamState) messageStop() (streamState, []llm.StreamEvent, error) {
	if state.finishReason == "" {
		return state, nil, providerBadResponse()
	}
	next := state
	next.stopped = true
	return next, nil, nil
}

func (state streamState) isComplete() bool {
	return state.started && state.stopped && state.finishReason != ""
}

func cloneIndexes(source map[int]struct{}) map[int]struct{} {
	clone := make(map[int]struct{}, len(source))
	for index := range source {
		clone[index] = struct{}{}
	}
	return clone
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
