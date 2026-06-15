package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"veloxmesh/internal/app"
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
