package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCoordinator struct {
	client    redis.Cmdable
	namespace string
	nodeID    string

	mu             sync.RWMutex
	currentRole    NodeRole
	currentLeader  string
	degradedReason string

	ch      chan struct{}
	stopCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewRedisCoordinator(client redis.Cmdable, namespace, nodeID string) *RedisCoordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &RedisCoordinator{
		client:      client,
		namespace:   namespace,
		nodeID:      nodeID,
		currentRole: RoleUnknown,
		ch:          make(chan struct{}, 1),
		stopCtx:     ctx,
		cancel:      cancel,
	}
}

func (r *RedisCoordinator) lockKey() string {
	if r.namespace == "" {
		return "coordination:leader"
	}
	return fmt.Sprintf("%s:coordination:leader", r.namespace)
}

func (r *RedisCoordinator) nodeKey(nodeID string) string {
	if r.namespace == "" {
		return fmt.Sprintf("coordination:node:%s", nodeID)
	}
	return fmt.Sprintf("%s:coordination:node:%s", r.namespace, nodeID)
}

func (r *RedisCoordinator) Start(ctx context.Context) {
	r.wg.Add(1)
	go r.runLoop()
}

func (r *RedisCoordinator) runLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(LeaderHeartbeatInterval)
	defer ticker.Stop()

	// Initial check
	r.checkLeadership()

	for {
		select {
		case <-r.stopCtx.Done():
			return
		case <-ticker.C:
			r.checkLeadership()
		}
	}
}

var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`)

var renewScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("pexpire", KEYS[1], ARGV[2])
else
    return 0
end
`)

func (r *RedisCoordinator) checkLeadership() {
	lockKey := r.lockKey()

	r.mu.RLock()
	wasLeader := r.currentRole == RoleLeader
	r.mu.RUnlock()

	if wasLeader {
		res, err := renewScript.Run(r.stopCtx, r.client, []string{lockKey}, r.nodeID, LeaderLeaseTTL.Milliseconds()).Result()
		if err != nil {
			r.updateState(RoleUnknown, "", fmt.Sprintf("redis renew error: %v", err))
			return
		}
		if val, ok := res.(int64); ok && val == 1 {
			r.updateState(RoleLeader, r.nodeID, "")
			return
		}
	}

	acquired, err := r.client.SetNX(r.stopCtx, lockKey, r.nodeID, LeaderLeaseTTL).Result()
	if err != nil {
		r.updateState(RoleUnknown, "", fmt.Sprintf("redis setnx error: %v", err))
		return
	}

	if acquired {
		r.updateState(RoleLeader, r.nodeID, "")
		return
	}

	leader, err := r.client.Get(r.stopCtx, lockKey).Result()
	if err != nil {
		if err == redis.Nil {
			r.updateState(RoleUnknown, "", "lock missing during read")
		} else {
			r.updateState(RoleUnknown, "", fmt.Sprintf("redis get error: %v", err))
		}
		return
	}

	r.updateState(RoleFollower, leader, "")
}

func (r *RedisCoordinator) updateState(role NodeRole, leaderID, reason string) {
	r.mu.Lock()
	changed := false
	if r.currentRole != role {
		r.currentRole = role
		changed = true
	}
	if r.currentLeader != leaderID {
		r.currentLeader = leaderID
		changed = true
	}
	if r.degradedReason != reason {
		r.degradedReason = reason
		changed = true
	}
	r.mu.Unlock()

	snap := r.Snapshot()

	// Background registration
	go func(s NodeSnapshot) {
		key := r.nodeKey(r.nodeID)
		data, err := json.Marshal(s)
		if err == nil {
			_ = r.client.Set(r.stopCtx, key, data, LeaderLeaseTTL).Err()
		}
	}(snap)

	if changed {
		select {
		case r.ch <- struct{}{}:
		default:
		}
	}
}

func (r *RedisCoordinator) Stop(ctx context.Context) {
	r.cancel()
	r.wg.Wait()

	r.mu.RLock()
	role := r.currentRole
	r.mu.RUnlock()

	if role == RoleLeader {
		_ = releaseScript.Run(ctx, r.client, []string{r.lockKey()}, r.nodeID).Err()
	}

	_ = r.client.Del(ctx, r.nodeKey(r.nodeID)).Err()
}

func (r *RedisCoordinator) Snapshot() NodeSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return NodeSnapshot{
		NodeID:         r.nodeID,
		Role:           r.currentRole,
		LeaderID:       r.currentLeader,
		Writable:       r.currentRole == RoleLeader,
		LagSummary:     "", 
		DegradedReason: r.degradedReason,
	}
}

func (r *RedisCoordinator) Topology() TopologySnapshot {
	snap := r.Snapshot()
	top := TopologySnapshot{
		Nodes:    make(map[string]NodeSnapshot),
		LeaderID: snap.LeaderID,
	}

	pattern := "coordination:node:*"
	if r.namespace != "" {
		pattern = fmt.Sprintf("%s:coordination:node:*", r.namespace)
	}

	// Use context.Background() in case stopCtx is cancelled, we might still want to inspect topology before final GC
	keys, err := r.client.Keys(context.Background(), pattern).Result()
	if err == nil {
		for _, key := range keys {
			data, err := r.client.Get(context.Background(), key).Bytes()
			if err == nil {
				var n NodeSnapshot
				if err := json.Unmarshal(data, &n); err == nil {
					top.Nodes[n.NodeID] = n
				}
			}
		}
	}

	top.Nodes[r.nodeID] = snap
	return top
}

func (r *RedisCoordinator) IsWritable() bool {
	return r.Snapshot().Writable
}

func (r *RedisCoordinator) WatchChanges() <-chan struct{} {
	return r.ch
}
