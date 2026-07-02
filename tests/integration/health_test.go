package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/app"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/replication"
	"veloxmesh/internal/coordination"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/http/handlers"
)

func TestHealthEndpoints(t *testing.T) {
	p1 := setupFakeProvider(t, "p1", 0, http.StatusOK)
	defer p1.Close()

	cfgPath := writeConfig(t, p1, p1, "round-robin")
	defer os.Remove(cfgPath)
	os.Setenv("CONFIG_FILE", cfgPath)
	defer os.Unsetenv("CONFIG_FILE")

	application, _ := app.New()

	t.Run("healthz returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("readyz behavior", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp["status"] != "ready" {
			t.Errorf("expected status ready, got %v", resp["status"])
		}

		// Assert capabilities are included and safe
		providers := resp["providers"].([]interface{})
		if len(providers) == 0 {
			t.Fatal("expected providers in readyz")
		}

		p1 := providers[0].(map[string]interface{})
		if p1["id"] != "p1" && p1["id"] != "p2" {
			t.Errorf("expected p1 or p2, got %v", p1["id"])
		}

		caps, ok := p1["capabilities"].(map[string]interface{})
		if !ok {
			t.Fatal("expected capabilities in readyz provider")
		}

		if caps["provider_type"] != "openai-compatible" {
			t.Errorf("expected provider_type openai-compatible, got %v", caps["provider_type"])
		}
		if caps["streaming"] != true {
			t.Errorf("expected streaming true, got %v", caps["streaming"])
		}

		// Ensure no secrets
		for _, p := range providers {
			pMap := p.(map[string]interface{})
			if _, hasKey := pMap["api_key"]; hasKey {
				t.Error("found api_key in readyz")
			}
			if _, hasKey := pMap["base_url"]; hasKey {
				t.Error("found base_url in readyz")
			}
		}
	})

	t.Run("readyz unavailable when all unhealthy", func(t *testing.T) {
		pFail := setupFakeProvider(t, "p-fail", 0, 500)
		defer pFail.Close()

		cfgPathFail := writeConfig(t, pFail, pFail, "round-robin")
		defer os.Remove(cfgPathFail)
		os.Setenv("CONFIG_FILE", cfgPathFail)
		appFail, _ := app.New()

		// make them unhealthy
		for i := 0; i < 3; i++ {
			doChatReq(t, appFail, "p1")
			doChatReq(t, appFail, "p2")
		}

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		appFail.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", rec.Code)
		}
	})
}

func TestHealthRecovery(t *testing.T) {
	// 1. provider p1 becomes unhealthy after failures.
	pFail := setupFakeProvider(t, "p-fail", 0, 500)
	defer pFail.Close()

	configJSON := fmt.Sprintf(`{
		"routing_strategy": "round-robin",
		"health_check": {
			"interval": "10ms",
			"failure_threshold": 3
		},
		"providers": [
			{
				"id": "p1",
				"type": "openai-compatible",
				"base_url": "%s",
				"api_key": "test-key",
				"models": ["gpt-4o"]
			}
		]
	}`, pFail.URL)

	f, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(configJSON))
	f.Close()
	cfgPath := f.Name()

	defer os.Remove(cfgPath)
	os.Setenv("CONFIG_FILE", cfgPath)
	defer os.Unsetenv("CONFIG_FILE")

	application, _ := app.New()

	for i := 0; i < 3; i++ {
		doChatReq(t, application, "p1")
	}

	snap := application.HealthStore().Snapshot("p1")
	if snap.Status != health.StatusUnhealthy {
		t.Fatalf("expected p1 to be unhealthy, got %s", snap.Status)
	}

	// 2. router avoids p1 while unhealthy
	// But in this test we only have p1.
	// Wait, doChatReq uses the router.
	rec, _ := doChatReq(t, application, "p1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 from gateway due to no routeable providers, got %d", rec.Code)
	}

	// 3. ProbeProvider(ctx, "p1") succeeds.
	importContext := context.Background()
	rpmSnap := application.RuntimeProviderManager.Snapshot()
	if rpmSnap == nil || rpmSnap.Prober == nil {
		t.Fatal("prober not available in snapshot")
	}
	res := rpmSnap.Prober.ProbeProvider(importContext, "p1")
	if !res.Available {
		t.Errorf("expected probe to be available")
	}

	// 4. p1 becomes routeable again.
	snap = application.HealthStore().Snapshot("p1")
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected p1 to be healthy after probe, got %s", snap.Status)
	}

	time.Sleep(15 * time.Millisecond)

	// Gateway should route again, even if the upstream still returns 500
	rec, _ = doChatReq(t, application, "p1")
	// The request will be routed, upstream returns 500, gateway maps to 502
	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected upstream error mapped to 502, got %d", rec.Code)
	}
}

type fakeLagReporter struct {
	elapsed time.Duration
	pending int64
}

func (f *fakeLagReporter) ReportLag() replication.LagSnapshot {
	return replication.LagSnapshot{
		Elapsed: f.elapsed,
		Pending: f.pending,
	}
}

func TestTopologyEndpoint(t *testing.T) {
	cluster := coordination.NewFakeCluster()
	leader := coordination.NewFakeCoordinator(cluster, "node-1")
	leader.Start(context.Background())
	defer leader.Stop(context.Background())

	lagReporter := &fakeLagReporter{elapsed: 1 * time.Second, pending: 0}
	
	handler := handlers.Topology(leader, lagReporter)

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/topology", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["role"] != "leader" {
		t.Errorf("expected role leader, got %v", resp["role"])
	}
	if resp["writable"] != true {
		t.Errorf("expected writable true, got %v", resp["writable"])
	}
}

func TestReadyzLagThreshold(t *testing.T) {
	cluster := coordination.NewFakeCluster()
	leader := coordination.NewFakeCoordinator(cluster, "node-1")
	leader.Start(context.Background())
	defer leader.Stop(context.Background())

	follower := coordination.NewFakeCoordinator(cluster, "node-2")
	follower.Start(context.Background())
	defer follower.Stop(context.Background())

	// Follower is lagged
	lagReporter := &fakeLagReporter{elapsed: 10 * time.Second, pending: 0}
	
	// mock svc for Readyz
	cfg := &config.Config{}
	
	healthStore := health.NewInMemoryStore()
	rpm := controlstate.NewRuntimeProviderManager(cfg, nil, healthStore)
	svc := gateway.NewService(rpm, nil, healthStore, false, 0, nil, nil, nil, rpm, nil)
	
	handler := handlers.Readyz(cfg, svc, follower, lagReporter)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for lagged follower, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["status"] != "unavailable" {
		t.Errorf("expected status unavailable, got %v", resp["status"])
	}
	if _, ok := resp["node_id"]; ok {
		t.Errorf("expected topology to not leak in readyz, found node_id")
	}
	if _, ok := resp["role"]; ok {
		t.Errorf("expected topology to not leak in readyz, found role")
	}
}
