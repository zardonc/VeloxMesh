package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/testenv"
)

func TestQdrantClientConfigInfersTLSFromAddress(t *testing.T) {
	tests := []struct {
		name   string
		addr   string
		host   string
		port   int
		useTLS bool
	}{
		{name: "host with port defaults to plaintext", addr: "qdrant.local:6334", host: "qdrant.local", port: 6334},
		{name: "host without port defaults to grpc plaintext", addr: "qdrant.local", host: "qdrant.local", port: 6334},
		{name: "http scheme is plaintext", addr: "http://qdrant.local:6334", host: "qdrant.local", port: 6334},
		{name: "https scheme enables tls", addr: "https://qdrant.local:6334", host: "qdrant.local", port: 6334, useTLS: true},
		{name: "https without port uses default grpc port", addr: "https://qdrant.local", host: "qdrant.local", port: 6334, useTLS: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := qdrantClientConfig(tt.addr, "test-key")
			if err != nil {
				t.Fatalf("qdrant client config: %v", err)
			}
			if cfg.Host != tt.host || cfg.Port != tt.port || cfg.UseTLS != tt.useTLS {
				t.Fatalf("got host=%q port=%d useTLS=%v", cfg.Host, cfg.Port, cfg.UseTLS)
			}
		})
	}
}

func TestQdrantClientConfigRejectsInvalidAddress(t *testing.T) {
	for _, addr := range []string{"grpc://qdrant.local:6334", "https://:6334", "qdrant.local:not-a-port"} {
		if _, err := qdrantClientConfig(addr, "test-key"); err == nil {
			t.Fatalf("expected invalid qdrant addr error for %q", addr)
		}
	}
}

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

func TestQdrantDeleteRemovesPayloadFilteredPoints(t *testing.T) {
	adapter, ctx := liveQdrantAdapter(t)
	collection := "scheduler_training_samples_delete_test_" + time.Now().UTC().Format("20060102150405000000000")
	defer func() { _ = adapter.client.DeleteCollection(context.Background(), collection) }()

	err := adapter.Insert(ctx, collection, [][]float32{{1, 0, 0}, {0, 1, 0}}, []map[string]interface{}{
		{"sample_id": "sample-delete", "scope": "delete-scope"},
		{"sample_id": "sample-keep", "scope": "delete-scope"},
	})
	if err != nil {
		t.Fatalf("insert qdrant vectors: %v", err)
	}
	if err := adapter.Delete(ctx, collection, map[string]interface{}{"sample_id": "sample-delete"}); err != nil {
		t.Fatalf("delete qdrant vector: %v", err)
	}
	results, err := adapter.Search(ctx, collection, []float32{1, 0, 0}, 5)
	if err != nil {
		t.Fatalf("search qdrant vectors: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected qdrant search results")
	}
	if _, ok := results[0]["score"].(float64); !ok {
		t.Fatalf("expected qdrant search score, got %#v", results[0])
	}
	for _, result := range results {
		if result["sample_id"] == "sample-delete" {
			t.Fatalf("deleted qdrant payload still returned: %#v", results)
		}
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
