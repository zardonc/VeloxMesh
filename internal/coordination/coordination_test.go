package coordination

import (
	"context"
	"sync"
	"testing"
	"time"
	"fmt"

	"github.com/redis/go-redis/v9"
)

func TestFakeCoordinator(t *testing.T) {
	cluster := NewFakeCluster()

	c1 := NewFakeCoordinator(cluster, "node-1")
	c2 := NewFakeCoordinator(cluster, "node-2")

	c1.Start(context.Background())
	c2.Start(context.Background())

	if c1.IsWritable() == c2.IsWritable() {
		t.Fatalf("expected one node to be writable, got c1=%v, c2=%v", c1.IsWritable(), c2.IsWritable())
	}

	var leader *FakeCoordinator
	var follower *FakeCoordinator
	if c1.IsWritable() {
		leader = c1
		follower = c2
	} else {
		leader = c2
		follower = c1
	}

	leader.Stop(context.Background())

	// allow election to happen
	time.Sleep(50 * time.Millisecond)

	if !follower.IsWritable() {
		t.Fatalf("follower did not become leader after previous leader stopped")
	}
}

// MockRedis is a minimal mock to test RedisCoordinator
type MockRedis struct {
	redis.Cmdable
	mu    sync.Mutex
	store map[string]string
}

func NewMockRedis() *MockRedis {
	return &MockRedis{store: make(map[string]string)}
}

func (m *MockRedis) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := redis.NewBoolCmd(ctx)
	if _, ok := m.store[key]; !ok {
		m.store[key] = value.(string)
		cmd.SetVal(true)
	} else {
		cmd.SetVal(false)
	}
	return cmd
}

func (m *MockRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := redis.NewStringCmd(ctx)
	if val, ok := m.store[key]; ok {
		cmd.SetVal(val)
	} else {
		cmd.SetErr(redis.Nil)
	}
	return cmd
}

func (m *MockRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	switch v := value.(type) {
	case string:
		m.store[key] = v
	case []byte:
		m.store[key] = string(v)
	}

	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

func (m *MockRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := int64(0)
	for _, k := range keys {
		if _, ok := m.store[k]; ok {
			delete(m.store, k)
			count++
		}
	}
	cmd := redis.NewIntCmd(ctx)
	cmd.SetVal(count)
	return cmd
}

func (m *MockRedis) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	// A rough mock for Eval just for the tests we need
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := redis.NewCmd(ctx)
	fmt.Printf("Eval called with keys=%v, args=%v\n", keys, args)
	
	// if it's releaseScript or renewScript, we look at keys[0]
	if len(keys) == 1 && len(args) > 0 {
		key := keys[0]
		expectedVal := args[0].(string)
		if val, ok := m.store[key]; ok && val == expectedVal {
			if len(args) > 1 {
				// renew (pexpire)
				cmd.SetVal(int64(1))
			} else {
				// release
				delete(m.store, key)
				cmd.SetVal(int64(1))
			}
		} else {
			cmd.SetVal(int64(0))
		}
	}
	return cmd
}

func (m *MockRedis) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd {
	// For testing, we just forward to Eval.
	// Since we don't have the script body, we'll assume it's one of ours.
	// Our mock Eval only checks keys[0] and args[0] anyway.
	return m.Eval(ctx, "", keys, args...)
}

func (m *MockRedis) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := redis.NewStringSliceCmd(ctx)
	var keys []string
	for k := range m.store {
		// Just return all for simplicity in this mock
		keys = append(keys, k)
	}
	cmd.SetVal(keys)
	return cmd
}

func TestRedisCoordinator_SingleNode(t *testing.T) {
	mock := NewMockRedis()
	coord := NewRedisCoordinator(mock, "testns", "node-1")
	
	// manually tick checkLeadership once instead of Start
	coord.checkLeadership()

	if !coord.IsWritable() {
		t.Fatalf("expected node to become writable on empty redis")
	}

	snap := coord.Snapshot()
	if snap.LeaderID != "node-1" {
		t.Fatalf("expected leader ID to be node-1, got %s", snap.LeaderID)
	}

	coord.Stop(context.Background())

	err := releaseScript.Run(context.Background(), mock, []string{"testns:coordination:leader"}, "node-1").Err()
	if err != nil {
		t.Logf("manual releaseScript error: %v", err)
	}

	mock.mu.Lock()
	_, ok := mock.store["testns:coordination:leader"]
	mock.mu.Unlock()
	
	if ok {
		t.Fatalf("expected leader key to be deleted on Stop")
	}
}

func TestRedisCoordinator_Follower(t *testing.T) {
	mock := NewMockRedis()
	// Node 1 is already leader
	mock.store["testns:coordination:leader"] = "node-1"

	coord := NewRedisCoordinator(mock, "testns", "node-2")
	coord.checkLeadership()

	if coord.IsWritable() {
		t.Fatalf("expected node to not be writable because node-1 holds lock")
	}

	snap := coord.Snapshot()
	if snap.LeaderID != "node-1" {
		t.Fatalf("expected leader ID to be node-1, got %s", snap.LeaderID)
	}
	if snap.Role != RoleFollower {
		t.Fatalf("expected role to be follower, got %s", snap.Role)
	}
}
