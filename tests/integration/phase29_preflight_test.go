//go:build phase29preflight

package integration

import (
	"context"
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

func newPhase29GeminiAdapter(env map[string]string) providers.EmbedAdapter {
	return gemini.NewAdapter(gemini.AdapterConfig{ID: "phase29-embedding", BaseURL: env["GEM_BASE_URL"], APIKey: env["GEM_PRIMARY_API_KEY"], ModelsCSV: env["GEM_EMBEDDING"]}).(providers.EmbedAdapter)
}

func TestPhase29SANSModels(t *testing.T) {
	env, err := godotenv.Read("../../.env.local")
	if err != nil {
		t.Fatal(err)
	}
	models := strings.Split(env["SANS_EMBEDDING"], ",")
	if len(models) != 2 || models[0] == models[1] {
		t.Fatal("two distinct configured SANS models required")
	}
	adapter := openai.NewAdapter("sans-primary", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], env["SANS_EMBEDDING"])
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			started := time.Now()
			response, err := adapter.Embed(ctx, &llm.EmbeddingRequest{Model: model, Input: []string{"What are the support hours?"}})
			if err != nil {
				t.Fatal(err)
			}
			if response == nil || len(response.Data) != 1 || len(response.Data[0].Embedding) == 0 {
				t.Fatal("invalid embedding response")
			}
			t.Logf("provider=sans-primary model=%s dimension=%d elapsed_ms=%d", model, len(response.Data[0].Embedding), time.Since(started).Milliseconds())
		})
	}
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
