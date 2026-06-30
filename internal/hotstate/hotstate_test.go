package hotstate

import (
	"context"
	"testing"
	"time"
)

func TestNamespacedKey(t *testing.T) {
	key := NamespacedKey("prod", "health", "p1")
	if key != "prod:health:p1" {
		t.Errorf("expected prod:health:p1, got %s", key)
	}

	keyEmpty := NamespacedKey("", "health", "p1")
	if keyEmpty != "unnamespaced:health:p1" {
		t.Errorf("expected unnamespaced:health:p1, got %s", keyEmpty)
	}
}

func TestLocalHotState(t *testing.T) {
	local := NewLocalHotState()
	ctx := context.Background()

	// Health
	_, err := local.GetHealthSnapshot(ctx, "p1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	err = local.SetHealthSnapshot(ctx, "p1", []byte("data"), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := local.GetHealthSnapshot(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("expected data, got %s", string(data))
	}

	// Probe
	_, err = local.GetProbeSnapshot(ctx, "p1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	err = local.SetProbeSnapshot(ctx, "p1", []byte("probe-data"), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err = local.GetProbeSnapshot(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "probe-data" {
		t.Errorf("expected probe-data, got %s", string(data))
	}

	// Auth
	_, err = local.GetCachedIdentity(ctx, "token1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	err = local.CacheIdentity(ctx, "token1", &CachedIdentity{ID: "token1", Enabled: true}, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identity, err := local.GetCachedIdentity(ctx, "token1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !identity.Enabled {
		t.Errorf("expected allowed to be true")
	}

	// TTL
	err = local.CacheIdentity(ctx, "token2", &CachedIdentity{ID: "token2", Enabled: true}, -time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = local.GetCachedIdentity(ctx, "token2")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss due to ttl expiry, got %v", err)
	}
}

func TestLocalHotState_PubSub(t *testing.T) {
	local := NewLocalHotState()
	ctx := context.Background()
	msg := &ConfigChangeMessage{ProviderID: "test"}
	if err := local.PublishConfigChange(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub, err := local.SubscribeConfigChanges(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-sub.Channel():
		t.Fatalf("should not receive any messages")
	default:
	}
	_ = sub.Close()
}

func TestLocalHotState_ByteCache(t *testing.T) {
	local := NewLocalHotState()
	ctx := context.Background()

	// Get missing
	_, err := local.GetBytes(ctx, "key1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	// Set and Get
	err = local.SetBytes(ctx, "key1", []byte("data1"), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := local.GetBytes(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "data1" {
		t.Errorf("expected data1, got %s", string(data))
	}

	// Delete
	err = local.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = local.GetBytes(ctx, "key1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestLocalHotState_AtomicLimiter(t *testing.T) {
	local := NewLocalHotState()
	ctx := context.Background()

	// 1st request
	count, allowed, err := local.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 || !allowed {
		t.Errorf("expected 1, true; got %d, %v", count, allowed)
	}

	// 2nd request
	count, allowed, err = local.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 || !allowed {
		t.Errorf("expected 2, true; got %d, %v", count, allowed)
	}

	// 3rd request (should be rejected)
	count, allowed, err = local.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 || allowed {
		t.Errorf("expected 2, false; got %d, %v", count, allowed)
	}
}

func TestLocalHotState_SessionBlacklist(t *testing.T) {
	local := NewLocalHotState()
	ctx := context.Background()

	isBlacklisted, err := local.IsBlacklisted(ctx, "sess1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBlacklisted {
		t.Errorf("expected false, got true")
	}

	err = local.BlacklistSession(ctx, "sess1", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	isBlacklisted, err = local.IsBlacklisted(ctx, "sess1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBlacklisted {
		t.Errorf("expected true, got false")
	}
}

func TestConfigChangeMessage(t *testing.T) {
	msg := ConfigChangeMessage{
		Type:     EventProvider,
		TargetID: "prod1",
		Action:   "create",
	}

	if msg.Type != EventProvider {
		t.Errorf("expected %s, got %s", EventProvider, msg.Type)
	}
}
