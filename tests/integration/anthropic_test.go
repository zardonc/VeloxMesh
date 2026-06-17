package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"veloxmesh/internal/app"
)

func TestChatCompletions_Anthropic(t *testing.T) {
	fakeAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}

		auth := r.Header.Get("x-api-key")
		if auth != "test-anthropic-key" {
			t.Errorf("unexpected api key: %s", auth)
		}

		resp := map[string]interface{}{
			"id":    "msg_123",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-3-haiku",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "Hello from fake anthropic",
				},
			},
			"stop_reason": "end_turn",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeAnthropic.Close()

	// Use writeConfig from chat_test.go, but we need custom config for Anthropic
	configJSON := `{"routing_strategy": "round-robin", "default_provider": "anthro-1", "providers": [
		{
			"id": "anthro-1",
			"type": "anthropic",
			"base_url": "` + fakeAnthropic.URL + `",
			"api_key": "test-anthropic-key",
			"models": ["claude-3-haiku"]
		}
	]}`
	f, _ := os.CreateTemp("", "config-anthro-*.json")
	f.Write([]byte(configJSON))
	f.Close()
	defer os.Remove(f.Name())

	os.Setenv("CONFIG_FILE", f.Name())
	defer os.Unsetenv("CONFIG_FILE")

	application, err := app.New()
	if err != nil {
		t.Fatal(err)
	}

	rec, resp := doChatReqModel(t, application, "anthro-1", "claude-3-haiku")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("X-Provider") != "anthro-1" {
		t.Errorf("expected provider anthro-1, got %s", rec.Header().Get("X-Provider"))
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "Hello from fake anthropic" {
		t.Errorf("unexpected response content: %+v", resp.Choices)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
}
