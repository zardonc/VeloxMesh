package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"veloxmesh/internal/app"
	"veloxmesh/internal/health"
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

	cfgPath := writeConfig(t, pFail, pFail, "round-robin")
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
	res := application.Prober.ProbeProvider(importContext, "p1")
	if !res.Available {
		t.Errorf("expected probe to be available")
	}

	// 4. p1 becomes routeable again.
	snap = application.HealthStore().Snapshot("p1")
	if snap.Status != health.StatusHealthy {
		t.Errorf("expected p1 to be healthy after probe, got %s", snap.Status)
	}

	// Gateway should route again, even if the upstream still returns 500
	rec, _ = doChatReq(t, application, "p1")
	// The request will be routed, upstream returns 500, gateway maps to 502
	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusInternalServerError {
		t.Errorf("expected upstream error mapped to 502, got %d", rec.Code)
	}
}
