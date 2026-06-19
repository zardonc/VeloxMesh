package hotstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type RedisClient struct {
	client    *redis.Client
	namespace string
}

func NewRedisClient(ctx context.Context, addr, password string, db int, namespace string) (*RedisClient, error) {
	if namespace == "" {
		return nil, errors.New("redis namespace must be configured")
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return &RedisClient{
		client:    client,
		namespace: namespace,
	}, nil
}

func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) GetHealthSnapshot(ctx context.Context, providerID string) ([]byte, error) {
	key := NamespacedKey(r.namespace, "health", providerID)
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return val, nil
}

func (r *RedisClient) SetHealthSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error {
	key := NamespacedKey(r.namespace, "health", providerID)
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisClient) GetProbeSnapshot(ctx context.Context, providerID string) ([]byte, error) {
	key := NamespacedKey(r.namespace, "probe", providerID)
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return val, nil
}

func (r *RedisClient) SetProbeSnapshot(ctx context.Context, providerID string, data []byte, ttl time.Duration) error {
	key := NamespacedKey(r.namespace, "probe", providerID)
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisClient) GetCachedAuthResult(ctx context.Context, tokenHash string) (bool, error) {
	key := NamespacedKey(r.namespace, "auth", tokenHash)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, ErrCacheMiss
		}
		return false, err
	}
	return val == "1", nil
}

func (r *RedisClient) CacheAuthResult(ctx context.Context, tokenHash string, allowed bool, ttl time.Duration) error {
	key := NamespacedKey(r.namespace, "auth", tokenHash)
	val := "0"
	if allowed {
		val = "1"
	}
	return r.client.Set(ctx, key, val, ttl).Err()
}

func (r *RedisClient) PublishConfigChange(ctx context.Context, msg *ConfigChangeMessage) error {
	channel := NamespacedKey(r.namespace, "channel", "config-change")
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal config change message: %w", err)
	}
	return r.client.Publish(ctx, channel, data).Err()
}

func (r *RedisClient) SubscribeConfigChanges(ctx context.Context) (Subscription, error) {
	channel := NamespacedKey(r.namespace, "channel", "config-change")
	pubsub := r.client.Subscribe(ctx, channel)
	
	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("failed to subscribe to config changes: %w", err)
	}
	
	return &redisSubscription{pubsub: pubsub}, nil
}

type redisSubscription struct {
	pubsub *redis.PubSub
}

func (s *redisSubscription) Channel() <-chan *ConfigChangeMessage {
	ch := make(chan *ConfigChangeMessage)
	go func() {
		defer close(ch)
		for msg := range s.pubsub.Channel() {
			var configMsg ConfigChangeMessage
			if err := json.Unmarshal([]byte(msg.Payload), &configMsg); err == nil {
				ch <- &configMsg
			}
		}
	}()
	return ch
}

func (s *redisSubscription) Close() error {
	return s.pubsub.Close()
}
