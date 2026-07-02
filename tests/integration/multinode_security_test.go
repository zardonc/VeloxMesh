package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

)

func TestMultiNodeSecurity(t *testing.T) {
	harness := NewMultiNodeHarness(t, 3)
	defer harness.Close()

	leader := waitForLeader(t, harness)
	if len(harness.GetFollowers()) == 0 {
		t.Fatal("no followers")
	}
	follower := harness.GetFollowers()[0]

	forbiddenTerms := []string{
		"leader", "follower", "primary", "replica",
		"node_id", "leader_id", "wal_lag", "writable",
		"degraded_reason", "failover", "topology",
	}

	assertNoForbiddenTerms := func(body string) {
		lowerBody := strings.ToLower(body)
		for _, term := range forbiddenTerms {
			if strings.Contains(lowerBody, term) {
				t.Errorf("found forbidden term %q in body: %s", term, body)
			}
		}
	}

	// 1. Check ordinary /healthz on follower
	resp, err := http.Get(follower.Server.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz get failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assertNoForbiddenTerms(string(body))

	// 2. Check ordinary /readyz on follower (which should fail due to lag if we set it up, but let's check successful first)
	respR, err := http.Get(follower.Server.URL + "/readyz")
	if err != nil {
		t.Fatalf("readyz get failed: %v", err)
	}
	defer respR.Body.Close()
	bodyR, _ := io.ReadAll(respR.Body)
	assertNoForbiddenTerms(string(bodyR))
	
	// Wait, readyz fails immediately because lag > threshold if no events are replicated? No, initially lag is 0.
	
	// 3. Check data-plane write error on follower
	payload := `{"id": "test-sec", "type": "openai-compatible", "base_url": "http://test", "api_key": "test", "models": ["test-model"]}`
	req, _ := http.NewRequest(http.MethodPost, follower.Server.URL+"/admin/v1/providers", bytes.NewBufferString(payload))
	// Add bad auth to simulate a real error? No, use good auth, we just want the error body for a non-writable node.
	req.Header.Set("Authorization", "Bearer test-admin-key")
	req.Header.Set("Content-Type", "application/json")
	
	respErr, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respErr.Body.Close()
	bodyErr, _ := io.ReadAll(respErr.Body)
	assertNoForbiddenTerms(string(bodyErr))
	
	// 4. Verify admin topology endpoint DOES return these terms for an authenticated admin
	reqAdmin, _ := http.NewRequest(http.MethodGet, leader.Server.URL+"/admin/v1/topology", nil)
	reqAdmin.Header.Set("Authorization", "Bearer test-admin-key")
	
	respAdmin, err := http.DefaultClient.Do(reqAdmin)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respAdmin.Body.Close()
	bodyAdmin, _ := io.ReadAll(respAdmin.Body)
	lowerAdmin := strings.ToLower(string(bodyAdmin))
	
	if !strings.Contains(lowerAdmin, "leader") {
		t.Errorf("admin topology missing 'leader'")
	}
	if !strings.Contains(lowerAdmin, "node_id") {
		t.Errorf("admin topology missing 'node_id'")
	}
}
