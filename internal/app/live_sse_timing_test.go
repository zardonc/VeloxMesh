//go:build phase29preflight

package app

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

// Observe complete SSE lines without changing underlying read sizes or flushing.
type liveSSEBody struct {
	io.ReadCloser
	recorder *liveTiming
	id       string
	started  time.Time
	pending  []byte
	first    bool
}

func (body *liveSSEBody) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	const maxDiagnosticLineBytes = 1024 * 1024
	if len(body.pending)+n > maxDiagnosticLineBytes {
		body.pending = nil
		return n, err
	}
	body.pending = append(body.pending, buffer[:n]...)
	for {
		newline := bytes.IndexByte(body.pending, '\n')
		if newline < 0 {
			return n, err
		}
		body.observe(strings.TrimSpace(string(body.pending[:newline])))
		body.pending = body.pending[newline+1:]
	}
}

func (body *liveSSEBody) observe(line string) {
	if line == "data: [DONE]" {
		body.record("sse_done")
		return
	}
	if !strings.HasPrefix(line, "data: ") {
		return
	}
	var chunk llm.ChatCompletionChunkResponse
	if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &chunk) != nil {
		return
	}
	if chunk.Usage != nil {
		body.record("sse_usage")
	}
	for _, choice := range chunk.Choices {
		if choice.FinishReason != nil {
			body.record("sse_finish")
		}
		if !body.first && (choice.Delta.Content != "" || len(choice.Delta.ToolCalls) > 0) {
			body.first = true
			body.record("sse_first_meaningful")
		}
	}
}

func (body *liveSSEBody) record(name string) {
	body.recorder.RecordStage(observability.StageMeasurement{ID: body.id, Name: name, Started: body.started, Elapsed: time.Since(body.started)})
}
