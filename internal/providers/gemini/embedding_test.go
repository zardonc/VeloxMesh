package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

func TestAdapterEmbeddingBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/models/test-embedding:batchEmbedContents") {
			t.Errorf("unexpected embedding path: %s", r.URL.Path)
		}
		var body struct {
			Requests []json.RawMessage `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Requests) != 2 {
			t.Errorf("batch size=%d error=%v", len(body.Requests), err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[1,0]},{"values":[0,1]}]}`))
	}))
	defer server.Close()
	adapter, ok := NewAdapter(AdapterConfig{ID: "test", BaseURL: server.URL, APIKey: "test", ModelsCSV: "gemini/test-embedding"}).(providers.EmbedAdapter)
	if !ok {
		t.Fatal("native Gemini adapter must implement EmbedAdapter")
	}
	result, err := adapter.Embed(context.Background(), &llm.EmbeddingRequest{Model: "gemini/test-embedding", Input: []string{"first", "second"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "gemini/test-embedding" || len(result.Data) != 2 || result.Data[1].Index != 1 || result.Data[1].Embedding[1] != 1 {
		t.Fatalf("unexpected batch result: %+v", result)
	}
}

func TestAdapterEmbeddingFailures(t *testing.T) {
	cases := []struct {
		name, response string
		status         int
		input          *llm.EmbeddingRequest
	}{
		{"nil request", "", http.StatusOK, nil},
		{"empty input", "", http.StatusOK, &llm.EmbeddingRequest{}},
		{"blank input", "", http.StatusOK, &llm.EmbeddingRequest{Input: []string{""}}},
		{"empty vector", `{"embeddings":[{"values":[]}]}`, http.StatusOK, &llm.EmbeddingRequest{Input: []string{"text"}}},
		{"missing result", `{"embeddings":[]}`, http.StatusOK, &llm.EmbeddingRequest{Input: []string{"text"}}},
		{"rate limit", `{"error":{"code":429,"message":"quota","status":"RESOURCE_EXHAUSTED"}}`, http.StatusTooManyRequests, &llm.EmbeddingRequest{Input: []string{"text"}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()
			adapter, ok := NewAdapter(AdapterConfig{ID: "test", BaseURL: server.URL, APIKey: "test", ModelsCSV: "test-embedding"}).(providers.EmbedAdapter)
			if !ok {
				t.Fatal("native Gemini adapter must implement EmbedAdapter")
			}
			if _, err := adapter.Embed(context.Background(), test.input); err == nil {
				t.Fatal("expected embedding failure")
			}
		})
	}
}
