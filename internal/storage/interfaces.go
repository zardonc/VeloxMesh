package storage

import (
	"context"
	"veloxmesh/internal/controlstate"
)

// CacheAdapter defines the interface for distributed caching (e.g., Redis).
type CacheAdapter interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttlSeconds int) error
	Delete(ctx context.Context, key string) error
}

// CoordAdapter defines the interface for cluster coordination (e.g., leader election, pub/sub).
type CoordAdapter interface {
	AcquireLock(ctx context.Context, key string, ttlSeconds int) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
	Publish(ctx context.Context, channel, message string) error
	Subscribe(ctx context.Context, channel string, handler func(message string)) error
}

// DBAdapter defines the boundary for relational storage extensions.
type DBAdapter interface {
	Ping(ctx context.Context) error
	Close() error
}

// VectorAdapter defines the interface for vector similarity search.
type VectorAdapter interface {
	Ping(ctx context.Context) error
	Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error
	Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error)
	Delete(ctx context.Context, collection string, filter map[string]interface{}) error
}

type VectorCollectionEnsurer interface {
	EnsureCollection(ctx context.Context, collection string, dimension int) error
}

// SemanticCacheAdapter defines the interface for semantic caching operations.
type SemanticCacheAdapter interface {
	Lookup(ctx context.Context, scope, model string, text string) (*controlstate.SemanticCacheEntry, error)
	Store(ctx context.Context, id, scope, model string, text string, response string, usageID *string) error
}
