//go:build phase29preflight

package app

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"veloxmesh/internal/cache"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
)

func TestPhase29LocalQueueBurst(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("isolated opt-in queue burst")
	}
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", "on")
	application, token := liveApplication(t, env)
	write := liveBurstWrite(t, application, token)
	recorder := &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: time.Now()}
	previous := observability.DefaultMetrics
	observability.DefaultMetrics = recorder
	defer func() { observability.DefaultMetrics = previous }()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	const candidates = 128
	var accepted int
	started := time.Now()
	for index := 0; index < candidates; index++ {
		if application.semanticCache.Enqueue(cache.CacheWrite{ID: fmt.Sprintf("burst-%d", index), Scope: write.Scope, Model: write.Model, Text: write.Text, Response: write.Response}) {
			accepted++
		}
	}
	enqueueElapsed := time.Since(started)
	if accepted > application.Config.Cache.QueueCapacity+application.Config.Cache.WriteWorkers || accepted == candidates {
		t.Fatalf("queue bound violated: accepted=%d", accepted)
	}
	closing := time.Now()
	application.Close()
	closeElapsed := time.Since(closing)
	runtime.ReadMemStats(&after)
	if closeElapsed > 1500*time.Millisecond {
		t.Fatalf("close exceeded grace allowance: %s", closeElapsed)
	}
	if application.semanticCache.Enqueue(cache.CacheWrite{}) {
		t.Fatal("accepted after close")
	}
	t.Logf("candidates=%d accepted=%d dropped_newest=%d enqueue_ms=%.3f close_ms=%.3f heap_before=%d heap_after=%d", candidates, accepted, candidates-accepted, float64(enqueueElapsed.Microseconds())/1000, float64(closeElapsed.Microseconds())/1000, before.HeapAlloc, after.HeapAlloc)
	liveLogBurstOutcomes(t, recorder)
}

func liveLogBurstOutcomes(t *testing.T, recorder *liveRecorder) {
	t.Helper()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	counts := map[string]int{}
	var collectionConflicts int
	for _, sample := range recorder.samples {
		if sample.Type == "operation" && !sample.OK {
			t.Logf("failed_operation=%s error_category=%s elapsed_ms=%.3f", sample.Name, sample.Error, sample.ElapsedMS)
		}
		if sample.Type == "operation" && sample.Name == "vector_insert" && sample.Error == "AlreadyExists" {
			collectionConflicts++
		}
		if sample.Type == "outcome" {
			counts[sample.Name]++
		}
	}
	t.Logf("reason_counts=%v", counts)
	if collectionConflicts > 0 {
		t.Fatalf("real concurrent collection creation conflicts=%d", collectionConflicts)
	}
}

func liveBurstWrite(t *testing.T, application *App, token string) cache.CacheWrite {
	t.Helper()
	temperature, maxTokens := 0.0, 256
	req := &llm.LLMRequest{Model: os.Getenv("SANS_PRIMARY_DEFAULT_MODEL"), Temperature: &temperature, MaxTokens: &maxTokens, Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveFAQSystem}, {Role: llm.RoleUser, Content: "How long is the trial for plan 1?"}}}
	scope, eligible := application.semanticCache.Eligible(token, "user", req)
	if !eligible {
		t.Fatal("burst request not eligible")
	}
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	response := liveHTTP(t, liveChain{url: server.URL, token: token, model: req.Model}, llm.ChatCompletionRequest{
		Model: req.Model, Temperature: req.Temperature, MaxTokens: req.MaxTokens, Messages: req.Messages})
	completion := liveDecodeChat(t, response)
	if completion.Choices[0].Message.Content == "" {
		t.Fatal("real burst seed response has no text")
	}
	choices, err := json.Marshal(completion.Choices)
	if err != nil {
		t.Fatal(err)
	}
	return cache.CacheWrite{Scope: scope, Model: req.Model, Text: req.Messages[1].Content, Response: string(choices)}
}
