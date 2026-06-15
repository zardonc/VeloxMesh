package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"veloxmesh/internal/app"
)

func TestChatCompletions_Gemini(t *testing.T) {
	fakeGemini := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("x-goog-api-key")
		if auth != "test-gemini-key" {
			t.Errorf("unexpected api key: %s", auth)
		}

		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"role": "model",
						"parts": []map[string]interface{}{
							{
								"text": "Hello from fake gemini",
							},
						},
					},
					"finishReason": "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeGemini.Close()

	configJSON := `{"routing_strategy": "round-robin", "default_provider": "gemini-1", "providers": [
		{
			"id": "gemini-1",
			"type": "gemini",
			"base_url": "` + fakeGemini.URL + `",
			"api_key": "test-gemini-key",
			"models": ["gemini-1.5-pro"]
		}
	]}`
	f, _ := os.CreateTemp("", "config-gemini-*.json")
	f.Write([]byte(configJSON))
	f.Close()
	defer os.Remove(f.Name())

	os.Setenv("CONFIG_FILE", f.Name())
	defer os.Unsetenv("CONFIG_FILE")

	application, err := app.New()
	if err != nil {
		t.Fatal(err)
	}

	rec, resp := doChatReq(t, application, "gemini-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Provider") != "gemini-1" {
		t.Errorf("expected provider gemini-1, got %s", rec.Header().Get("X-Provider"))
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "Hello from fake gemini" {
		t.Errorf("unexpected response content: %+v", resp.Choices)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
}
