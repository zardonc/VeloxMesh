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

func (r *RedisClient) GetCachedIdentity(ctx context.Context, tokenHash string) (*CachedIdentity, error) {
	key := NamespacedKey(r.namespace, "auth", tokenHash)
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	var identity CachedIdentity
	if err := json.Unmarshal(val, &identity); err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *RedisClient) CacheIdentity(ctx context.Context, tokenHash string, identity *CachedIdentity, ttl time.Duration) error {
	key := NamespacedKey(r.namespace, "auth", tokenHash)
	val, err := json.Marshal(identity)
	if err != nil {
		return err
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

func (r *RedisClient) GetBytes(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return val, nil
}

func (r *RedisClient) SetBytes(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisClient) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

var checkAndIncScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
local limit = tonumber(ARGV[1])
if current and tonumber(current) >= limit then
	return {tonumber(current), 0}
end
current = redis.call("INCR", KEYS[1])
if tonumber(current) == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return {tonumber(current), 1}
`)

func (r *RedisClient) CheckAndIncrement(ctx context.Context, key string, limit int64, window time.Duration) (int64, bool, error) {
	res, err := checkAndIncScript.Run(ctx, r.client, []string{key}, limit, window.Milliseconds()).Result()
	if err != nil {
		return 0, false, err
	}
	
	vals, ok := res.([]interface{})
	if !ok || len(vals) != 2 {
		return 0, false, fmt.Errorf("unexpected script result type: %T", res)
	}
	
	count, ok1 := vals[0].(int64)
	allowedInt, ok2 := vals[1].(int64)
	if !ok1 || !ok2 {
		return 0, false, fmt.Errorf("unexpected script return values")
	}
	
	return count, allowedInt == 1, nil
}

func (r *RedisClient) IsBlacklisted(ctx context.Context, sessionID string) (bool, error) {
	key := NamespacedKey(r.namespace, "blacklist", sessionID)
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	return val == "1", nil
}

func (r *RedisClient) BlacklistSession(ctx context.Context, sessionID string, ttl time.Duration) error {
	key := NamespacedKey(r.namespace, "blacklist", sessionID)
	return r.client.Set(ctx, key, "1", ttl).Err()
}

func (r *RedisClient) AggregateCost(ctx context.Context, providerID, model, apiKeyID string, credits int64) error {
	if apiKeyID == "" {
		apiKeyID = "anonymous"
	}
	key := NamespacedKey(r.namespace, "cost_agg", fmt.Sprintf("%s:%s:%s", providerID, model, apiKeyID))
	return r.client.IncrBy(ctx, key, credits).Err()
}
