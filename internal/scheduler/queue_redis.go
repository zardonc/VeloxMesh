package scheduler

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"veloxmesh/internal/hotstate"
)

const redisTaskLockTTL = 30 * time.Second

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

func (q *RedisQueue) PeekMin(ctx context.Context, limit int) ([]QueueItem, error) {
	if limit < 1 {
		return []QueueItem{}, nil
	}
	values, err := q.cmd.ZRangeWithScores(ctx, q.key, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	items := make([]QueueItem, 0, len(values))
	for _, value := range values {
		taskID, ok := value.Member.(string)
		if !ok {
			return nil, ErrTaskNotFound
		}
		items = append(items, QueueItem{TaskID: taskID, Score: value.Score})
	}
	return items, nil
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

type RedisTaskLocker struct {
	cmd       redis.Cmdable
	namespace string
	ttl       time.Duration
}

func NewRedisTaskLocker(cmd redis.Cmdable, namespace string) *RedisTaskLocker {
	return &RedisTaskLocker{cmd: cmd, namespace: namespace, ttl: redisTaskLockTTL}
}

func (l *RedisTaskLocker) Claim(ctx context.Context, taskID string) (bool, error) {
	return l.cmd.SetNX(ctx, l.key(taskID), "1", l.ttl).Result()
}

func (l *RedisTaskLocker) Release(ctx context.Context, taskID string) error {
	return l.cmd.Del(ctx, l.key(taskID)).Err()
}

func (l *RedisTaskLocker) key(taskID string) string {
	return hotstate.NamespacedKey(l.namespace, "scheduler_task_lock", taskID)
}
