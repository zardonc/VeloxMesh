//go:build phase29preflight

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/gemini"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/storage"
)

type phase29GeminiAdapter struct {
	providers.ProviderAdapter
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newPhase29GeminiAdapter(env map[string]string) *phase29GeminiAdapter {
	return &phase29GeminiAdapter{
		ProviderAdapter: gemini.NewAdapter(gemini.AdapterConfig{ID: "phase29-embedding", BaseURL: env["GEM_BASE_URL"], APIKey: env["GEM_PRIMARY_API_KEY"], ModelsCSV: env["GEM_EMBEDDING"]}),
		baseURL:         env["GEM_BASE_URL"], apiKey: env["GEM_PRIMARY_API_KEY"], model: env["GEM_EMBEDDING"], client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *phase29GeminiAdapter) Embed(ctx context.Context, input *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	if input == nil || len(input.Input) != 1 || input.Model != a.model {
		return nil, fmt.Errorf("phase29 embedding requires one input and configured model")
	}
	model := strings.TrimPrefix(a.model, "gemini/")
	body, err := json.Marshal(map[string]any{"content": map[string]any{"parts": []map[string]string{{"text": input.Input[0]}}}})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(a.baseURL, "/") + "/v1beta/models/" + model + ":embedContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", a.apiKey)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&payload); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || len(payload.Embedding.Values) == 0 {
		return nil, fmt.Errorf("gemini embedding status=%d code=%d message=%q", resp.StatusCode, payload.Error.Code, payload.Error.Message)
	}
	return &llm.EmbeddingResponse{Model: a.model, Data: []llm.Embedding{{Index: 0, Embedding: payload.Embedding.Values}}}, nil
}

func TestPhase29ProviderSmoke(t *testing.T) {
	if os.Getenv("PHASE29_QDRANT_API_KEY") == "" {
		t.Fatal("PHASE29_QDRANT_API_KEY is required for authenticated Qdrant checks")
	}
	if os.Getenv("PHASE29_QDRANT_ADDR") == "" {
		t.Fatal("PHASE29_QDRANT_ADDR is required")
	}
	env, err := godotenv.Read("../../.env.local")
	if err != nil {
		t.Fatal(err)
	}
	primary := openai.NewAdapter("phase29-primary", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], env["SANS_PRIMARY_DEFAULT_MODEL"])
	embed := newPhase29GeminiAdapter(env)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	vector, err := storage.NewQdrantVectorAdapter(os.Getenv("PHASE29_QDRANT_ADDR"), os.Getenv("PHASE29_QDRANT_API_KEY"))
	if err != nil {
		t.Fatalf("qdrant: %v", err)
	}
	if err := vector.Ping(ctx); err != nil {
		t.Fatalf("qdrant ping: %v", err)
	}
	temperature, maxTokens := 0.0, 256
	started := time.Now()
	response, err := primary.Complete(ctx, &llm.LLMRequest{Model: env["SANS_PRIMARY_DEFAULT_MODEL"], Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: "Answer static FAQ questions in one sentence."}, {Role: llm.RoleUser, Content: "What are the support hours?"}}})
	if err != nil || response == nil || len(response.Choices) == 0 {
		t.Fatalf("primary: %v", err)
	}
	t.Logf("primary_model=%s full_call_ms=%d", env["SANS_PRIMARY_DEFAULT_MODEL"], time.Since(started).Milliseconds())
	started = time.Now()
	embedding, err := embed.Embed(ctx, &llm.EmbeddingRequest{Model: env["GEM_EMBEDDING"], Input: []string{"What are the support hours?"}})
	if err != nil || embedding == nil || len(embedding.Data) != 1 {
		t.Fatalf("embedding: %v", err)
	}
	t.Logf("embedding_config_id=%s dimension=%d full_call_ms=%d", env["GEM_EMBEDDING"], len(embedding.Data[0].Embedding), time.Since(started).Milliseconds())
}
