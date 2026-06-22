package integration_test

import (
	"context"
	"os"
	"reflect"
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
	namespace := "veloxmesh:test:" + time.Now().Format("20060102150405")
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
