package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"veloxmesh/internal/app"
	"veloxmesh/internal/llm"
)

func setupFakeProvider(t *testing.T, id string, latency time.Duration, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(latency)
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}

		resp := llm.ChatCompletionResponse{
			ID:      "fake-id",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o",
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.Message{
						Role:    llm.RoleAssistant,
						Content: fmt.Sprintf("Response from %s", id),
					},
					FinishReason: "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

func writeConfig(t *testing.T, p1, p2 *httptest.Server, strategy string) string {
	configJSON := fmt.Sprintf(`{
		"routing_strategy": "%s",
		"default_provider": "p1",
		"providers": [
			{
				"id": "p1",
				"type": "openai-compatible",
				"base_url": "%s",
				"api_key": "test-key",
				"models": ["gpt-4o"]
			},
			{
				"id": "p2",
				"type": "openai-compatible",
				"base_url": "%s",
				"api_key": "test-key",
				"models": ["gpt-4o"]
			}
		]
	}`, strategy, p1.URL, p2.URL)

	f, err := os.CreateTemp("", "config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(configJSON))
	f.Close()
	return f.Name()
}

func doChatReq(t *testing.T, a *app.App, override string) (*httptest.ResponseRecorder, llm.ChatCompletionResponse) {
	chatReq := llm.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: "Hello"},
		},
	}
	body, _ := json.Marshal(chatReq)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+a.Config.DevAPIKey)
	if override != "" {
		req.Header.Set("X-Route-To", override)
	}

	rec := httptest.NewRecorder()
	a.Router.ServeHTTP(rec, req)

	var resp llm.ChatCompletionResponse
	if rec.Code == http.StatusOK {
		json.NewDecoder(rec.Body).Decode(&resp)
	}
	return rec, resp
}

func TestChatCompletions_MultiProvider(t *testing.T) {
	p1 := setupFakeProvider(t, "p1", 10*time.Millisecond, http.StatusOK)
	defer p1.Close()
	p2 := setupFakeProvider(t, "p2", 50*time.Millisecond, http.StatusOK)
	defer p2.Close()

	cfgPath := writeConfig(t, p1, p2, "round-robin")
	defer os.Remove(cfgPath)
	os.Setenv("CONFIG_FILE", cfgPath)
	defer os.Unsetenv("CONFIG_FILE")

	application, err := app.New()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("RoundRobin", func(t *testing.T) {
		rec1, _ := doChatReq(t, application, "")
		rec2, _ := doChatReq(t, application, "")

		if rec1.Code != http.StatusOK || rec2.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d and %d", rec1.Code, rec2.Code)
		}

		prov1 := rec1.Header().Get("X-Provider")
		prov2 := rec2.Header().Get("X-Provider")

		if prov1 == prov2 {
			t.Errorf("expected round robin to alternate, got %s twice", prov1)
		}
	})

	t.Run("Override", func(t *testing.T) {
		rec, resp := doChatReq(t, application, "p2")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if rec.Header().Get("X-Provider") != "p2" {
			t.Errorf("expected provider p2, got %s", rec.Header().Get("X-Provider"))
		}
		if resp.Choices[0].Message.Content != "Response from p2" {
			t.Errorf("expected response from p2, got %s", resp.Choices[0].Message.Content)
		}
	})

	t.Run("Unhealthy Override", func(t *testing.T) {
		// make p1 unhealthy
		p1Fail := setupFakeProvider(t, "p1", 0, http.StatusInternalServerError)
		defer p1Fail.Close()

		cfgPathFail := writeConfig(t, p1Fail, p2, "round-robin")
		defer os.Remove(cfgPathFail)
		os.Setenv("CONFIG_FILE", cfgPathFail)
		appFail, _ := app.New()

		// 3 failures to make it unhealthy
		for i := 0; i < 3; i++ {
			doChatReq(t, appFail, "p1")
		}

		rec, _ := doChatReq(t, appFail, "p1")
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 for unhealthy override, got %d", rec.Code)
		}
	})
}
