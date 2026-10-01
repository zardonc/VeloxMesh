package cache

import (
	"testing"
	"veloxmesh/internal/llm"
)

func TestSemanticCacheEmbeddingProviderChangesScope(t *testing.T) {
	first := boundsConfig()
	first.EmbeddingProvider = "provider-one"
	second := first
	second.EmbeddingProvider = "provider-two"
	profile := SemanticCacheUseCase{UseCaseID: "FAQ", KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ"}
	if cacheScope("key", profile, first) == cacheScope("key", profile, second) {
		t.Fatal("providers with the same model ID must not share vectors or answers")
	}
}

func TestSemanticCacheFreezesTrustedConfiguration(t *testing.T) {
	config := boundsConfig()
	config.UseCases = []SemanticCacheUseCase{{APIKeyIDs: []string{"key-one"}, UseCaseID: "FAQ", KnowledgeVersion: "v1", TargetModel: "model", SystemPrompt: "FAQ"}}
	service := NewSemanticCacheService(config, nil, nil, nil)
	t.Cleanup(service.Close)
	temperature, maxTokens := 0.0, 256
	request := &llm.LLMRequest{Model: "model", Temperature: &temperature, MaxTokens: &maxTokens, Messages: []llm.Message{{Role: llm.RoleSystem, Content: "FAQ"}, {Role: llm.RoleUser, Content: "question"}}}
	before, ok := service.Eligible("key-one", "user", request)
	if !ok {
		t.Fatal("fixture not eligible")
	}
	config.UseCases[0].KnowledgeVersion = "v2"
	config.UseCases[0].APIKeyIDs[0] = "key-two"
	after, ok := service.Eligible("key-one", "user", request)
	if !ok || before != after {
		t.Fatal("external mutation changed trusted cache identity")
	}
}
