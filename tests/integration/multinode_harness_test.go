package integration

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"veloxmesh/internal/app"
	"veloxmesh/internal/config"

)

type MultiNodeHarness struct {
	mr    *miniredis.Miniredis
	nodes []*Node
}

type Node struct {
	ID          string
	App         *app.App
	Server      *httptest.Server
	DBPath      string
	RedisClient *redis.Client
}

func NewMultiNodeHarness(t *testing.T, nodeCount int) *MultiNodeHarness {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	harness := &MultiNodeHarness{
		mr:    mr,
		nodes: make([]*Node, 0, nodeCount),
	}

	for i := 1; i <= nodeCount; i++ {
		harness.nodes = append(harness.nodes, harness.startNode(t, i))
	}

	return harness
}

func (h *MultiNodeHarness) startNode(t *testing.T, idx int) *Node {
	f, err := os.CreateTemp("", "sqlite-node-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	f.Close()
	dbPath := f.Name()

	cfg := &config.Config{
		NodeID:               "node-" + string(rune('0'+idx)),
		MultiNodeEnabled:     true,
		RedisEnabled:         true,
		RedisAddr:            h.mr.Addr(),
		RedisNamespace:       "test-cluster",
		ControlStateBackend:  "sqlite",
		ControlStateDSN:      dbPath,
		ControlStateMigrateOnStartup: true,
	}

	// To fully instantiate App with real components, we use app.New but we must mock the config load
	// Since app.New loads from env, we will set env vars for this node.
	// But it's easier to build it directly, wait, we can just set env vars and call app.New()
	
	os.Setenv("MULTI_NODE_ENABLED", "true")
	os.Setenv("REDIS_ENABLED", "true")
	os.Setenv("REDIS_ADDR", h.mr.Addr())
	os.Setenv("REDIS_NAMESPACE", "test-cluster")
	os.Setenv("NODE_ID", cfg.NodeID)
	os.Setenv("CONTROL_STATE_BACKEND", "sqlite")
	os.Setenv("CONTROL_STATE_DSN", dbPath)
	os.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	os.Setenv("CONTROL_STATE_ENCRYPTION_KEY", "12345678901234567890123456789012")
	os.Setenv("GATEWAY_DATA_ADDR", ":0")
	os.Setenv("GATEWAY_ADMIN_ADDR", ":0")
	os.Setenv("GATEWAY_METRICS_ADDR", ":0")
	os.Setenv("ADMIN_API_KEY", "test-admin-key")
	
	application, err := app.New()
	if err != nil {
		t.Fatalf("failed to create app: %v", err)
	}

	// Start the application context
	ctx := context.Background()
	go application.Run(ctx) // This will start the components, but we don't need its internal HTTP server.
	
	// We'll use our own httptest.Server to simulate routing.
	server := httptest.NewServer(application.Router)

	rdb := redis.NewClient(&redis.Options{Addr: h.mr.Addr()})

	return &Node{
		ID:          cfg.NodeID,
		App:         application,
		Server:      server,
		DBPath:      dbPath,
		RedisClient: rdb,
	}
}

func (h *MultiNodeHarness) Close() {
	for _, n := range h.nodes {
		if n.Server != nil {
			n.Server.Close()
		}
		if n.App != nil && n.App.ShutdownTracing != nil {
			n.App.ShutdownTracing(context.Background())
		}
		os.Remove(n.DBPath)
	}
	h.mr.Close()
}

func (h *MultiNodeHarness) StopNode(id string) {
	for i, n := range h.nodes {
		if n.ID == id {
			if n.Server != nil {
				n.Server.Close()
			}
			if n.App != nil && n.App.ShutdownTracing != nil {
				n.App.ShutdownTracing(context.Background())
			}
			if n.App != nil && n.App.Coordinator != nil {
				n.App.Coordinator.Stop(context.Background())
			}
			// Remove from list
			h.nodes = append(h.nodes[:i], h.nodes[i+1:]...)
			return
		}
	}
}

func (h *MultiNodeHarness) GetLeader() *Node {
	for _, n := range h.nodes {
		if n.App.Coordinator.IsWritable() {
			return n
		}
	}
	return nil
}

func (h *MultiNodeHarness) GetFollowers() []*Node {
	var followers []*Node
	for _, n := range h.nodes {
		if !n.App.Coordinator.IsWritable() {
			followers = append(followers, n)
		}
	}
	return followers
}

func TestMultiNodeHarness(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	// Wait for election
	var leader *Node
	for i := 0; i < 50; i++ {
		leader = harness.GetLeader()
		if leader != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if leader == nil {
		for _, n := range harness.nodes {
			t.Logf("Node %s: Role %v, Writable: %v, Leader: %s, Error: %s", 
				n.ID, n.App.Coordinator.Snapshot().Role, n.App.Coordinator.Snapshot().Writable, 
				n.App.Coordinator.Snapshot().LeaderID, n.App.Coordinator.Snapshot().DegradedReason)
		}
		t.Fatal("expected to find a leader within 5s")
	}
	
	followers := harness.GetFollowers()
	if len(followers) != 2 {
		t.Fatalf("expected 2 followers, got %d", len(followers))
	}
	
	// Verify distinct SQLite DBs
	if leader.DBPath == followers[0].DBPath {
		t.Fatal("expected distinct DB paths")
	}
}
