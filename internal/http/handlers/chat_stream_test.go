package handlers

import (
	"bytes"
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	gatewayerrors "veloxmesh/internal/errors"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/routing"
)

type terminalStreamAdapter struct {
	captureChatAdapter
	events []llm.StreamEvent
}

func (a *terminalStreamAdapter) Stream(context.Context, *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, len(a.events))
	for _, event := range a.events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

type cancellationAwareStreamAdapter struct {
	captureChatAdapter
	cancelled chan struct{}
	once      sync.Once
}

func (a *cancellationAwareStreamAdapter) Stream(ctx context.Context, _ *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 1)
	ch <- llm.StreamEvent{DeltaContent: "first"}
	go func() {
		<-ctx.Done()
		a.once.Do(func() { close(a.cancelled) })
		close(ch)
	}()
	return ch, nil
}

type pendingStreamAdapter struct {
	captureChatAdapter
	started   chan struct{}
	cancelled chan struct{}
	once      sync.Once
}

func (a *pendingStreamAdapter) Stream(ctx context.Context, _ *llm.LLMRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	a.once.Do(func() { close(a.started) })
	go func() {
		<-ctx.Done()
		close(a.cancelled)
		close(ch)
	}()
	return ch, nil
}

type failingStreamWriter struct {
	header   http.Header
	failAt   int
	writes   int
	status   int
	contents bytes.Buffer
}

func (w *failingStreamWriter) Header() http.Header { return w.header }

func (w *failingStreamWriter) WriteHeader(status int) { w.status = status }

func (w *failingStreamWriter) Write(value []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, stderrors.New("client disconnected")
	}
	return w.contents.Write(value)
}

func (w *failingStreamWriter) Flush() {}

func newStreamHandler(adapter providers.ProviderAdapter) http.Handler {
	store := health.NewInMemoryStore()
	store.EnsureProvider(adapter.ID(), 3, 1)
	registry := providers.NewRegistry(&config.Config{}, []providers.ProviderAdapter{adapter}, nil)
	router := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	service := gateway.NewService(router, admission.NewPassThroughController(), store, false, 1, nil, nil, pipeline.DefaultRegistry(), nil, nil)
	return middleware.RequestID(http.HandlerFunc(NewChatHandler(service).ChatCompletions))
}

func newStreamRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set(middleware.RequestIDHeader, "req-stream")
	return req
}

