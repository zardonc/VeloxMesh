package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/testenv"
)

func TestQdrantEnsureCollectionCreatesRealCollection(t *testing.T) {
	adapter, ctx := liveQdrantAdapter(t)
	collection := "scheduler_training_samples_test_" + time.Now().UTC().Format("20060102150405000000000")
	defer func() { _ = adapter.client.DeleteCollection(context.Background(), collection) }()

	if err := adapter.EnsureCollection(ctx, collection, 3); err != nil {
		t.Fatalf("ensure qdrant collection: %v", err)
	}
	exists, err := adapter.client.CollectionExists(ctx, collection)
	if err != nil {
		t.Fatalf("check qdrant collection: %v", err)
	}
	if !exists {
		t.Fatalf("expected qdrant collection %q to exist", collection)
	}
}

func TestQdrantInsertReusesEnsureCollection(t *testing.T) {
	adapter, ctx := liveQdrantAdapter(t)
	collection := "scheduler_training_samples_insert_test_" + time.Now().UTC().Format("20060102150405000000000")
	defer func() { _ = adapter.client.DeleteCollection(context.Background(), collection) }()

	err := adapter.Insert(ctx, collection, [][]float32{{1, 0, 0}}, []map[string]interface{}{{"sample_id": "sample-1"}})
	if err != nil {
		t.Fatalf("insert qdrant vector: %v", err)
	}
	exists, err := adapter.client.CollectionExists(ctx, collection)
	if err != nil {
		t.Fatalf("check qdrant collection: %v", err)
	}
	if !exists {
		t.Fatalf("expected qdrant insert to create %q", collection)
	}
}

func TestQdrantEnsureCollectionRejectsInvalidDimension(t *testing.T) {
	adapter := &QdrantVectorAdapter{}
	if err := adapter.EnsureCollection(context.Background(), "scheduler_training_samples", 0); err == nil {
		t.Fatalf("expected invalid dimension error")
	}
}

func liveQdrantAdapter(t *testing.T) (*QdrantVectorAdapter, context.Context) {
	t.Helper()
	testenv.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	addr := os.Getenv("QDRANT_ADDR")
	if addr == "" {
		t.Fatalf("QDRANT_ADDR is required for real qdrant tests")
	}
	adapter, err := NewQdrantVectorAdapter(addr, os.Getenv("QDRANT_API_KEY"))
	if err != nil {
		t.Fatalf("new qdrant adapter: %v", err)
	}
	return adapter, ctx
}
