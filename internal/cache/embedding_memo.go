package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"sync"
	"time"
)

// Per-service storage. Keys contain opaque scope and hashes, never raw questions.
// No network operation occurs under mu; concurrent cold calls remain independent.
type embeddingMemo struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	recent   *list.List
	capacity int
	ttl      time.Duration
}

type memoEntry struct {
	key     string
	vector  []float32
	expires time.Time
}

func newEmbeddingMemo(config SemanticCacheConfig) *embeddingMemo {
	if !config.Enabled || config.EmbeddingMemoCapacity <= 0 || config.EmbeddingMemoTTL <= 0 {
		return nil
	}
	return &embeddingMemo{entries: make(map[string]*list.Element), recent: list.New(),
		capacity: config.EmbeddingMemoCapacity, ttl: config.EmbeddingMemoTTL}
}

func memoKey(query CacheLookup) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{query.Scope, query.Model, query.Text}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func (m *embeddingMemo) get(query CacheLookup) ([]float32, bool) {
	if m == nil {
		return nil, false
	}
	key := memoKey(query)
	m.mu.Lock()
	defer m.mu.Unlock()
	element, exists := m.entries[key]
	if !exists {
		return nil, false
	}
	entry := element.Value.(memoEntry)
	if !time.Now().Before(entry.expires) {
		m.remove(element)
		return nil, false
	}
	m.recent.MoveToFront(element)
	return slices.Clone(entry.vector), true
}

func (m *embeddingMemo) put(query CacheLookup, vector []float32) {
	if m == nil {
		return
	}
	entry := memoEntry{key: memoKey(query), vector: slices.Clone(vector), expires: time.Now().Add(m.ttl)}
	m.mu.Lock()
	defer m.mu.Unlock()
	if previous := m.entries[entry.key]; previous != nil {
		m.remove(previous)
	}
	m.entries[entry.key] = m.recent.PushFront(entry)
	if m.recent.Len() > m.capacity {
		m.remove(m.recent.Back())
	}
}

func (m *embeddingMemo) remove(element *list.Element) {
	delete(m.entries, element.Value.(memoEntry).key)
	m.recent.Remove(element)
}
