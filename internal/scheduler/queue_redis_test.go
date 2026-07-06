package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func TestRedisQueueRealZSetOperations(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	defer client.Del(ctx, queue.keyForTest())

	for _, item := range []QueueItem{{TaskID: "t2", Score: 2}, {TaskID: "t1", Score: 1}} {
		if err := queue.Push(ctx, item); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	length, err := queue.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 2 {
		t.Fatalf("Len=%d, want 2", length)
	}
	item, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if item.TaskID != "t1" || item.Score != 1 {
		t.Fatalf("unexpected first item: %#v", item)
	}
	if err := queue.Remove(ctx, "t2"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := queue.Remove(ctx, "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestRedisQueueStoresOnlyTaskIDMember(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	defer client.Del(ctx, queue.keyForTest())

	if err := queue.Push(ctx, QueueItem{TaskID: "task-safe-id", Score: 4}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	values, err := client.ZRangeWithScores(ctx, queue.keyForTest(), 0, -1).Result()
	if err != nil {
		t.Fatalf("ZRangeWithScores: %v", err)
	}
	if len(values) != 1 || values[0].Member != "task-safe-id" || values[0].Score != 4 {
		t.Fatalf("unexpected Redis member: %#v", values)
	}
}

func TestRedisQueuePeekMinDoesNotPopAndPushReplacesScore(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	defer client.Del(ctx, queue.keyForTest())

	for _, item := range []QueueItem{
		{TaskID: "later", Score: 3},
		{TaskID: "first", Score: 2},
		{TaskID: "later", Score: 1},
	} {
		if err := queue.Push(ctx, item); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}
	items, err := queue.PeekMin(ctx, 2)
	if err != nil {
		t.Fatalf("PeekMin: %v", err)
	}
	if len(items) != 2 || items[0].TaskID != "later" || items[0].Score != 1 || items[1].TaskID != "first" {
		t.Fatalf("unexpected peek order: %#v", items)
	}
	length, err := queue.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 2 {
		t.Fatalf("PeekMin mutated Redis length to %d", length)
	}
	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "later" || got.Score != 1 {
		t.Fatalf("unexpected first pop after peek: %#v", got)
	}
}

func TestRedisTaskLockerUsesSetNXAndTTL(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	locker := NewRedisTaskLocker(client, "veloxmesh-test")
	defer client.Del(ctx, queue.keyForTest(), locker.key("task-lock"))

	claimed, err := locker.Claim(ctx, "task-lock")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !claimed {
		t.Fatalf("expected first claim")
	}
	claimed, err = locker.Claim(ctx, "task-lock")
	if err != nil {
		t.Fatalf("Claim duplicate: %v", err)
	}
	if claimed {
		t.Fatalf("expected SET NX duplicate claim to fail")
	}
	ttl, err := client.TTL(ctx, locker.key("task-lock")).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > redisTaskLockTTL {
		t.Fatalf("unexpected lock TTL %s", ttl)
	}
	if err := locker.Release(ctx, "task-lock"); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestExecutorSkipsRedisLockedTaskWithoutDelivery(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	locker := NewRedisTaskLocker(client, "veloxmesh-test")
	defer client.Del(ctx, queue.keyForTest(), locker.key("task-skip"))

	claimed, err := locker.Claim(ctx, "task-skip")
	if err != nil || !claimed {
		t.Fatalf("preclaim lock: claimed=%v err=%v", claimed, err)
	}
	registry := NewResultRegistry()
	registry.RegisterTask(Task{ID: "task-skip"}, func(context.Context) TaskResult {
		t.Fatalf("handler must not execute when Redis lock is already claimed")
		return TaskResult{}
	})
	if err := queue.Push(ctx, QueueItem{TaskID: "task-skip", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	executor := &Executor{Queue: queue, Registry: registry, Locker: locker}
	if err := executor.RunOne(ctx); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := registry.Wait(waitCtx, "task-skip"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected no delivery, got %v", err)
	}
}

func TestFallbackQueueUsesMemoryAfterPrimaryError(t *testing.T) {
	ctx := context.Background()
	badRedis := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond})
	defer badRedis.Close()
	q := NewFallbackQueue(NewRedisQueue(badRedis, "veloxmesh-test", "unavailable"), NewMemoryQueue())
	if err := q.Push(ctx, QueueItem{TaskID: "t1", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	length, err := q.Len(ctx)
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if length != 1 {
		t.Fatalf("Len=%d, want 1", length)
	}
	item, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if item.TaskID != "t1" {
		t.Fatalf("unexpected fallback item: %#v", item)
	}
}

func newRealRedisQueue(t *testing.T) (*redis.Client, *RedisQueue) {
	t.Helper()
	_ = godotenv.Load("../../.env")
	addr := os.Getenv("SCHEDULER_TEST_REDIS_ADDR")
	if addr == "" {
		addr = os.Getenv("REDIS_ADDR")
	}
	if addr == "" && os.Getenv("DEV_SERVER_IP") != "" {
		addr = os.Getenv("DEV_SERVER_IP") + ":6379"
	}
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("real Redis required at %s for scheduler queue tests: %v", addr, err)
	}
	queue := NewRedisQueue(client, "veloxmesh-test", fmt.Sprintf("queue-%d", time.Now().UnixNano()))
	return client, queue
}
