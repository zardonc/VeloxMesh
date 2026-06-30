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
	identity  *CachedIdentity
	expiresAt time.Time
}

type limitItem struct {
	count     int64
	expiresAt time.Time
}

type LocalHotState struct {
	mu             sync.RWMutex
	healthItems    map[string]cacheItem
	probeItems     map[string]cacheItem
	authItems      map[string]authCacheItem
	byteItems      map[string]cacheItem
	limitItems     map[string]limitItem
	blacklistItems map[string]cacheItem
}

func NewLocalHotState() *LocalHotState {
	return &LocalHotState{
		healthItems:    make(map[string]cacheItem),
		probeItems:     make(map[string]cacheItem),
		authItems:      make(map[string]authCacheItem),
		byteItems:      make(map[string]cacheItem),
		limitItems:     make(map[string]limitItem),
		blacklistItems: make(map[string]cacheItem),
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

func (l *LocalHotState) GetCachedIdentity(ctx context.Context, tokenHash string) (*CachedIdentity, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.authItems[tokenHash]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, ErrCacheMiss
	}
	// return a copy to prevent mutation
	identityCopy := *item.identity
	return &identityCopy, nil
}

func (l *LocalHotState) CacheIdentity(ctx context.Context, tokenHash string, identity *CachedIdentity, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	identityCopy := *identity
	l.authItems[tokenHash] = authCacheItem{
		identity:  &identityCopy,
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

func (l *LocalHotState) GetBytes(ctx context.Context, key string) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.byteItems[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, ErrCacheMiss
	}
	return item.data, nil
}

func (l *LocalHotState) SetBytes(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.byteItems[key] = cacheItem{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (l *LocalHotState) Delete(ctx context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byteItems, key)
	return nil
}

func (l *LocalHotState) CheckAndIncrement(ctx context.Context, key string, limit int64, window time.Duration) (int64, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	now := time.Now()
	item, ok := l.limitItems[key]
	if !ok || now.After(item.expiresAt) {
		// Start a new window
		item = limitItem{
			count:     0,
			expiresAt: now.Add(window),
		}
	}
	
	if item.count >= limit {
		// Do not increment
		l.limitItems[key] = item
		return item.count, false, nil
	}
	
	item.count++
	l.limitItems[key] = item
	return item.count, true, nil
}

func (l *LocalHotState) IsBlacklisted(ctx context.Context, sessionID string) (bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	item, ok := l.blacklistItems[sessionID]
	if !ok || time.Now().After(item.expiresAt) {
		return false, nil
	}
	return true, nil
}

func (l *LocalHotState) BlacklistSession(ctx context.Context, sessionID string, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.blacklistItems[sessionID] = cacheItem{
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (l *LocalHotState) AggregateCost(ctx context.Context, providerID, model, apiKeyID string, credits int64) error {
	// Not implemented in local cache as it's primarily a Redis concern for this task
	return nil
}
