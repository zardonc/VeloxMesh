package storage

import (
	"context"
	"errors"
	"sync"
	"time"
)

// MemoryCacheAdapter provides an in-memory implementation of CacheAdapter.
type MemoryCacheAdapter struct {
	mu   sync.RWMutex
	data map[string]cacheItem
}

type cacheItem struct {
	value     []byte
	expiresAt time.Time
}

func NewMemoryCacheAdapter() *MemoryCacheAdapter {
	return &MemoryCacheAdapter{
		data: make(map[string]cacheItem),
	}
}

func (m *MemoryCacheAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil, errors.New("key not found")
	}
	if time.Now().After(item.expiresAt) {
		return nil, errors.New("key expired")
	}
	return item.value, nil
}

func (m *MemoryCacheAdapter) Set(ctx context.Context, key string, value []byte, ttlSeconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.data[key] = cacheItem{
		value:     value,
		expiresAt: time.Now().Add(time.Duration(ttlSeconds) * time.Second),
	}
	return nil
}

func (m *MemoryCacheAdapter) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// NoopCoordAdapter provides a no-op implementation of CoordAdapter.
type NoopCoordAdapter struct{}

func NewNoopCoordAdapter() *NoopCoordAdapter {
	return &NoopCoordAdapter{}
}

func (n *NoopCoordAdapter) AcquireLock(ctx context.Context, key string, ttlSeconds int) (bool, error) {
	// In a single-node no-op setup, we can pretend we got the lock.
	return true, nil
}

func (n *NoopCoordAdapter) ReleaseLock(ctx context.Context, key string) error {
	return nil
}

func (n *NoopCoordAdapter) Publish(ctx context.Context, channel, message string) error {
	return nil
}

func (n *NoopCoordAdapter) Subscribe(ctx context.Context, channel string, handler func(message string)) error {
	return nil
}

// SQLiteDBAdapter is a wrapper for DBAdapter when using SQLite.
type SQLiteDBAdapter struct {
	// Add inner sql.DB or similar if needed in future
}

func NewSQLiteDBAdapter() *SQLiteDBAdapter {
	return &SQLiteDBAdapter{}
}

func (s *SQLiteDBAdapter) Ping(ctx context.Context) error {
	return nil
}

func (s *SQLiteDBAdapter) Close() error {
	return nil
}

// LanceDBVectorAdapter provides a disabled implementation of VectorAdapter.
// LanceDB Go bindings require CGO_ENABLED=1, which is not supported in this
// pure Go (CGO_ENABLED=0) deployment environment.
type LanceDBVectorAdapter struct{}

func NewLanceDBVectorAdapter() *LanceDBVectorAdapter {
	return &LanceDBVectorAdapter{}
}

func (l *LanceDBVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	return errors.New("lancedb vector adapter is disabled: requires CGO_ENABLED=1")
}

func (l *LanceDBVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	return nil, errors.New("lancedb vector adapter is disabled: requires CGO_ENABLED=1")
}
