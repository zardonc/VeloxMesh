package app

import (
	"context"
	"testing"

	"veloxmesh/internal/config"
	"veloxmesh/internal/storage"
)

func TestNewVectorAdapterDefaultsToNoopWhenLanceDBUnavailable(t *testing.T) {
	cfg := &config.Config{}
	adapter := newVectorAdapter(context.Background(), cfg, discardLogger())
	if _, ok := adapter.(*storage.NoopVectorAdapter); !ok {
		t.Fatalf("expected noop vector adapter for unavailable default LanceDB, got %T", adapter)
	}
}

func TestNewVectorAdapterExplicitLanceDBDegradesWhenUnavailable(t *testing.T) {
	cfg := &config.Config{SemanticCacheVectorStore: "lancedb"}
	adapter := newVectorAdapter(context.Background(), cfg, discardLogger())
	if _, ok := adapter.(*storage.DegradedVectorAdapter); !ok {
		t.Fatalf("expected degraded vector adapter for explicit unavailable LanceDB, got %T", adapter)
	}
}

func TestNewQdrantVectorAdapterDegradesWithoutFallbacks(t *testing.T) {
	cfg := &config.Config{
		SemanticCacheVectorStore: "qdrant",
		QdrantAddr:               "127.0.0.1:1",
	}
	adapter := newVectorAdapter(context.Background(), cfg, discardLogger())
	if _, ok := adapter.(*storage.DegradedVectorAdapter); !ok {
		t.Fatalf("expected degraded vector adapter for unavailable Qdrant, got %T", adapter)
	}
}
