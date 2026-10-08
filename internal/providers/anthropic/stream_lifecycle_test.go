package anthropic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

func TestStreamSDKFailurePreservesProviderClassification(t *testing.T) {
	for _, testCase := range []struct {
		name string
		err  error
		code string
	}{
		{"authentication", &anthropic.Error{StatusCode: http.StatusUnauthorized}, gatewayerrors.ProviderAuthError},
		{"rate limit", &anthropic.Error{StatusCode: http.StatusTooManyRequests}, gatewayerrors.ProviderRateLimit},
		{"request timeout", &anthropic.Error{StatusCode: http.StatusRequestTimeout}, gatewayerrors.ProviderTimeout},
		{"transport deadline", context.DeadlineExceeded, gatewayerrors.ProviderTimeout},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](nil, testCase.err)
			events := make(chan llm.StreamEvent)
			adapter := &Adapter{id: "test-provider"}
			go adapter.runStream(context.Background(), streamRun{stream: stream, events: events, model: "test-model"})
			output := collectAnthropicEvents(events)
			if len(output) != 1 {
				t.Fatalf("expected one terminal error, got %#v", output)
			}
			gatewayError, ok := output[0].Error.(*gatewayerrors.GatewayError)
			if !ok || gatewayError.Code != testCase.code || output[0].Done {
				t.Fatalf("error = %v, want %s without success", output[0].Error, testCase.code)
			}
		})
	}
}

func TestStreamClosesSDKBodyOnEveryExit(t *testing.T) {
	valid := anthropicStream(
		anthropicEvent("message_start", map[string]any{"message": map[string]any{"usage": map[string]any{"input_tokens": 1}}}),
		anthropicEvent("message_delta", map[string]any{"delta": map[string]any{"stop_reason": "end_turn"}}),
		anthropicEvent("message_stop", map[string]any{}),
	)
	for _, testCase := range []struct {
		name     string
		body     string
		bad      bool
		closeErr error
	}{
		{name: "complete", body: valid},
		{name: "invalid lifecycle", body: anthropicEvent("message_stop", map[string]any{}), bad: true},
		{name: "malformed SSE JSON", body: "event: message_start\ndata: {\n\n", bad: true},
		{name: "close failure", body: valid, bad: true, closeErr: io.ErrUnexpectedEOF},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := &countedStreamBody{Reader: strings.NewReader(testCase.body), closeErr: testCase.closeErr}
			decoder := ssestream.NewDecoder(&http.Response{Body: body, Header: http.Header{"Content-Type": {"text/event-stream"}}})
			stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](decoder, nil)
			events := make(chan llm.StreamEvent)
			adapter := &Adapter{id: "test-provider"}
			go adapter.runStream(context.Background(), streamRun{stream: stream, events: events})
			output := collectAnthropicEvents(events)
			if testCase.bad {
				assertClosedStreamFailure(t, output)
			} else if len(output) == 0 || !output[len(output)-1].Done {
				t.Fatal("valid stream did not finish")
			}
			if body.closed != 1 {
				t.Fatalf("stream body closed %d times, want exactly once", body.closed)
			}
		})
	}
}

func assertClosedStreamFailure(t *testing.T, events []llm.StreamEvent) {
	t.Helper()
	errorCount := 0
	for _, event := range events {
		if event.Done {
			t.Fatal("failed stream emitted Done")
		}
		if event.Error != nil {
			errorCount++
			if gatewayerrors.TranslateError(event.Error).Code != gatewayerrors.ProviderBadResponse {
				t.Fatalf("unexpected stream error: %v", event.Error)
			}
		}
	}
	if errorCount != 1 {
		t.Fatalf("expected exactly one terminal error, got %d", errorCount)
	}
}

type countedStreamBody struct {
	io.Reader
	closed   int
	closeErr error
}

func (body *countedStreamBody) Close() error {
	body.closed++
	return body.closeErr
}
