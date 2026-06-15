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

		if len(resp.Data) != 1 {
			t.Errorf("expected 1 unique model, got %d", len(resp.Data))
		} else if resp.Data[0].ID != "gpt-4o" {
			t.Errorf("expected gpt-4o, got %s", resp.Data[0].ID)
		}
	})
}
