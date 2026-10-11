package gemini

// Failure cases: duplicate terminal; content after terminal; truncated body after
// terminal; cancellation after native finish but before clean EOF. Usage-only
// trailers must remain legal, and no failed stream may emit a successful Done.
import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayErr "veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
)

const boundaryTerminal = `{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`
const boundaryContent = `{"candidates":[{"content":{"role":"model","parts":[{"text":"late"}]}}]}`
const boundaryUsage = `{"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`

type boundaryCase struct {
	tail        string
	abort       bool
	cancel      bool
	expectError bool
}

type boundaryResult struct {
	done    int
	failure error
}

func TestGeminiStreamTerminalBoundaries(t *testing.T) {
	cases := map[string]boundaryCase{
		"clean":                          {},
		"usage trailer":                  {tail: boundaryUsage},
		"content after terminal":         {tail: boundaryContent, expectError: true},
		"duplicate terminal":             {tail: boundaryTerminal, expectError: true},
		"truncated after terminal":       {abort: true, expectError: true},
		"cancel after finish before EOF": {cancel: true, expectError: true},
	}
	for name, scenario := range cases {
		t.Run(name, func(t *testing.T) { checkStreamBoundary(t, scenario) })
	}
}

func checkStreamBoundary(t *testing.T, scenario boundaryCase) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\n", boundaryTerminal)
		w.(http.Flusher).Flush()
		if scenario.cancel {
			<-r.Context().Done()
			return
		}
		if scenario.abort {
			panic(http.ErrAbortHandler)
		}
		if scenario.tail != "" {
			fmt.Fprintf(w, "data: %s\n\n", scenario.tail)
		}
	}))
	defer server.Close()
	adapter := NewAdapter(AdapterConfig{ID: "boundary", BaseURL: server.URL, APIKey: "test-key", ModelsCSV: "model"}).(*Adapter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events, err := adapter.Stream(ctx, &llm.LLMRequest{Model: "model", Messages: []llm.Message{{Role: llm.RoleUser, Content: "greeting"}}})
	if err != nil {
		t.Fatal(err)
	}
	var done int
	var failure error
	for event := range events {
		if event.Done {
			done++
		}
		if event.Error != nil {
			failure = event.Error
		}
		if scenario.cancel && event.FinishReason != "" {
			cancel()
		}
	}
	assertStreamBoundary(t, scenario, boundaryResult{done: done, failure: failure})
}

func assertStreamBoundary(t *testing.T, scenario boundaryCase, result boundaryResult) {
	t.Helper()
	done, failure := result.done, result.failure
	if !scenario.expectError {
		if failure != nil || done != 1 {
			t.Fatalf("clean stream: done=%d error=%v", done, failure)
		}
		return
	}
	if failure == nil || done != 0 {
		t.Fatalf("failed stream: done=%d error=%v", done, failure)
	}
	if scenario.cancel {
		if !errors.Is(failure, context.Canceled) {
			t.Fatalf("cancel classification: %v", failure)
		}
		return
	}
	var problem *gatewayErr.GatewayError
	if !errors.As(failure, &problem) || problem.Code != gatewayErr.ProviderBadResponse {
		t.Fatalf("protocol classification: %v", failure)
	}
}
