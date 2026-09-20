package integration_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/hotstate"
)

func TestRedisHotState_PubSub(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping redis tests, REDIS_ADDR not set")
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	ctx := context.Background()
	namespace := uniqueRedisNamespace(t)
	client, err := hotstate.NewRedisClient(ctx, redisAddr, redisPassword, 0, namespace)
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}
	defer client.Close()

	sub, err := client.SubscribeConfigChanges(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Close()

	msg := &hotstate.ConfigChangeMessage{
		ProviderID: "test-prov",
		Action:     "create",
		Revision:   1,
		Timestamp:  time.Now().UTC().Round(time.Millisecond),
	}

	// Publish in a separate goroutine
	go func() {
		time.Sleep(100 * time.Millisecond)
		err := client.PublishConfigChange(ctx, msg)
		if err != nil {
			t.Errorf("failed to publish: %v", err)
		}
	}()

	select {
	case received := <-sub.Channel():
		if !reflect.DeepEqual(msg, received) {
			t.Errorf("expected %+v, got %+v", msg, received)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for config change message")
	}
}

func TestRedisHotState_SecretSafe(t *testing.T) {
	// Verify that ConfigChangeMessage has no secret fields by reflection
	msgType := reflect.TypeOf(hotstate.ConfigChangeMessage{})
	for i := 0; i < msgType.NumField(); i++ {
		field := msgType.Field(i)
		name := field.Name
		if name == "APIKey" || name == "Secret" || name == "Ciphertext" || name == "Nonce" || name == "Authorization" || name == "Bearer" || name == "Prompt" {
			t.Fatalf("ConfigChangeMessage contains restricted field: %s", name)
		}
	}
}

func getRedisClient(t *testing.T) (*hotstate.RedisClient, string) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping redis tests, REDIS_ADDR not set")
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")

	ctx := context.Background()
	namespace := uniqueRedisNamespace(t)
	client, err := hotstate.NewRedisClient(ctx, redisAddr, redisPassword, 0, namespace)
	if err != nil {
		t.Fatalf("failed to connect to redis: %v", err)
	}
	return client, namespace
}

func uniqueRedisNamespace(t *testing.T) string {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	return "veloxmesh:test:" + name + ":" + fmt.Sprint(time.Now().UnixNano())
}

func TestRedisHotState_ByteCache(t *testing.T) {
	client, _ := getRedisClient(t)
	defer client.Close()
	ctx := context.Background()

	// Get missing
	_, err := client.GetBytes(ctx, "key1")
	if err != hotstate.ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	// Set and Get
	err = client.SetBytes(ctx, "key1", []byte("data1"), time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := client.GetBytes(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != "data1" {
		t.Errorf("expected data1, got %s", string(data))
	}

	// Delete
	err = client.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = client.GetBytes(ctx, "key1")
	if err != hotstate.ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestRedisHotState_AtomicLimiter(t *testing.T) {
	client, _ := getRedisClient(t)
	defer client.Close()
	ctx := context.Background()

	// 1st request
	count, allowed, err := client.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 || !allowed {
		t.Errorf("expected 1, true; got %d, %v", count, allowed)
	}

	// 2nd request
	count, allowed, err = client.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 || !allowed {
		t.Errorf("expected 2, true; got %d, %v", count, allowed)
	}

	// 3rd request (should be rejected)
	count, allowed, err = client.CheckAndIncrement(ctx, "lim1", 2, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 || allowed {
		t.Errorf("expected 2, false; got %d, %v", count, allowed)
	}
}

func TestRedisHotState_SessionBlacklist(t *testing.T) {
	client, _ := getRedisClient(t)
	defer client.Close()
	ctx := context.Background()

	isBlacklisted, err := client.IsBlacklisted(ctx, "sess1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBlacklisted {
		t.Errorf("expected false, got true")
	}

	err = client.BlacklistSession(ctx, "sess1", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	isBlacklisted, err = client.IsBlacklisted(ctx, "sess1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBlacklisted {
		t.Errorf("expected true, got false")
	}
}
