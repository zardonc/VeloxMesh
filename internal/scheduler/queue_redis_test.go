package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
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

func TestExecutorRunsRealRedisQueuedTask(t *testing.T) {
	ctx := context.Background()
	client, queue := newRealRedisQueue(t)
	defer client.Del(ctx, queue.keyForTest())

	registry := NewResultRegistry()
	registry.RegisterTask(Task{ID: "task-run"}, func(context.Context) TaskResult {
		return TaskResult{Response: "ok"}
	})
	if err := queue.Push(ctx, QueueItem{TaskID: "task-run", Score: 1}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	executor := &Executor{Queue: queue, Registry: registry}
	if err := executor.RunOne(ctx); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	result, err := registry.Wait(ctx, "task-run")
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.Response != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestFallbackQueueUsesMemoryWhenRealRedisPopFailsAfterPeek(t *testing.T) {
	ctx := context.Background()
	client, redisQueue := newRealRedisQueue(t)
	opts := *client.Options()
	cleanup := redis.NewClient(&opts)
	defer cleanup.Close()
	defer cleanup.Del(ctx, redisQueue.keyForTest())

	if err := redisQueue.Push(ctx, QueueItem{TaskID: "primary", Score: 1}); err != nil {
		t.Fatalf("Push primary: %v", err)
	}
	fallback := NewMemoryQueue()
	if err := fallback.Push(ctx, QueueItem{TaskID: "fallback", Score: 2}); err != nil {
		t.Fatalf("Push fallback: %v", err)
	}
	primary := &closeAfterPeekQueue{QueueBackend: redisQueue, close: client.Close}
	queue := NewFallbackQueue(primary, fallback)

	got, err := queue.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin: %v", err)
	}
	if got.TaskID != "fallback" {
		t.Fatalf("expected fallback item after Redis pop failure, got %#v", got)
	}
	if queue.primaryAvailable.Load() {
		t.Fatalf("expected primary to be marked unavailable after Redis pop failure")
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

func TestFallbackQueueUsesMemoryAfterRuntimeRedisFailure(t *testing.T) {
	ctx := context.Background()
	client, redisQueue := newRealRedisQueue(t)
	opts := *client.Options()
	cleanup := redis.NewClient(&opts)
	defer cleanup.Close()
	defer cleanup.Del(ctx, redisQueue.keyForTest())

	q := NewFallbackQueue(redisQueue, NewMemoryQueue())
	if err := q.Push(ctx, QueueItem{TaskID: "before-failure", Score: 1}); err != nil {
		t.Fatalf("Push before failure: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close Redis client: %v", err)
	}
	if _, err := q.PopMin(ctx); !errors.Is(err, ErrQueueEmpty) {
		t.Fatalf("expected empty fallback after Redis pop failure, got %v", err)
	}
	if err := q.Push(ctx, QueueItem{TaskID: "after-failure", Score: 1}); err != nil {
		t.Fatalf("Push after failure: %v", err)
	}
	item, err := q.PopMin(ctx)
	if err != nil {
		t.Fatalf("PopMin fallback item: %v", err)
	}
	if item.TaskID != "after-failure" {
		t.Fatalf("unexpected fallback item after runtime failure: %#v", item)
	}
}

func TestFallbackQueueDoesNotSerializePrimaryPushIO(t *testing.T) {
	primary := &blockingQueue{entered: make(chan struct{}, 2), release: make(chan struct{})}
	queue := NewFallbackQueue(primary, NewMemoryQueue())
	done := make(chan error, 2)

	for _, id := range []string{"a", "b"} {
		go func(taskID string) {
			done <- queue.Push(context.Background(), QueueItem{TaskID: taskID, Score: 1})
		}(id)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-primary.entered:
		case <-time.After(50 * time.Millisecond):
			close(primary.release)
			t.Fatalf("primary Push calls were serialized before entering the primary queue")
		}
	}
	close(primary.release)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("Push: %v", err)
		}
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

type blockingQueue struct {
	entered chan struct{}
	release chan struct{}
	count   atomic.Int32
}

func (q *blockingQueue) Push(context.Context, QueueItem) error {
	q.count.Add(1)
	q.entered <- struct{}{}
	<-q.release
	return nil
}

func (q *blockingQueue) PeekMin(context.Context, int) ([]QueueItem, error) {
	return nil, ErrQueueEmpty
}

func (q *blockingQueue) PopMin(context.Context) (QueueItem, error) {
	return QueueItem{}, ErrQueueEmpty
}

func (q *blockingQueue) Remove(context.Context, string) error {
	return ErrTaskNotFound
}

func (q *blockingQueue) Len(context.Context) (int64, error) {
	return int64(q.count.Load()), nil
}

type closeAfterPeekQueue struct {
	QueueBackend
	close  func() error
	closed bool
}

func (q *closeAfterPeekQueue) PeekMin(ctx context.Context, limit int) ([]QueueItem, error) {
	items, err := q.QueueBackend.PeekMin(ctx, limit)
	if err == nil && !q.closed {
		q.closed = true
		_ = q.close()
	}
	return items, err
}
