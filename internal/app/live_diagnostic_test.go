//go:build phase29preflight

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
	"veloxmesh/internal/llm"
)

// Availability is established by a real completion and its persisted settlement.
func TestLiveModelAvailability(t *testing.T) {
	liveModelAvailability(t, 32)
}

func TestLiveModelAvailabilityBudget(t *testing.T) {
	liveModelAvailability(t, 256)
}

func liveModelAvailability(t *testing.T, limit int) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real application opt-in")
	}
	t.Setenv("PHASE29_MODE", "off")
	chain := newLiveChain(t)
	payload := llm.ChatCompletionRequest{Model: chain.model, MaxTokens: &limit,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with OK."}}}
	started := time.Now()
	response := liveHTTP(t, chain, payload)
	var completion llm.ChatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&completion); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		t.Fatal("real completion returned no text")
	}
	liveAssertSettlement(t, chain, 1)
	shipLogJSON(t, map[string]any{"type": "availability", "model": chain.model,
		"elapsed_ms": float64(time.Since(started).Microseconds()) / 1000, "usage": completion.Usage})
}

// Retain upstream error evidence separately from gateway error translation.
func TestLiveRawAvailability(t *testing.T) {
	liveRawAvailability(t, 32)
}

func TestLiveRawAvailabilityBudget(t *testing.T) {
	liveRawAvailability(t, 256)
}

func liveRawAvailability(t *testing.T, limit int) {
	env := liveEnvironment(t)
	payload := llm.ChatCompletionRequest{Model: env["SANS_PRIMARY_DEFAULT_MODEL"], MaxTokens: &limit,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Reply with OK."}}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(env["SANS_BASE_URL"], "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+env["SANS_PRIMARY_API_KEY"])
	req.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: liveHTTPTimeout}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upstream status=%d body=%s", response.StatusCode, data)
	}
	var result llm.ChatCompletionResponse
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) == 0 {
		t.Fatal("upstream returned no choices")
	}
	shipLogJSON(t, map[string]any{"type": "raw_availability", "model": payload.Model, "status": response.StatusCode, "usage": result.Usage,
		"max_tokens": limit, "content_characters": len(result.Choices[0].Message.Content), "finish_reason": result.Choices[0].FinishReason})
}

func TestLiveNativeAvailability(t *testing.T) {
	env := liveEnvironment(t)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: env["SANS_PRIMARY_API_KEY"], Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: env["SANS_BASE_URL"]}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	response, err := client.Models.GenerateContent(ctx, env["SANS_PRIMARY_DEFAULT_MODEL"], genai.Text("Reply with OK."), &genai.GenerateContentConfig{MaxOutputTokens: 32})
	if err != nil {
		t.Fatalf("native upstream error: %v", err)
	}
	if response.Text() == "" {
		t.Fatal("native response has no text")
	}
	shipLogJSON(t, map[string]any{"type": "native_availability", "model": env["SANS_PRIMARY_DEFAULT_MODEL"]})
}

func TestLiveDirectLoad(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real upstream opt-in")
	}
	env := liveEnvironment(t)
	recorder := installLiveTiming(t, nil, time.Now())
	t.Cleanup(func() { recorder.dump(t) })
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: env["SANS_BASE_URL"], token: env["SANS_PRIMARY_API_KEY"],
		model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: liveWarmupIndex, origin: time.Now(), direct: true}
	warm := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "warmup", "sample": warm})
	if !warm.OK {
		t.Fatal("direct upstream warmup failed")
	}
	options.origin = time.Now()
	failures := shipScheduledLoad(t, nil, options)
	if failures > 0 {
		t.Fatalf("direct upstream failed requests=%d", failures)
	}
}

func liveMixedLoad(t *testing.T, mode string) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real distributed load opt-in")
	}
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", mode)
	application, token := liveApplication(t, env)
	recorder := installLiveTiming(t, application, time.Now())
	t.Cleanup(func() { application.Close(); recorder.dump(t) })
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	models := strings.Split(env["PHASE29_MIXED_MODELS"], ",")
	if len(models) < 2 {
		t.Fatal("multiple real models required")
	}
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: server.URL, token: token, model: models[0], models: models, origin: time.Now()}
	for _, model := range models {
		warm := options
		warm.model, warm.index = model, liveWarmupIndex
		sample := shipRequest(warm)
		shipLogJSON(t, map[string]any{"type": "warmup", "sample": sample})
		if !sample.OK {
			t.Fatalf("distributed warmup failed model=%s", model)
		}
	}
	options.origin = time.Now()
	failures := shipScheduledLoad(t, application, options)
	if failures > 0 {
		t.Fatalf("distributed failed requests=%d", failures)
	}
}

func TestLiveMixedLoadOff(t *testing.T) { liveMixedLoad(t, "off") }
func TestLiveMixedLoadOn(t *testing.T)  { liveMixedLoad(t, "on") }

func TestLiveStageAccounting(t *testing.T) {
	chain := newLiveChain(t)
	recorder := chain.timing
	temperature, limit := 0.0, 256
	response := liveHTTP(t, chain, llm.ChatCompletionRequest{Model: chain.model, Temperature: &temperature, MaxTokens: &limit,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveFAQSystem}, {Role: llm.RoleUser, Content: "How long is the trial for plan 101?"}}})
	id := response.Header.Get("X-Request-ID")
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	liveAssertSettlement(t, chain, 1)
	chain.app.Close()
	liveAssertStagePartition(t, recorder, id)
}

func liveAssertStagePartition(t *testing.T, recorder *liveTiming, id string) {
	t.Helper()
	required := []string{"http_handler", "request_rules", "cache_read", "routing", "admission", "provider_complete", "response_rules", "settlement", "write_enqueue", "store_total", "write_queue_wait"}
	durations := map[string]time.Duration{}
	recorder.mu.Lock()
	for _, stage := range recorder.stages {
		if stage.ID == id {
			durations[stage.Name] += time.Duration(stage.ElapsedMS * float64(time.Millisecond))
		}
	}
	recorder.mu.Unlock()
	for _, name := range required {
		if durations[name] <= 0 {
			t.Fatalf("missing real request stage id=%s name=%s", id, name)
		}
	}
	var partition time.Duration
	for _, name := range required[1:9] {
		partition += durations[name]
	}
	const timerTolerance = 5 * time.Millisecond
	if partition > durations["http_handler"]+timerTolerance {
		t.Fatalf("foreground timings overlap: partition=%v handler=%v", partition, durations["http_handler"])
	}
}
