package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/app"
	"veloxmesh/internal/llm"
)

func TestChatCompletionsStreamingEndpointCompatibility(t *testing.T) {
	p1 := setupFakeProvider(t, "p1", 0, http.StatusOK)
	defer p1.Close()
	p2 := setupFakeProvider(t, "p2", 0, http.StatusOK)
	defer p2.Close()
	application := newStreamingIntegrationApp(t, p1, p2)

	rec := serveStreamingRequest(t, application, context.Background())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Stream from p") || bytes.Count(rec.Body.Bytes(), []byte("data: [DONE]")) != 1 {
		t.Fatalf("unexpected streaming response: %s", body)
	}
}

func TestChatCompletionsStreamingCancellationStopsTerminalOutput(t *testing.T) {
	started := make(chan struct{})
	upstreamCancelled := make(chan struct{})
	p1 := blockingStreamProvider(started, upstreamCancelled)
	defer p1.Close()
	application := newStreamingIntegrationApp(t, p1, p1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := streamingRequest(t, application).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		defer close(done)
		application.Router.ServeHTTP(rec, req)
	}()

	<-started
	cancel()
	<-done
	select {
	case <-upstreamCancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not observe cancellation")
	}
	if strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("cancelled request emitted done frame: %s", rec.Body.String())
	}
}

func TestChatCompletionsStreamingRequiresAuthorization(t *testing.T) {
	p1 := setupFakeProvider(t, "p1", 0, http.StatusOK)
	defer p1.Close()
	application := newStreamingIntegrationApp(t, p1, p1)
	requestBody, err := json.Marshal(llm.ChatCompletionRequest{
		Model: "gpt-4o", Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestBody))
	rec := httptest.NewRecorder()

	application.Router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "data: [DONE]") {
		t.Fatalf("unexpected unauthorized stream response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func newStreamingIntegrationApp(t *testing.T, p1, p2 *httptest.Server) *app.App {
	t.Helper()
	cfgPath := writeConfig(t, p1, p2, "round-robin")
	t.Cleanup(func() { _ = os.Remove(cfgPath) })
	os.Setenv("CONFIG_FILE", cfgPath)
	t.Cleanup(func() { os.Unsetenv("CONFIG_FILE") })
	application, err := app.New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return application
}

func serveStreamingRequest(t *testing.T, application *app.App, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	application.Router.ServeHTTP(rec, streamingRequest(t, application).WithContext(ctx))
	return rec
}

func streamingRequest(t *testing.T, application *app.App) *http.Request {
	t.Helper()
	body, err := json.Marshal(llm.ChatCompletionRequest{
		Model: "gpt-4o", Stream: true, Messages: []llm.Message{{Role: llm.RoleUser, Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
	return req
}

func blockingStreamProvider(started, cancelled chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-request.Context().Done()
		close(cancelled)
	}))
}
