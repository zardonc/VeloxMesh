package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"veloxmesh/internal/config"
)

func TestSemanticCacheDurableStartup(t *testing.T) {
	setSemanticNeighborAppEnv(t, "127.0.0.1:1")
	t.Setenv("SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED", "false")
	t.Setenv("REDIS_ENABLED", "false")
	t.Setenv("SEMANTIC_CACHE_VECTOR_STORE", "")
	cacheConfig := config.CacheConfig{
		Enabled: true, Provider: "openai-primary", EmbeddingModel: "embedding-fixture",
		VectorDimension: 3, TTL: "1h", Threshold: 0.99, MaxCandidates: 10,
		ReadTimeout: "100ms", ReadConcurrency: 4, WriteTimeout: "2s", WriteWorkers: 2, QueueCapacity: 32, ShutdownGrace: "1s",
		UseCases: []config.CacheUseCaseConfig{{ReuseMode: "semantic", UseCaseID: "phase29-static-faq",
			APIKeyIDs: []string{"startup-fixture-key"}, KnowledgeVersion: "faq-v1",
			TargetModel: "gpt-4o-mini", SystemPrompt: "Static test FAQ."}},
	}
	path := filepath.Join(t.TempDir(), "cache.json")
	body, err := json.Marshal(cacheConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CACHE_CONFIG_FILE", path)
	application, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	if application.semanticCache == nil {
		t.Fatal("durable provider registry must be activated before cache initialization")
	}
}
