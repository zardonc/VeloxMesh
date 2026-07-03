package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type topologyResponse struct {
	NodeID         string `json:"node_id"`
	Role           string `json:"role"`
	LeaderID       string `json:"leader_id"`
	Writable       bool   `json:"writable"`
	DegradedReason string `json:"degraded_reason"`
	WALLagElapsed  int64  `json:"wal_lag_elapsed"`
	WALLagPending  int64  `json:"wal_lag_pending"`
}

// getTopology queries the topology endpoint using the admin key.
func getTopology(t *testing.T, url string) topologyResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url+"/admin/v1/topology", nil)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed to get topology from %s: %v", url, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("topology endpoint returned status %d", resp.StatusCode)
	}
	
	var topo topologyResponse
	if err := json.NewDecoder(resp.Body).Decode(&topo); err != nil {
		t.Fatalf("failed to decode topology response: %v", err)
	}
	return topo
}

// discoverWritableNode acts as the LB script to find the current writable node.
func discoverWritableNode(t *testing.T, harness *MultiNodeHarness) *Node {
	t.Helper()
	for _, n := range harness.nodes {
		topo := getTopology(t, n.Server.URL)
		if topo.Writable {
			return n
		}
	}
	return nil
}

func TestMultiNodeLeaderAwareAdminWriteRouting(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	// Wait for a leader to be elected in the harness
	_ = waitForLeader(t, harness)

	// LB script discovering the writable node
	writableNode := discoverWritableNode(t, harness)
	if writableNode == nil {
		t.Fatal("LB could not discover a writable node")
	}

	// Attempt mutative admin write (POST /admin/v1/providers)
	payload := `{"id": "test-provider-lb", "name": "Test LB", "type": "openai-compatible", "base_url": "http://test", "api_key": "test", "models": ["test-model"]}`
	req, _ := http.NewRequest(http.MethodPost, writableNode.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected successful write on writable node, got status %d: %s", resp.StatusCode, body)
	}
}

func TestMultiNodeLeaderAwareRetryAfterFailover(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	_ = waitForLeader(t, harness)

	// Initial LB discovery
	initialWritableNode := discoverWritableNode(t, harness)
	if initialWritableNode == nil {
		t.Fatal("initial LB discovery failed")
	}

	// 1. Kill the current writable node to force a failover
	harness.StopNode(initialWritableNode.ID)

	// Wait for failover election (the test harness has waitForLeader)
	newLeader := waitForLeader(t, harness)
	if newLeader == nil {
		t.Fatal("failed to elect new leader")
	}

	// 2. Simulate stale LB state: Admin client attempts to write to a surviving follower
	// because it still thinks the old node is leader (or it randomizes and hits a follower).
	// We'll intentionally hit a known follower.
	followers := harness.GetFollowers()
	if len(followers) == 0 {
		t.Fatal("no surviving followers")
	}
	staleTarget := followers[0]

	payload := `{"id": "test-provider-lb-stale", "name": "Test LB Stale", "type": "openai-compatible", "base_url": "http://test", "api_key": "test", "models": ["test-model"]}`
	req, _ := http.NewRequest(http.MethodPost, staleTarget.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to stale target failed: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from follower, got %d", resp.StatusCode)
	}
	
	bodyErr, _ := io.ReadAll(resp.Body)
	assertNoForbiddenTerms(t, string(bodyErr))
	
	// 3. Admin Client gets 503, triggers bounded topology refresh
	newWritableNode := discoverWritableNode(t, harness)
	if newWritableNode == nil {
		t.Fatal("LB failed to discover new writable node after failover")
	}
	if newWritableNode.ID == initialWritableNode.ID {
		t.Fatalf("LB found old leader %s again", newWritableNode.ID)
	}

	// 4. Retry against the new writable node
	reqRetry, _ := http.NewRequest(http.MethodPost, newWritableNode.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	reqRetry.Header.Set("Authorization", "Bearer test-admin-key")
	reqRetry.Header.Set("Content-Type", "application/json")

	respRetry, err := http.DefaultClient.Do(reqRetry)
	if err != nil {
		t.Fatalf("retry request failed: %v", err)
	}
	defer respRetry.Body.Close()

	if respRetry.StatusCode != http.StatusCreated && respRetry.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(respRetry.Body)
		t.Fatalf("expected successful retry write on new leader, got status %d: %s", respRetry.StatusCode, body)
	}
}
