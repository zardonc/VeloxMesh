package replication

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type StreamProducer interface {
	Append(ctx context.Context, event ChangeEvent) (string, error)
}

func NewChangeEvent(repository, operation, targetID string, payload interface{}) (ChangeEvent, error) {
	if repository == "semantic_cache_vectors" || repository == "qdrant" || repository == "redis_vss" || repository == "vectors" {
		return ChangeEvent{}, errors.New("cannot create change event for vector storage categories")
	}

	var payloadBytes []byte
	var payloadHash string
	if payload != nil {
		var err error
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return ChangeEvent{}, err
		}
		hash := sha256.Sum256(payloadBytes)
		payloadHash = hex.EncodeToString(hash[:])
	}

	return ChangeEvent{
		Repository:  repository,
		Operation:   operation,
		TargetID:    targetID,
		PayloadHash: payloadHash,
		Payload:     payloadBytes,
		Timestamp:   time.Now().UTC(),
	}, nil
}

type RedisStreamProducer struct {
	client redis.Cmdable
	stream string
}

func NewRedisStreamProducer(client redis.Cmdable, stream string) *RedisStreamProducer {
	return &RedisStreamProducer{
		client: client,
		stream: stream,
	}
}

func (p *RedisStreamProducer) Append(ctx context.Context, event ChangeEvent) (string, error) {
	b, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	args := &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]interface{}{
			"event": b,
		},
	}

	id, err := p.client.XAdd(ctx, args).Result()
	if err != nil {
		return "", err
	}
	return id, nil
}
