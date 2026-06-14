package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"veloxmesh/internal/app"
)

func TestHealthEndpoints(t *testing.T) {
	application := app.New()

	t.Run("healthz returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("readyz behavior", func(t *testing.T) {
		// Mock a valid configuration for readyz to pass
		application.Config.PrimaryAPIKey = "test-api-key"

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if application.Config.DefaultProvider == "" {
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("expected 503, got %d", rec.Code)
			}
		} else {
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
		}
	})
}
