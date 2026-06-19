package hotstate

import (
	"context"
	"sync"
	"time"
)

type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

type authCacheItem struct {
	allowed   bool
	expiresAt time.Time
}

type LocalHotState struct {
	mu          sync.RWMutex
	healthItems map[string]cacheItem
	probeItems  map[string]cacheItem
	authItems   map[string]authCacheItem
}

func NewLocalHotState() *LocalHotState {
	return &LocalHotState{
		healthItems: make(map[string]cacheItem),
		probeItems:  make(map[string]cacheItem),
		authItems:   make(map[string]authCacheItem),
	}
}

func (l *LocalHotState) Ping(ctx context.Context) error {
	return nil
}

func (l *LocalHotState) Close() error {
	return nil
}

func (l *LocalHotState) GetHealthSnapshot(ctx context.Context, providerID string) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.healthItems[providerID]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, ErrCacheMiss
	}
	return item.data, nil
}

func (l *LocalHotState) SetHealthSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.healthItems[providerID] = cacheItem{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (l *LocalHotState) GetProbeSnapshot(ctx context.Context, providerID string) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.probeItems[providerID]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, ErrCacheMiss
	}
	return item.data, nil
}

func (l *LocalHotState) SetProbeSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.probeItems[providerID] = cacheItem{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (l *LocalHotState) GetCachedAuthResult(ctx context.Context, tokenHash string) (bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.authItems[tokenHash]
	if !ok || time.Now().After(item.expiresAt) {
		return false, ErrCacheMiss
	}
	return item.allowed, nil
}

func (l *LocalHotState) CacheAuthResult(ctx context.Context, tokenHash string, allowed bool, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.authItems[tokenHash] = authCacheItem{
		allowed:   allowed,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// localSubscription is a no-op subscription for local mode.
type localSubscription struct {
	ch chan *ConfigChangeMessage
}

func (s *localSubscription) Channel() <-chan *ConfigChangeMessage {
	return s.ch
}

func (s *localSubscription) Close() error {
	return nil
}

func (l *LocalHotState) PublishConfigChange(ctx context.Context, msg *ConfigChangeMessage) error {
	// No-op for local hot state
	return nil
}

func (l *LocalHotState) SubscribeConfigChanges(ctx context.Context) (Subscription, error) {
	// Return a dummy subscription that never receives messages
	return &localSubscription{ch: make(chan *ConfigChangeMessage)}, nil
}
