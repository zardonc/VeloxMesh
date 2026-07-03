package coordination

import (
	"context"
	"sync"
)

// FakeCluster simulates a Redis cluster for testing leader election.
type FakeCluster struct {
	mu       sync.Mutex
	leaderID string
	nodes    map[string]*FakeCoordinator
}

func NewFakeCluster() *FakeCluster {
	return &FakeCluster{
		nodes: make(map[string]*FakeCoordinator),
	}
}

func (c *FakeCluster) elect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If current leader is still registered and active, do nothing
	if c.leaderID != "" {
		if _, ok := c.nodes[c.leaderID]; ok {
			return
		}
	}

	// Otherwise, pick the first one as leader
	c.leaderID = ""
	for id := range c.nodes {
		c.leaderID = id
		break
	}

	// Notify all nodes
	for _, n := range c.nodes {
		n.notify()
	}
}

func (c *FakeCluster) register(n *FakeCoordinator) {
	c.mu.Lock()
	c.nodes[n.nodeID] = n
	c.mu.Unlock()
	c.elect()
}

func (c *FakeCluster) unregister(id string) {
	c.mu.Lock()
	delete(c.nodes, id)
	wasLeader := c.leaderID == id
	if wasLeader {
		c.leaderID = ""
	}
	c.mu.Unlock()

	if wasLeader {
		c.elect()
	}
}

func (c *FakeCluster) getTopology() TopologySnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := TopologySnapshot{
		Nodes:    make(map[string]NodeSnapshot),
		LeaderID: c.leaderID,
	}

	for id := range c.nodes {
		role := RoleFollower
		writable := false
		if id == c.leaderID {
			role = RoleLeader
			writable = true
		}
		snap.Nodes[id] = NodeSnapshot{
			NodeID:   id,
			Role:     role,
			LeaderID: c.leaderID,
			Writable: writable,
		}
	}
	return snap
}

// FakeCoordinator implements Coordinator for in-process tests.
type FakeCoordinator struct {
	cluster *FakeCluster
	nodeID  string
	ch      chan struct{}
}

func NewFakeCoordinator(cluster *FakeCluster, nodeID string) *FakeCoordinator {
	return &FakeCoordinator{
		cluster: cluster,
		nodeID:  nodeID,
		ch:      make(chan struct{}, 1),
	}
}

func (f *FakeCoordinator) Start(ctx context.Context) {
	f.cluster.register(f)
}

func (f *FakeCoordinator) Stop(ctx context.Context) {
	f.cluster.unregister(f.nodeID)
}

func (f *FakeCoordinator) notify() {
	select {
	case f.ch <- struct{}{}:
	default:
	}
}

func (f *FakeCoordinator) Snapshot() NodeSnapshot {
	top := f.cluster.getTopology()
	if snap, ok := top.Nodes[f.nodeID]; ok {
		return snap
	}
	return NodeSnapshot{
		NodeID:   f.nodeID,
		Role:     RoleUnknown,
		Writable: false,
	}
}

func (f *FakeCoordinator) Topology() TopologySnapshot {
	return f.cluster.getTopology()
}

func (f *FakeCoordinator) IsWritable() bool {
	return f.Snapshot().Writable
}

func (f *FakeCoordinator) WatchChanges() <-chan struct{} {
	return f.ch
}
