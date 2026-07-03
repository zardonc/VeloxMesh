package integration

import (
	"bytes"
	"io"

	"net/http"
	"testing"
	"time"
)

func waitForLeader(t *testing.T, harness *MultiNodeHarness) *Node {
	for i := 0; i < 50; i++ {
		leader := harness.GetLeader()
		if leader != nil {
			return leader
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("expected to find a leader within 5s")
	return nil
}

func TestMultiNodeLeaderLoss(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	leader1 := waitForLeader(t, harness)
	if leader1 == nil {
		t.FailNow()
	}

	// Stop leader to simulate crash/shutdown
	harness.StopNode(leader1.ID)

	// To speed up miniredis lease expiration, we can fast forward
	// Wait, graceful shutdown drops the lock immediately via releaseScript.
	// So a follower will pick it up on its next heartbeat (within 3 seconds).

	leader2 := waitForLeader(t, harness)
	if leader2.ID == leader1.ID {
		t.Fatalf("expected new leader, got same: %s", leader2.ID)
	}
}

func TestMultiNodeRedisOutage(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	leader := waitForLeader(t, harness)

	// Simulate Redis outage by closing the miniredis server
	harness.BreakRedis()

	// Wait a tiny bit, but NOT 4 seconds. The leader still holds the lock for 2 seconds.
	// It thinks it's still the leader. Let's make a write!
	payload := `{"id": "test-prov-fallback", "name": "Fallback Prov", "type": "openai-compatible", "base_url": "http://test", "api_key": "test", "models": ["test-model"]}`
	req, _ := http.NewRequest(http.MethodPost, leader.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201 Created from leader with fallback log, got %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Verify it exists in SQLite on leader
	reqGet, _ := http.NewRequest(http.MethodGet, leader.Server.URL+"/admin/v1/providers/test-prov-fallback", nil)
	reqGet.Header.Set("Authorization", "Bearer test-admin-key")
	respGet, err := http.DefaultClient.Do(reqGet)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	defer respGet.Body.Close()

	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, provider should exist on leader, got %d", respGet.StatusCode)
	}

	// Wait for Redis lease to expire
	time.Sleep(4 * time.Second)

	// Now restore Redis
	harness.RestoreRedis(t)

	// Wait for election to happen again and the recovery worker to flush the fallback log
	time.Sleep(4 * time.Second)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		for _, node := range harness.nodes {
			if node.ID == leader.ID {
				continue
			}
			reqCheck, _ := http.NewRequest(http.MethodGet, node.Server.URL+"/admin/v1/providers/test-prov-fallback", nil)
			reqCheck.Header.Set("Authorization", "Bearer test-admin-key")
			respCheck, err := http.DefaultClient.Do(reqCheck)
			if err == nil && respCheck.StatusCode == http.StatusOK {
				respCheck.Body.Close()
				return
			}
			if respCheck != nil {
				respCheck.Body.Close()
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("expected recovered provider on a node that did not perform the original write")
}

func TestMultiNodeReplication(t *testing.T) {
	harness := NewMultiNodeHarness(t, 2)
	defer harness.Close()

	leader := waitForLeader(t, harness)
	followers := harness.GetFollowers()
	if len(followers) == 0 {
		t.Fatal("expected at least one follower")
	}
	follower := followers[0]

	// Send an admin write to the leader
	payload := `{"id": "repl-prov", "name": "Repl Prov", "type": "openai-compatible", "base_url": "http://test", "api_key": "test", "models": ["repl-model"]}`
	req, _ := http.NewRequest(http.MethodPost, leader.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on leader, got %d", resp.StatusCode)
	}

	// Wait for Redis stream consumer to pick up the change on follower
	// Polling is 100ms in consumer.
	time.Sleep(1 * time.Second)

	// Check if the follower's RuntimeProviderManager got the update
	// We can test this by checking the models endpoint on the follower (which hits HotState / RuntimeProviderManager)
	reqGet, _ := http.NewRequest(http.MethodGet, follower.Server.URL+"/v1/models", nil)
	reqGet.Header.Set("Authorization", "Bearer test-dev-key")
	respGet, err := http.DefaultClient.Do(reqGet)
	if err != nil {
		t.Fatalf("follower get request failed: %v", err)
	}
	defer respGet.Body.Close()

	if respGet.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK from follower /v1/models, got %d", respGet.StatusCode)
	}

	// We expect repl-model to be present
	buf := new(bytes.Buffer)
	buf.ReadFrom(respGet.Body)
	if !bytes.Contains(buf.Bytes(), []byte("repl-model")) {
		t.Fatalf("expected follower to have 'repl-model' in models list, got %s", buf.String())
	}
}