func TestChatCompletionsStreamWritesOneDoneAfterCompletedStream(t *testing.T) {
	adapter := &terminalStreamAdapter{events: []llm.StreamEvent{{DeltaContent: "ok"}, {Done: true}, {Done: true}}}
	rec := httptest.NewRecorder()

	newStreamHandler(adapter).ServeHTTP(rec, newStreamRequest())

	if got := bytes.Count(rec.Body.Bytes(), []byte("data: [DONE]")); got != 1 {
		t.Fatalf("expected one done frame, got %d: %s", got, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"role":"assistant"`)) {
		t.Fatalf("first chunk did not set assistant role: %s", rec.Body.String())
	}
}

func TestChatCompletionsStreamWritesErrorThenDoneForProviderFailure(t *testing.T) {
	providerErr := gatewayerrors.NewGatewayError("provider_unavailable", "provider failed", http.StatusBadGateway)
	adapter := &terminalStreamAdapter{events: []llm.StreamEvent{{Error: providerErr}, {Done: true}}}
	rec := httptest.NewRecorder()

	newStreamHandler(adapter).ServeHTTP(rec, newStreamRequest())

	body := rec.Body.String()
	if !bytes.Contains([]byte(body), []byte("event: error")) || bytes.Count([]byte(body), []byte("data: [DONE]")) != 1 {
		t.Fatalf("expected one error and done sequence: %s", body)
	}
}

func TestChatCompletionsStreamWritesChunkBeforeProviderError(t *testing.T) {
	providerErr := gatewayerrors.NewGatewayError("provider_unavailable", "provider failed", http.StatusBadGateway)
	adapter := &terminalStreamAdapter{events: []llm.StreamEvent{{DeltaContent: "partial"}, {Error: providerErr}}}
	rec := httptest.NewRecorder()

	newStreamHandler(adapter).ServeHTTP(rec, newStreamRequest())

	body := rec.Body.String()
	chunkIndex := bytes.Index([]byte(body), []byte(`"content":"partial"`))
	errorIndex := bytes.Index([]byte(body), []byte("event: error"))
	if chunkIndex < 0 || errorIndex <= chunkIndex || bytes.Count([]byte(body), []byte("data: [DONE]")) != 1 {
		t.Fatalf("expected chunk followed by one error terminal: %s", body)
	}
}

func TestChatCompletionsStreamCancelsRequestAfterWriteFailure(t *testing.T) {
	adapter := &cancellationAwareStreamAdapter{cancelled: make(chan struct{})}
	writer := &failingStreamWriter{header: make(http.Header), failAt: 1}

	newStreamHandler(adapter).ServeHTTP(writer, newStreamRequest())

	select {
	case <-adapter.cancelled:
	case <-time.After(time.Second):
		t.Fatal("stream context was not cancelled after write failure")
	}
	if writer.writes != 1 || writer.contents.Len() != 0 {
		t.Fatalf("handler wrote after client failure: writes=%d body=%q", writer.writes, writer.contents.String())
	}
}

func TestChatCompletionsStreamStopsAfterTerminalWriteFailure(t *testing.T) {
	providerErr := gatewayerrors.NewGatewayError("provider_unavailable", "provider failed", http.StatusBadGateway)
	adapter := &terminalStreamAdapter{events: []llm.StreamEvent{{Error: providerErr}}}
	writer := &failingStreamWriter{header: make(http.Header), failAt: 4}

	newStreamHandler(adapter).ServeHTTP(writer, newStreamRequest())

	if writer.writes != 4 || bytes.Contains(writer.contents.Bytes(), []byte("data: [DONE]")) {
		t.Fatalf("terminal write failure was followed by output: writes=%d body=%q", writer.writes, writer.contents.String())
	}
}

func TestChatCompletionsStreamStopsAfterRequestCancellation(t *testing.T) {
	adapter := &pendingStreamAdapter{started: make(chan struct{}), cancelled: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := newStreamRequest().WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		newStreamHandler(adapter).ServeHTTP(rec, req)
	}()

	<-adapter.started
	cancel()
	<-done
	<-adapter.cancelled
	if rec.Body.Len() != 0 {
		t.Fatalf("request cancellation wrote stream bytes: %s", rec.Body.String())
	}
}

func TestToolStreamImmediate(t *testing.T) {
	index, callID, toolType, name, arguments := 0, "call-1", llm.ToolTypeFunction, "lookup", `{"city":"Paris"}`
	adapter := &terminalStreamAdapter{events: []llm.StreamEvent{
		{ToolCalls: []llm.ToolCallChunk{{Index: &index, ID: &callID, Type: &toolType, Function: &llm.FunctionCallChunk{Name: &name, Arguments: &arguments}}}},
		{FinishReason: "tool_calls"},
		{Done: true},
	}}
	recorder := httptest.NewRecorder()
	newStreamHandler(adapter).ServeHTTP(recorder, newToolStreamRequest())

	body := recorder.Body.String()
	toolIndex := bytes.Index([]byte(body), []byte(`"tool_calls"`))
	finishIndex := bytes.Index([]byte(body), []byte(`"finish_reason":"tool_calls"`))
	if toolIndex < 0 || finishIndex <= toolIndex || bytes.Count([]byte(body), []byte("data: [DONE]")) != 1 {
		t.Fatalf("tool SSE sequence=%s", body)
	}
}

func newToolStreamRequest() *http.Request {
	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"lookup"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":"auto"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set(middleware.RequestIDHeader, "req-tool-stream")
	return req
}
