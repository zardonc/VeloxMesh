package storage

import (
	"context"
	"os"
	"strings"
	"testing"

	"veloxmesh/internal/testenv"
)

func TestRedisVSSVectorAdapter_Integration(t *testing.T) {
	testenv.Load()
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping Redis VSS test because REDIS_ADDR is not set")
	}

	adapter, err := NewRedisVSSVectorAdapter(context.Background(), redisAddr, "", 0, "test")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "connection refused") ||
			strings.Contains(strings.ToLower(err.Error()), "actively refused it") ||
			strings.Contains(strings.ToLower(err.Error()), "unavailable") ||
			strings.Contains(strings.ToLower(err.Error()), "no such host") {
			t.Skipf("Skipping Redis VSS test because Redis Stack is not available: %v", err)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	defer adapter.client.Close()
	ctx := context.Background()

	vectors := [][]float32{{1.0, 2.0, 3.0}}
	metadata := []map[string]interface{}{{"id": "test-id", "scope": "test-scope"}}

	err = adapter.Insert(ctx, "test_collection", vectors, metadata)
	if err != nil {
		t.Fatalf("insert failed: %v", err)
	}

	results, err := adapter.Search(ctx, "test_collection", []float32{1.0, 2.0, 3.0}, 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Errorf("expected at least 1 result")
	}
	if _, ok := results[0]["score"].(float64); !ok {
		t.Fatalf("expected Redis VSS search score, got %#v", results[0])
	}

	err = adapter.Delete(ctx, "test_collection", map[string]interface{}{"id": "test-id"})
	if err != nil {
		t.Errorf("delete failed: %v", err)
	}
}
