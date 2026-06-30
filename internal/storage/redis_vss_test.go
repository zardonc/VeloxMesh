package storage

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRedisVSSVectorAdapter_Integration(t *testing.T) {
	// Simple test that just checks initialization without needing real Redis
	// Real integration would need a Redis instance with RediSearch loaded.
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
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

	err = adapter.Delete(ctx, "test_collection", map[string]interface{}{"id": "test-id"})
	if err != nil {
		t.Errorf("delete failed: %v", err)
	}
}
