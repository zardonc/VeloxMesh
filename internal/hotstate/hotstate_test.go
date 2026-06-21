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
	allowed, err := local.GetCachedAuthResult(ctx, "token1")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	err = local.CacheAuthResult(ctx, "token1", true, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	allowed, err = local.GetCachedAuthResult(ctx, "token1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Errorf("expected allowed to be true")
	}

	// TTL
	err = local.CacheAuthResult(ctx, "token2", true, -time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = local.GetCachedAuthResult(ctx, "token2")
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
