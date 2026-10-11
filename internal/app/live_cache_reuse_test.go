//go:build phase29preflight

package app

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/storage"
)

func TestLiveCacheMissEmbeddingReuse(t *testing.T) {
	chain := newLiveChain(t)
	// Empty scopes intentionally skip lookup; seed a candidate to exercise reuse.
	liveDecodeChat(t, liveHTTP(t, chain, liveFAQPayload(chain.model, "What is a database transaction?")))
	liveWaitStoreCount(t, chain.timing, 1)
	temperature, limit := 0.0, 256
	request := llm.ChatCompletionRequest{Model: chain.model, Temperature: &temperature, MaxTokens: &limit,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveFAQSystem}, {Role: llm.RoleUser, Content: "How long is the trial for plan 101?"}}}
	response := liveHTTP(t, chain, request)
	id := response.Header.Get("X-Request-ID")
	if response.Header.Get("X-Cache-Hit") == "true" {
		t.Fatal("fresh request unexpectedly hit cache")
	}
	first := liveDecodeChat(t, response)
	chain.app.semanticCache.Close()
	liveAssertSingleEmbedding(t, chain.timing, id)
	hit := liveHTTP(t, chain, request)
	if hit.Header.Get("X-Cache-Hit") != "true" {
		t.Fatal("real asynchronously stored response was not retrieved")
	}
	second := liveDecodeChat(t, hit)
	if first.Choices[0].Message.Content != second.Choices[0].Message.Content || second.Usage.TotalTokens != 0 {
		t.Fatal("cache changed response or charged cached tokens")
	}
	liveAssertSettlement(t, chain, 2)
}

func liveAssertSingleEmbedding(t *testing.T, recorder *liveTiming, id string) {
	t.Helper()
	counts := map[string]int{}
	recorder.mu.Lock()
	for _, stage := range recorder.stages {
		if stage.ID == id {
			counts[stage.Name]++
		}
	}
	recorder.mu.Unlock()
	shipLogJSON(t, map[string]any{"type": "embedding_reuse", "request_id": id, "counts": counts})
	if counts["embedding_http"] != 1 || counts["lookup_embedding"] != 1 || counts["store_embedding"] != 0 || counts["vector_insert"] != 1 {
		t.Fatalf("cache miss failed to reuse real embedding: %v", counts)
	}
}

func TestLiveQdrantConcurrentCollection(t *testing.T) {
	liveEnvironment(t)
	adapter, err := storage.NewQdrantVectorAdapter("127.0.0.1:6334", os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	const workers, rounds, dimension = 16, 4, 3
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for round := 0; round < rounds; round++ {
		collection := fmt.Sprintf("reuse_race_%d_%d", time.Now().UnixNano(), round)
		liveConcurrentEnsure(t, adapter, liveEnsureOptions{ctx: ctx, collection: collection})
		if err := adapter.EnsureCollection(ctx, collection, dimension+1); err == nil {
			t.Fatal("existing collection with wrong dimension was accepted")
		}
	}
	t.Logf("real Qdrant cold collections=%d concurrent creators=%d schema mismatch rejected", rounds, workers)
}

type liveEnsureOptions struct {
	ctx        context.Context
	collection string
}

func liveConcurrentEnsure(t *testing.T, adapter *storage.QdrantVectorAdapter, options liveEnsureOptions) {
	t.Helper()
	const workers, dimension = 16, 3
	start, failures := make(chan struct{}), make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			failures <- adapter.EnsureCollection(options.ctx, options.collection, dimension)
		}()
	}
	close(start)
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Errorf("real concurrent collection creation: %v", err)
		}
	}
}
