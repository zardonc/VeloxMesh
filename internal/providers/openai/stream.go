package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"strings"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/toolstream"
)

type streamRead struct {
	ctx    context.Context
	events chan<- llm.StreamEvent
	model  string
}

type streamReadState struct {
	protocol      toolstream.State
	indexes       map[int]struct{}
	sawEvent      bool
	sawToolCall   bool
	terminal      bool
	sawFinalUsage bool
}

type streamChunkState struct {
	event       llm.StreamEvent
	protocol    toolstream.State
	indexes     map[int]struct{}
	hasToolCall bool
}

func (a *Adapter) readStream(run streamRead, resp io.ReadCloser) {
	defer close(run.events)
	defer resp.Close()
	stopClose := context.AfterFunc(run.ctx, func() { _ = resp.Close() })
	defer stopClose()
	state := streamReadState{protocol: toolstream.New(toolstream.Config{GenerateID: a.generateToolCallID}), indexes: map[int]struct{}{}}
	reader := bufio.NewReader(resp)
	for {
		line, readErr := reader.ReadBytes('\n')
		data := strings.TrimSpace(string(line))
		if strings.HasPrefix(data, "data:") {
			next, keepReading := a.handleStreamData(run, state, strings.TrimSpace(strings.TrimPrefix(data, "data:")))
			state = next
			if !keepReading {
				return
			}
		}
		if readErr == nil {
			continue
		}
		if run.ctx.Err() != nil {
			return
		}
		if state.sawToolCall || !state.sawEvent || !errors.Is(readErr, io.EOF) {
			a.sendStreamError(run.ctx, run.events)
			return
		}
		sendStreamEvent(run.ctx, run.events, llm.StreamEvent{Done: true, Provider: a.id, Model: run.model})
		return
	}
}

func (a *Adapter) handleStreamData(run streamRead, state streamReadState, data string) (streamReadState, bool) {
	if data == "[DONE]" {
		if state.sawToolCall && !state.terminal {
			a.sendStreamError(run.ctx, run.events)
			return state, false
		}
		sendStreamEvent(run.ctx, run.events, llm.StreamEvent{Done: true, Provider: a.id, Model: run.model})
		return state, false
	}
	var chunk streamChunk
	if json.Unmarshal([]byte(data), &chunk) != nil {
		a.sendStreamError(run.ctx, run.events)
		return state, false
	}
	if state.terminal {
		return a.handleFinalUsage(run, state, chunk)
	}
	event, protocol, indexes, hasToolCall, err := a.normalizeStreamChunk(state.protocol, chunk, run.model)
	next, valid := a.completeStreamEvent(state, streamChunkState{event: event, protocol: protocol, indexes: indexes, hasToolCall: hasToolCall})
	if err != nil || !valid {
		a.sendStreamError(run.ctx, run.events)
		return state, false
	}
	if !emitStreamChunk(run, event) {
		return state, false
	}
	next.sawEvent = true
	return next, true
}

func emitStreamChunk(run streamRead, event llm.StreamEvent) bool {
	if event.DeltaContent == "" && event.FinishReason == "" && event.Usage == nil && len(event.ToolCalls) == 0 {
		return true
	}
	return sendStreamEvent(run.ctx, run.events, event)
}

func (a *Adapter) handleFinalUsage(run streamRead, state streamReadState, chunk streamChunk) (streamReadState, bool) {
	if state.sawFinalUsage || len(chunk.Choices) != 0 || !validFinalUsage(chunk.Usage) {
		a.sendStreamError(run.ctx, run.events)
		return state, false
	}
	next := state
	next.sawFinalUsage = true
	return next, sendStreamEvent(run.ctx, run.events, llm.StreamEvent{Usage: chunk.Usage, Provider: a.id, Model: run.model})
}

func validFinalUsage(usage *llm.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.PromptTokens >= 0 && usage.CompletionTokens >= 0 && usage.TotalTokens >= 0 && usage.TotalTokens == usage.PromptTokens+usage.CompletionTokens
}

func (a *Adapter) completeStreamEvent(state streamReadState, chunk streamChunkState) (streamReadState, bool) {
	next := state
	next.protocol, next.indexes = chunk.protocol, maps.Clone(state.indexes)
	for index := range chunk.indexes {
		next.indexes[index] = struct{}{}
	}
	next.sawToolCall = state.sawToolCall || chunk.hasToolCall
	if chunk.event.FinishReason == "" {
		return next, true
	}
	if !next.sawToolCall {
		next.terminal = chunk.event.FinishReason != "tool_calls"
		return next, next.terminal
	}
	if chunk.event.FinishReason != "tool_calls" {
		return state, false
	}
	completed, err := finishToolStream(next.protocol, next.indexes)
	if err != nil {
		return state, false
	}
	next.protocol, next.terminal = completed, true
	return next, true
}

func (a *Adapter) normalizeStreamChunk(state toolstream.State, chunk streamChunk, fallbackModel string) (llm.StreamEvent, toolstream.State, map[int]struct{}, bool, error) {
	event := llm.StreamEvent{Provider: a.id, Model: chunk.Model, Usage: chunk.Usage}
	if event.Model == "" {
		event.Model = fallbackModel
	}
	indexes := map[int]struct{}{}
	if len(chunk.Choices) == 0 {
		return event, state, indexes, false, nil
	}
	choice := chunk.Choices[0]
	event.DeltaContent = choice.Delta.Content
	if choice.FinishReason != nil {
		event.FinishReason = *choice.FinishReason
	}
	if len(choice.Delta.ToolCalls) == 0 {
		return event, state, indexes, false, nil
	}
	next := state
	chunks := make([]llm.ToolCallChunk, 0, len(choice.Delta.ToolCalls))
	for _, fragment := range choice.Delta.ToolCalls {
		updated, normalized, err := next.Apply(fragment)
		if err != nil || normalized.Index == nil {
			return llm.StreamEvent{}, state, indexes, false, providerToolError()
		}
		next = updated
		indexes[*normalized.Index] = struct{}{}
		chunks = append(chunks, normalized)
	}
	event.ToolCalls = chunks
	return event, next, indexes, true, nil
}

func finishToolStream(state toolstream.State, indexes map[int]struct{}) (toolstream.State, error) {
	next := state
	for index := range indexes {
		updated, err := next.CompleteCall(index)
		if err != nil {
			return state, providerToolError()
		}
		next = updated
	}
	finished, _, err := next.Finish("tool_calls")
	if err != nil {
		return state, providerToolError()
	}
	return finished, nil
}

func sendStreamEvent(ctx context.Context, ch chan<- llm.StreamEvent, event llm.StreamEvent) bool {
	select {
	case ch <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (a *Adapter) sendStreamError(ctx context.Context, ch chan<- llm.StreamEvent) {
	sendStreamEvent(ctx, ch, llm.StreamEvent{Error: providerToolError(), Provider: a.id})
}
