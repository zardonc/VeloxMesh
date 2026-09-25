// Package toolstream normalizes provider tool-call deltas without owning stream lifecycle.
package toolstream

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

const toolCallsFinishReason = "tool_calls"

type Config struct {
	GenerateID func() string
}

type State struct {
	calls      map[int]callState
	ids        map[string]int
	finished   bool
	generateID func() string
}

type callState struct {
	id        string
	name      string
	arguments string
	complete  bool
}

type Completion struct {
	FinishReason string
	Calls        []llm.ToolCall
}

func New(config Config) State {
	return State{
		calls:      make(map[int]callState),
		ids:        make(map[string]int),
		generateID: config.GenerateID,
	}
}

func (state State) Apply(chunk llm.ToolCallChunk) (State, llm.ToolCallChunk, error) {
	if state.finished {
		return state, llm.ToolCallChunk{}, protocolError()
	}

	index, err := state.resolveIndex(chunk.Index)
	if err != nil {
		return state, llm.ToolCallChunk{}, err
	}

	call, exists := state.calls[index]
	if !exists {
		return state.applyNew(index, chunk)
	}
	return state.applyExisting(index, call, chunk)
}

func (state State) CompleteCall(index int) (State, error) {
	call, exists := state.calls[index]
	if state.finished || !exists || call.complete || !isCompleteJSON(call.arguments) {
		return state, protocolError()
	}

	next := state.copy()
	call.complete = true
	next.calls[index] = call
	return next, nil
}

func (state State) Finish(reason string) (State, Completion, error) {
	if state.finished || reason != toolCallsFinishReason || len(state.calls) == 0 {
		return state, Completion{}, protocolError()
	}
	if !state.allCallsComplete() {
		return state, Completion{}, protocolError()
	}

	next := state.copy()
	next.finished = true
	return next, Completion{FinishReason: toolCallsFinishReason, Calls: state.callsInIndexOrder()}, nil
}

func (state State) applyNew(index int, chunk llm.ToolCallChunk) (State, llm.ToolCallChunk, error) {
	name, arguments, ok := newCallFields(chunk)
	if !ok {
		return state, llm.ToolCallChunk{}, protocolError()
	}

	id, err := state.callID(chunk.ID)
	if err != nil {
		return state, llm.ToolCallChunk{}, err
	}
	if _, used := state.ids[id]; used {
		return state, llm.ToolCallChunk{}, protocolError()
	}

	next := state.copy()
	next.calls[index] = callState{id: id, name: name, arguments: arguments}
	next.ids[id] = index
	return next, normalizedChunk(index, id, name, chunkArguments(chunk.Function)), nil
}

func (state State) applyExisting(index int, call callState, chunk llm.ToolCallChunk) (State, llm.ToolCallChunk, error) {
	if call.complete || !matchesExistingCall(call, chunk) {
		return state, llm.ToolCallChunk{}, protocolError()
	}

	next := state.copy()
	if arguments := fragment(chunk.Function); arguments != "" {
		call.arguments += arguments
		next.calls[index] = call
	}
	return next, opaqueChunk(index, chunk), nil
}

func (state State) resolveIndex(index *int) (int, error) {
	if index != nil {
		if *index < 0 {
			return 0, protocolError()
		}
		return *index, nil
	}

	active := state.activeIndexes()
	if len(active) != 1 {
		return 0, protocolError()
	}
	return active[0], nil
}

func (state State) callID(id *string) (string, error) {
	if id != nil {
		if strings.TrimSpace(*id) == "" {
			return "", protocolError()
		}
		return *id, nil
	}
	if state.generateID == nil {
		return "", protocolError()
	}
	generated := state.generateID()
	if strings.TrimSpace(generated) == "" {
		return "", protocolError()
	}
	return generated, nil
}

func (state State) copy() State {
	return State{
		calls:      cloneCalls(state.calls),
		ids:        cloneIDs(state.ids),
		finished:   state.finished,
		generateID: state.generateID,
	}
}

