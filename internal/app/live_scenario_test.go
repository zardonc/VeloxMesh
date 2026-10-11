//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveCacheHitLoad(t *testing.T)    { liveScenario(t, "hit") }
func TestLiveCacheBypassLoad(t *testing.T) { liveScenario(t, "bypass") }
func TestLiveMissBurst(t *testing.T)       { liveScenario(t, "miss_burst") }
func TestLiveCacheLoadOffBare(t *testing.T) {
	t.Setenv("PHASE29_TIMING_DISABLED", "true")
	shipLoad(t, "off")
}
func TestLiveCacheLoadOnBare(t *testing.T) {
	t.Setenv("PHASE29_TIMING_DISABLED", "true")
	shipLoad(t, "on")
}

func liveScenario(t *testing.T, scenario string) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_MODE", "on")
	application, token := liveApplication(t, env)
	recorder := installLiveTiming(t, application, time.Now())
	t.Cleanup(func() { application.Close(); recorder.dump(t) })
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: server.URL, token: token,
		model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: liveWarmupIndex, origin: time.Now()}
	if scenario == "hit" {
		options.question = "How long is the trial for plan 101?"
	}
	if scenario == "bypass" {
		options.system = liveFAQSystem + " Respond in English."
	}
	warm := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "warmup", "sample": warm})
	if !warm.OK {
		t.Fatal("real scenario warmup failed")
	}
	if scenario == "hit" {
		application.semanticCache.Close()
	}
	options.origin = time.Now()
	samples := liveBurstRequests(t, options)
	application.Close()
	liveScenarioResults(t, samples, scenario)
	if scenario == "hit" {
		for _, sample := range samples {
			if sample.AnswerHash != warm.AnswerHash {
				t.Fatal("cache hit changed the real seed answer")
			}
		}
	}
}

func liveBurstRequests(t *testing.T, options liveRequestOptions) []shipSample {
	t.Helper()
	count, concurrency := liveInteger(t, "PHASE29_COUNT"), liveInteger(t, "PHASE29_CLIENT_CONCURRENCY")
	const maxScenarioSamples = 1000
	if count < 1 || count > maxScenarioSamples || concurrency < 1 {
		t.Fatal("invalid real scenario inputs")
	}
	samples := make([]shipSample, count)
	sem, start := make(chan struct{}, concurrency), make(chan struct{})
	var group sync.WaitGroup
	var active atomic.Int64
	for index := 0; index < count; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			sem <- struct{}{}
			defer func() { <-sem }()
			request := options
			request.index, request.concurrent = index+1, active.Add(1)
			defer active.Add(-1)
			samples[index] = shipRequest(request)
		}(index)
	}
	close(start)
	group.Wait()
	return samples
}

func liveScenarioResults(t *testing.T, samples []shipSample, scenario string) {
	t.Helper()
	var failed, hits int
	var peak int64
	for _, sample := range samples {
		shipLogJSON(t, sample)
		if !sample.OK {
			failed++
		}
		if sample.Hit {
			hits++
		}
		if sample.Concurrent > peak {
			peak = sample.Concurrent
		}
	}
	shipLogJSON(t, map[string]any{"type": "scenario_metadata", "scenario": scenario, "count": len(samples), "failed": failed,
		"hits": hits, "actual_client_concurrency": peak, "embedding_model": os.Getenv("PHASE29_MODEL")})
	validHits := liveScenarioHitCount(scenario, hits, len(samples))
	if failed != 0 || !validHits {
		t.Fatalf("real scenario=%s failed=%d hits=%d", scenario, failed, hits)
	}
	if peak != int64(liveInteger(t, "PHASE29_CLIENT_CONCURRENCY")) {
		t.Fatalf("target concurrent load not reached: peak=%d", peak)
	}
}

func liveScenarioHitCount(scenario string, hits, total int) bool {
	switch scenario {
	case "hit":
		return hits == total
	case "bypass", "hit_baseline":
		return hits == 0
	default:
		return true
	}
}
