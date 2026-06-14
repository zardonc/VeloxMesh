package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"veloxmesh/internal/app"
	"veloxmesh/internal/llm"
)

func TestChatCompletions(t *testing.T) {
	application := app.New()

	t.Run("unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("missing messages", func(t *testing.T) {
		body := []byte(`{"model": "gpt-4o"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("valid request without provider override handles errors gracefully", func(t *testing.T) {
		chatReq := llm.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []llm.Message{
				{Role: llm.RoleUser, Content: "Hello"},
			},
		}
		body, _ := json.Marshal(chatReq)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
		rec := httptest.NewRecorder()

		application.Router.ServeHTTP(rec, req)

		// Since we don't have a real upstream or fake, it will likely return 502 Bad Gateway
		// or another error depending on if the base URL is reachable.
		if rec.Code != http.StatusBadGateway && rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 502 or 500, got %d", rec.Code)
		}
	})
}
