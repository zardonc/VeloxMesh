package scheduler

import (
	"context"

	"github.com/redis/go-redis/v9"

	"veloxmesh/internal/hotstate"
)

type RedisQueue struct {
	cmd redis.Cmdable
	key string
}

func NewRedisQueue(cmd redis.Cmdable, namespace string, queueName string) *RedisQueue {
	return &RedisQueue{
		cmd: cmd,
		key: hotstate.NamespacedKey(namespace, "scheduler_queue", queueName),
	}
}

func (q *RedisQueue) Push(ctx context.Context, item QueueItem) error {
	return q.cmd.ZAdd(ctx, q.key, redis.Z{Score: item.Score, Member: item.TaskID}).Err()
}

func (q *RedisQueue) PopMin(ctx context.Context) (QueueItem, error) {
	values, err := q.cmd.ZPopMin(ctx, q.key, 1).Result()
	if err != nil {
		return QueueItem{}, err
	}
	if len(values) == 0 {
		return QueueItem{}, ErrQueueEmpty
	}
	taskID, ok := values[0].Member.(string)
	if !ok {
		return QueueItem{}, ErrTaskNotFound
	}
	return QueueItem{TaskID: taskID, Score: values[0].Score}, nil
}

func (q *RedisQueue) Remove(ctx context.Context, taskID string) error {
	removed, err := q.cmd.ZRem(ctx, q.key, taskID).Result()
	if err != nil {
		return err
	}
	if removed == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (q *RedisQueue) Len(ctx context.Context) (int64, error) {
	return q.cmd.ZCard(ctx, q.key).Result()
}

func (q *RedisQueue) keyForTest() string {
	return q.key
}
