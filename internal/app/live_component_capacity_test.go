//go:build phase29preflight

package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/storage"
)

const liveComponentCount = 500
const liveComponentDeadline = 2 * time.Second

type liveComponentWork struct {
	name string
	run  func(context.Context, int) error
}

func TestLiveRedisCapacity(t *testing.T) {
	liveEnvironment(t)
	client, err := hotstate.NewRedisClient(context.Background(), liveRedisAddress(), "", 0, fmt.Sprint("component-", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	data, err := json.Marshal(map[string]any{"provider": "component", "measured_at": time.Now(), "pending_requests": 0})
	if err != nil {
		t.Fatal(err)
	}
	liveComponentWindows(t, liveComponentWork{name: "redis_health_set_get", run: func(ctx context.Context, index int) error {
		key := fmt.Sprint("health-", index)
		if err := client.SetHealthSnapshot(ctx, key, data, time.Minute); err != nil {
			return err
		}
		value, err := client.GetHealthSnapshot(ctx, key)
		if err == nil && !bytes.Equal(value, data) {
			return fmt.Errorf("health snapshot mismatch")
		}
		return err
	}})
	liveComponentWindows(t, liveComponentWork{name: "redis_limiter_lua", run: func(ctx context.Context, index int) error {
		_, allowed, err := client.CheckAndIncrement(ctx, fmt.Sprint("limit-", index), 100, time.Minute)
		if err == nil && !allowed {
			return fmt.Errorf("unexpected limiter rejection")
		}
		return err
	}})
}

func TestLiveQdrantCapacity(t *testing.T) {
	liveEnvironment(t)
	vector := liveComponentVector(t)
	adapter, err := storage.NewQdrantVectorAdapter("127.0.0.1:6334", os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	collection := fmt.Sprint("component_capacity_", time.Now().UnixNano())
	if err := adapter.Insert(context.Background(), collection, [][]float32{vector}, []map[string]interface{}{{"id": "real-embedding-seed"}}); err != nil {
		t.Fatal(err)
	}
	liveComponentWindows(t, liveComponentWork{name: "qdrant_query", run: func(ctx context.Context, _ int) error {
		results, err := adapter.Search(ctx, collection, vector, 10)
		if err == nil && (len(results) == 0 || results[0]["score"].(float64) < 0.99) {
			return fmt.Errorf("real vector round-trip mismatch")
		}
		return err
	}})
	liveComponentWindows(t, liveComponentWork{name: "qdrant_insert_confirmed", run: func(ctx context.Context, index int) error {
		return adapter.Insert(ctx, collection, [][]float32{vector}, []map[string]interface{}{{"id": fmt.Sprint("write-", index)}})
	}})
}

func TestLiveSQLiteCapacity(t *testing.T) {
	env := liveEnvironment(t)
	repo, _, _ := liveRepository(t)
	liveRepositoryCapacity(t, repo.SemanticCache(), liveComponentEntry(t, env))
}

func TestLivePostgresCapacity(t *testing.T) {
	env := liveEnvironment(t)
	dsn := env["POSTGRES_TEST_DSN"]
	if dsn == "" {
		t.Fatal("explicit isolated PostgreSQL DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveRecoveryWait)
	defer cancel()
	repo, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal("open isolated PostgreSQL component")
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	liveRepositoryCapacity(t, repo.SemanticCache(), liveComponentEntry(t, env))
}

func liveComponentVector(t *testing.T) []float32 {
	adapter := openai.NewAdapter("component-embedding", os.Getenv("PHASE29_EMBEDDING_BASE_URL"), os.Getenv("PHASE29_EMBEDDING_API_KEY"), os.Getenv("PHASE29_MODEL"))
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	response, err := adapter.Embed(ctx, &llm.EmbeddingRequest{Model: os.Getenv("PHASE29_MODEL"), Input: []string{"How long is the trial for plan 101?"}})
	if err != nil || response == nil || len(response.Data) != 1 {
		t.Fatalf("real component embedding: %v", err)
	}
	return response.Data[0].Embedding
}

func liveComponentEntry(t *testing.T, env map[string]string) controlstate.SemanticCacheEntry {
	vector := liveComponentVector(t)
	adapter := openai.NewAdapter("component-chat", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], env["SANS_PRIMARY_DEFAULT_MODEL"])
	payload := liveFAQPayload(env["SANS_PRIMARY_DEFAULT_MODEL"], "How long is the trial for plan 101?")
	request := &llm.LLMRequest{Model: payload.Model, Messages: payload.Messages, Temperature: payload.Temperature, MaxTokens: payload.MaxTokens}
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	response, err := adapter.Complete(ctx, request)
	if err != nil || response == nil || len(response.Choices) == 0 {
		t.Fatalf("real component response: %v", err)
	}
	encoded, err := json.Marshal(response.Choices)
	if err != nil {
		t.Fatal(err)
	}
	return controlstate.SemanticCacheEntry{Scope: fmt.Sprint("capacity-", time.Now().UnixNano()), Model: request.Model, Vector: liveEncodeVector(vector), Response: string(encoded), Enabled: true, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
}

func liveEncodeVector(vector []float32) []byte {
	encoded := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(encoded[index*4:], math.Float32bits(value))
	}
	return encoded
}

func liveRepositoryCapacity(t *testing.T, repo controlstate.SemanticCacheRepository, source controlstate.SemanticCacheEntry) {
	liveComponentWindows(t, liveComponentWork{name: "repository_cache_store_read", run: func(ctx context.Context, index int) error {
		entry := source
		entry.ID = fmt.Sprintf("%s-%d", source.Scope, index)
		if err := repo.Store(ctx, &entry); err != nil {
			return err
		}
		read, err := repo.GetCandidate(ctx, entry.ID, entry.Scope, entry.Model)
		if err == nil && (read == nil || read.Response != source.Response || !bytes.Equal(read.Vector, source.Vector)) {
			return fmt.Errorf("repository round-trip mismatch")
		}
		return err
	}})
}

func liveComponentWindows(t *testing.T, work liveComponentWork) {
	for _, concurrency := range []int{1, 4, 8, 16} {
		liveComponentWindow(t, work, concurrency)
	}
}

func liveComponentWindow(t *testing.T, work liveComponentWork, concurrency int) {
	samples := make([]liveSample, liveComponentCount)
	sem, started := make(chan struct{}, concurrency), time.Now()
	var group sync.WaitGroup
	for index := range liveComponentCount {
		sem <- struct{}{}
		group.Add(1)
		go func(index int) {
			defer group.Done()
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), liveComponentDeadline)
			defer cancel()
			begin := time.Now()
			err := work.run(ctx, index)
			sample := liveSample{Type: "component_client", Name: work.name, ElapsedMS: float64(time.Since(begin).Microseconds()) / 1000, Concurrent: int64(concurrency), OK: err == nil}
			if err != nil {
				sample.Error = err.Error()
			}
			samples[index] = sample
		}(index)
	}
	group.Wait()
	shipLogJSON(t, map[string]any{"type": "component_window", "name": work.name, "concurrency": concurrency, "count": liveComponentCount, "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000, "client_location": "VM loopback"})
	failures := 0
	for _, sample := range samples {
		shipLogJSON(t, sample)
		if !sample.OK {
			failures++
		}
	}
	if failures > 0 {
		t.Errorf("real component %s C%d failures=%d", work.name, concurrency, failures)
	}
}
