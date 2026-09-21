package handlers

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"time"

	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func (h *ChatHandler) streamChatCompletions(w http.ResponseWriter, r *http.Request, req *llm.LLMRequest, reqID string, start time.Time) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	ch, respMeta, err := h.service.HandleChatCompletionStream(ctx, req)
	if err != nil {
		sendGatewayError(w, gatewayerrors.TranslateError(err))
		return
	}
	configureStreamResponse(w, reqID, respMeta, time.Since(start))

	flusher, ok := w.(http.Flusher)
	if !ok {
		sendError(w, "not_supported", "Streaming not supported by client", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	emitter := sseEmitter{writer: w, flusher: flusher}
	streamSSEEvents(ctx, cancel, ch, emitter, reqID, respMeta.Model)
}

func configureStreamResponse(w http.ResponseWriter, reqID string, resp *llm.LLMResponse, duration time.Duration) {
	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("X-Provider", resp.Provider)
	w.Header().Set("X-Model", resp.Model)
	w.Header().Set("X-Cache-Hit", "false")
	w.Header().Set("X-Cache-Level", "none")
	w.Header().Set("X-Latency-E2E-Ms", fmt.Sprintf("%d", duration.Milliseconds()))
	w.Header().Set("X-Queue-Wait-Ms", fmt.Sprintf("%d", resp.QueueWaitMs))
	if resp.Strategy != "" {
		w.Header().Set("X-Routing-Strategy", resp.Strategy)
	}
	if resp.AttemptCount > 0 {
		w.Header().Set("X-Provider-Attempts", fmt.Sprintf("%d", resp.AttemptCount))
	}
	w.Header().Set("X-Fallback-Used", fmt.Sprintf("%t", resp.FallbackUsed))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func streamSSEEvents(ctx context.Context, cancel context.CancelFunc, events <-chan llm.StreamEvent, emitter sseEmitter, reqID, model string) {
	first := true
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			if event.Error != nil {
				if stderrors.Is(event.Error, context.Canceled) {
					return
				}
				if emitter.writeError(event.Error) != nil {
					cancel()
				}
				return
			}
			if event.Done {
				if emitter.writeDone() != nil {
					cancel()
				}
				return
			}
			if emitter.writeChunk(streamChunk(reqID, model, event, first)) != nil {
				cancel()
				return
			}
			first = false
		}
	}
}

func streamChunk(reqID, model string, event llm.StreamEvent, first bool) llm.ChatCompletionChunkResponse {
	var finishReason *string
	if event.FinishReason != "" {
		value := event.FinishReason
		finishReason = &value
	}
	chunk := llm.ChatCompletionChunkResponse{
		ID: reqID, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: model,
		Choices: []llm.ChunkChoice{{
			Index:        0,
			Delta:        llm.Delta{Content: event.DeltaContent, ToolCalls: event.ToolCalls},
			FinishReason: finishReason,
		}},
		Usage: event.Usage,
	}
	if first {
		chunk.Choices[0].Delta.Role = llm.RoleAssistant
	}
	return chunk
}

type sseEmitter struct {
	writer  io.Writer
	flusher http.Flusher
}

func (e sseEmitter) writeChunk(chunk llm.ChatCompletionChunkResponse) error {
	return e.writeData(chunk)
}

func (e sseEmitter) writeError(err error) error {
	if _, writeErr := fmt.Fprint(e.writer, "event: error\n"); writeErr != nil {
		return writeErr
	}
	if err := e.writeData(gatewayerrors.TranslateError(err)); err != nil {
		return err
	}
	return e.writeDone()
}

func (e sseEmitter) writeData(value any) error {
	if _, err := fmt.Fprint(e.writer, "data: "); err != nil {
		return err
	}
	if err := json.NewEncoder(e.writer).Encode(value); err != nil {
		return err
	}
	if _, err := fmt.Fprint(e.writer, "\n"); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}

func (e sseEmitter) writeDone() error {
	if _, err := fmt.Fprint(e.writer, "data: [DONE]\n\n"); err != nil {
		return err
	}
	e.flusher.Flush()
	return nil
}
