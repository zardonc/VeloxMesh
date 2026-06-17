package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"veloxmesh/internal/app"
	"veloxmesh/internal/http/handlers"
)

func TestModelsEndpoint(t *testing.T) {
	p1 := setupFakeProvider(t, "p1", 0, http.StatusOK)
	defer p1.Close()
	p2 := setupFakeProvider(t, "p2", 0, http.StatusOK)
	defer p2.Close()

	cfgPath := writeConfig(t, p1, p2, "round-robin")
	defer os.Remove(cfgPath)
	os.Setenv("CONFIG_FILE", cfgPath)
	defer os.Unsetenv("CONFIG_FILE")

	application, _ := app.New()

	t.Run("list models", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}

		var resp handlers.ModelsResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp.Object != "list" {
			t.Errorf("expected object list, got %s", resp.Object)
		}

		if len(resp.Data) != 3 {
			t.Errorf("expected 3 unique models, got %d", len(resp.Data))
		}

		foundGPT4o := false
		foundP1Only := false
		foundP2Only := false

		for _, m := range resp.Data {
			if m.ID == "gpt-4o" {
				foundGPT4o = true
			} else if m.ID == "p1-only" {
				foundP1Only = true
			} else if m.ID == "p2-only" {
				foundP2Only = true
			}
		}

		if !foundGPT4o || !foundP1Only || !foundP2Only {
			t.Errorf("expected gpt-4o, p1-only, p2-only in models response, got %v", resp.Data)
		}
	})
}