func (state State) activeIndexes() []int {
	indexes := make([]int, 0, len(state.calls))
	for index, call := range state.calls {
		if !call.complete {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func (state State) allCallsComplete() bool {
	for _, call := range state.calls {
		if !call.complete {
			return false
		}
	}
	return true
}

func (state State) callsInIndexOrder() []llm.ToolCall {
	indexes := make([]int, 0, len(state.calls))
	for index := range state.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	calls := make([]llm.ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := state.calls[index]
		calls = append(calls, llm.ToolCall{
			ID:   call.id,
			Type: llm.ToolTypeFunction,
			Function: llm.FunctionCall{
				Name:      call.name,
				Arguments: call.arguments,
			},
		})
	}
	return calls
}

func newCallFields(chunk llm.ToolCallChunk) (string, string, bool) {
	if chunk.Type == nil || *chunk.Type != llm.ToolTypeFunction || chunk.Function == nil || chunk.Function.Name == nil {
		return "", "", false
	}
	name := strings.TrimSpace(*chunk.Function.Name)
	if name == "" {
		return "", "", false
	}
	return name, fragment(chunk.Function), true
}

func matchesExistingCall(call callState, chunk llm.ToolCallChunk) bool {
	if chunk.ID != nil && *chunk.ID != call.id {
		return false
	}
	if chunk.Type != nil && *chunk.Type != llm.ToolTypeFunction {
		return false
	}
	if chunk.Function == nil {
		return chunk.ID != nil || chunk.Type != nil
	}
	if chunk.Function.Name != nil && *chunk.Function.Name != call.name {
		return false
	}
	return true
}

func normalizedChunk(index int, id, name string, arguments *string) llm.ToolCallChunk {
	indexCopy := index
	idCopy := id
	typeCopy := llm.ToolTypeFunction
	nameCopy := name
	return llm.ToolCallChunk{
		Index: &indexCopy,
		ID:    &idCopy,
		Type:  &typeCopy,
		Function: &llm.FunctionCallChunk{
			Name:      &nameCopy,
			Arguments: arguments,
		},
	}
}

func opaqueChunk(index int, chunk llm.ToolCallChunk) llm.ToolCallChunk {
	indexCopy := index
	emitted := llm.ToolCallChunk{Index: &indexCopy}
	if chunk.ID != nil {
		idCopy := *chunk.ID
		emitted.ID = &idCopy
	}
	if chunk.Type != nil {
		typeCopy := *chunk.Type
		emitted.Type = &typeCopy
	}
	if chunk.Function != nil {
		emitted.Function = &llm.FunctionCallChunk{}
		if chunk.Function.Name != nil {
			nameCopy := *chunk.Function.Name
			emitted.Function.Name = &nameCopy
		}
		emitted.Function.Arguments = chunkArguments(chunk.Function)
	}
	return emitted
}
func fragment(function *llm.FunctionCallChunk) string {
	if function == nil || function.Arguments == nil {
		return ""
	}
	return *function.Arguments
}

func chunkArguments(function *llm.FunctionCallChunk) *string {
	if function == nil || function.Arguments == nil {
		return nil
	}
	arguments := *function.Arguments
	return &arguments
}

func isCompleteJSON(arguments string) bool {
	return len(arguments) > 0 && jsonValid(arguments)
}

func jsonValid(arguments string) bool {
	return json.Valid([]byte(arguments))
}

func cloneCalls(calls map[int]callState) map[int]callState {
	clone := make(map[int]callState, len(calls))
	for index, call := range calls {
		clone[index] = call
	}
	return clone
}

func cloneIDs(ids map[string]int) map[string]int {
	clone := make(map[string]int, len(ids))
	for id, index := range ids {
		clone[id] = index
	}
	return clone
}

func protocolError() error {
	return gatewayerrors.NewGatewayError(gatewayerrors.ProviderBadResponse, "invalid tool call stream from provider", http.StatusBadGateway)
}
