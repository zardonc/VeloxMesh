package coordination

import (
	"context"
	"time"
)

type NodeRole string

const (
	RoleLeader   NodeRole = "leader"
	RoleFollower NodeRole = "follower"
	RoleUnknown  NodeRole = "unknown"
)

const (
	LeaderLeaseTTL          = 10 * time.Second
	LeaderHeartbeatInterval = 3 * time.Second
)

type NodeSnapshot struct {
	NodeID         string   `json:"node_id"`
	Role           NodeRole `json:"role"`
	LeaderID       string   `json:"leader_id"`
	Writable       bool     `json:"writable"`
	LagSummary     string   `json:"lag_summary"`
	DegradedReason string   `json:"degraded_reason"`
}

type TopologySnapshot struct {
	Nodes    map[string]NodeSnapshot `json:"nodes"`
	LeaderID string                  `json:"leader_id"`
}

type Coordinator interface {
	Start(ctx context.Context)
	Stop(ctx context.Context)
	Snapshot() NodeSnapshot
	Topology() TopologySnapshot
	IsWritable() bool
	WatchChanges() <-chan struct{}
}

type NoopCoordinator struct{}

func NewNoopCoordinator() *NoopCoordinator {
	return &NoopCoordinator{}
}

func (n *NoopCoordinator) Start(ctx context.Context) {}

func (n *NoopCoordinator) Stop(ctx context.Context) {}

func (n *NoopCoordinator) Snapshot() NodeSnapshot {
	return NodeSnapshot{
		NodeID:   "standalone",
		Role:     RoleLeader,
		LeaderID: "standalone",
		Writable: true,
	}
}

func (n *NoopCoordinator) Topology() TopologySnapshot {
	snap := n.Snapshot()
	return TopologySnapshot{
		Nodes:    map[string]NodeSnapshot{"standalone": snap},
		LeaderID: "standalone",
	}
}

func (n *NoopCoordinator) IsWritable() bool {
	return true
}

func (n *NoopCoordinator) WatchChanges() <-chan struct{} {
	ch := make(chan struct{})
	// never signals because standalone state never changes
	return ch
}
