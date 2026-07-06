package storage

import (
	"context"
	"os"
	"testing"

	"veloxmesh/internal/testenv"
)

func TestPGVectorDimensionValidation(t *testing.T) {
	adapter := &PGVectorAdapter{dimension: 2}
	if err := adapter.Insert(context.Background(), "c", [][]float32{{1, 2, 3}}, nil); err == nil {
		t.Fatalf("expected insert dimension error")
	}
	if _, err := adapter.Search(context.Background(), "c", []float32{1}, 1); err == nil {
		t.Fatalf("expected search dimension error")
	}
}

func TestPGVectorMetadataAllowlist(t *testing.T) {
	meta := safePGVectorMetadata(map[string]interface{}{
		"id":       "id-1",
		"scope":    "scope-1",
		"model":    "gpt-4",
		"usage_id": "usage-1",
		"response": `{"ok":true}`,
		"prompt":   "raw prompt must not be stored",
	})
	if _, ok := meta["prompt"]; ok {
		t.Fatalf("raw prompt leaked into pgvector metadata")
	}
	if meta["id"] != "id-1" || meta["scope"] != "scope-1" || meta["model"] != "gpt-4" {
		t.Fatalf("safe metadata fields missing: %+v", meta)
	}
}

func TestPGVectorMigrationAndSearch(t *testing.T) {
	testenv.Load()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Fatalf("POSTGRES_TEST_DSN is required for real pgvector tests")
	}
	ctx := context.Background()
	adapter, err := NewPGVectorAdapter(ctx, dsn, PGVectorOptions{
		Dimension:          1536,
		HNSWM:              16,
		HNSWEFConstruction: 64,
		SearchEF:           40,
	})
	if err != nil {
		t.Fatalf("new pgvector adapter: %v", err)
	}

	const prompt = "raw prompt sentinel"
	vector := pgVectorTestEmbedding(1536)
	err = adapter.Insert(ctx, "semantic_cache:scope-a:gpt-4", [][]float32{vector}, []map[string]interface{}{{
		"id":     "pgv-1",
		"scope":  "scope-a",
		"model":  "gpt-4",
		"prompt": prompt,
	}})
	if err != nil {
		t.Fatalf("insert pgvector: %v", err)
	}
	results, err := adapter.Search(ctx, "semantic_cache:scope-a:gpt-4", vector, 1)
	if err != nil {
		t.Fatalf("search pgvector: %v", err)
	}
	if len(results) != 1 || results[0]["id"] != "pgv-1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if _, ok := results[0]["prompt"]; ok {
		t.Fatalf("raw prompt leaked into search metadata")
	}
}

func TestPGVectorEnsureCollectionUsesRealSchema(t *testing.T) {
	testenv.Load()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Fatalf("POSTGRES_TEST_DSN is required for real pgvector tests")
	}
	ctx := context.Background()
	adapter, err := NewPGVectorAdapter(ctx, dsn, PGVectorOptions{
		Dimension:          1536,
		HNSWM:              16,
		HNSWEFConstruction: 64,
		SearchEF:           40,
	})
	if err != nil {
		t.Fatalf("new pgvector adapter: %v", err)
	}
	if err := adapter.EnsureCollection(ctx, "scheduler_training_samples", 1536); err != nil {
		t.Fatalf("ensure pgvector collection: %v", err)
	}
	if err := adapter.EnsureCollection(ctx, "scheduler_training_samples", 3); err == nil {
		t.Fatalf("expected pgvector dimension mismatch")
	}
}

func pgVectorTestEmbedding(dimension int) []float32 {
	vector := make([]float32, dimension)
	vector[0] = 1
	return vector
}
