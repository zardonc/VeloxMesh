//go:build phase29preflight

package app

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

func TestLiveEmbeddingModelSwitch(t *testing.T) {
	env := liveEnvironment(t)
	first, second := os.Getenv("PHASE29_MODEL"), env["PHASE29_SECOND_EMBEDDING_MODEL"]
	if second == "" || second == first {
		t.Fatal("two distinct real embedding model identifiers required")
	}
	t.Setenv("PHASE29_MODE", "on")
	repo, dsn, keyID := liveRepository(t)
	database := liveDatabase{dsn: dsn, keyID: keyID}
	var scopes []string
	for index, model := range []string{first, second, first} {
		t.Setenv("PHASE29_MODEL", model)
		application, _ := liveApplicationWithDatabase(t, env, database)
		recorder := installLiveTiming(t, application, time.Now())
		server := httptest.NewServer(application.Router)
		chain := liveChain{app: application, repo: repo, url: server.URL, token: keyID, model: env["SANS_PRIMARY_DEFAULT_MODEL"], timing: recorder}
		scopes = append(scopes, liveEmbeddingLifecycle(t, chain, index == 2))
		server.Close()
		application.Close()
		recorder.dump(t)
	}
	if scopes[0] == scopes[1] || scopes[0] != scopes[2] {
		t.Fatal("embedding model scopes did not isolate and restore")
	}
	if count := liveUsageCount(t, liveChain{repo: repo, token: keyID}); count != 2 {
		t.Fatalf("model switch billed %d upstream requests, expected two seeds", count)
	}
	shipLogJSON(t, map[string]any{"type": "embedding_model_switch", "models": []string{first, second}, "separate_scopes": true, "restored_old_hit": true})
}

func liveEmbeddingLifecycle(t *testing.T, chain liveChain, expectHit bool) string {
	t.Helper()
	request := liveFAQPayload(chain.model, "How long is the trial for plan 101?")
	response := liveHTTP(t, chain, request)
	if (response.Header.Get("X-Cache-Hit") == "true") != expectHit {
		t.Fatalf("model switch first response expected hit=%v", expectHit)
	}
	seed := liveDecodeChat(t, response)
	chain.app.semanticCache.Close()
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("real embedding model failed write-to-hit lifecycle")
	}
	cached := liveDecodeChat(t, hit)
	if cached.Choices[0].Message.Content != seed.Choices[0].Message.Content || cached.Usage.TotalTokens != 0 {
		t.Fatal("embedding model lifecycle changed answer or cached billing")
	}
	scope, eligible := chain.app.semanticCache.Eligible(chain.token, "user", &llm.LLMRequest{Model: request.Model,
		Temperature: request.Temperature, MaxTokens: request.MaxTokens, Messages: request.Messages})
	if !eligible {
		t.Fatal("model switch request ineligible")
	}
	return scope
}
