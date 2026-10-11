//go:build phase29preflight

package app

import (
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// Failure cases: wrong profile/schedule; failed or already cached seed; missing
// persistence; HTTP failure, wrong hit flag/hash, extra upstream work/settlement.
// These scheduled windows are distinct from the existing burst hit benchmarks.
const (
	controlledHitIntervalMS  = 125
	controlledHitConcurrency = 4
	controlledHitQuestion    = "How long is the trial for plan 101?"
	controlledBypassSuffix   = " Respond in English."
)

type controlledHitScenario struct {
	name, mode, reuseMode string
	hit, bypass           bool
}

type controlledHitExpectation struct {
	scenario controlledHitScenario
	seed     shipSample
	timing   *liveTiming
}

type controlledHitCounts struct {
	failed, hits, wrongHash, upstream int
	peak                              int64
}

func TestLiveControlledSameQuestionOff(t *testing.T) {
	controlledHitLoad(t, controlledHitScenario{name: "same_question_off", mode: "off", reuseMode: "semantic"})
}

func TestLiveControlledSemanticHit(t *testing.T) {
	controlledHitLoad(t, controlledHitScenario{name: "semantic_hit", mode: "on", reuseMode: "semantic", hit: true})
}

func TestLiveControlledExactHit(t *testing.T) {
	controlledHitLoad(t, controlledHitScenario{name: "exact_hit", mode: "on", reuseMode: "exact", hit: true})
}

func TestLiveControlledBypass(t *testing.T) {
	controlledHitLoad(t, controlledHitScenario{name: "bypass", mode: "on", reuseMode: "semantic", bypass: true})
}

func controlledHitLoad(t *testing.T, scenario controlledHitScenario) {
	env := liveEnvironment(t)
	configureControlledHit(t, scenario)
	count := liveInteger(t, "PHASE29_COUNT")
	chain := newLiveChainWithEnvironment(t, env)
	controlledHitProfile(t, chain, scenario)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token,
		model: chain.model, system: liveFAQSystem, question: controlledHitQuestion, index: liveWarmupIndex, origin: time.Now()}
	if scenario.bypass {
		options.system = liveFAQSystem + controlledBypassSuffix
	}
	seed := shipRequest(options)
	shipLogJSON(t, map[string]any{"type": "warmup", "scenario": scenario.name, "sample": seed})
	if !seed.OK || seed.Hit {
		t.Fatal("scheduled seed failed or hit an unrelated answer; measurement not started")
	}
	if scenario.hit {
		liveWaitStoreCount(t, chain.timing, 1)
	}
	liveAssertSettlement(t, chain, 1)
	time.Sleep(controlledQuietPeriod)
	shipLogJSON(t, map[string]any{"type": "seed_ready", "seed_persisted": scenario.hit,
		"seed_settlements": 1, "quiet_ms": controlledQuietPeriod.Milliseconds()})
	expectation := controlledHitExpectation{scenario: scenario, seed: seed, timing: chain.timing}
	options.onSamples = func(samples []shipSample) { controlledHitSamples(t, samples, expectation) }
	shipLogJSON(t, map[string]any{"type": "measurement_start", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	options.origin = time.Now()
	failures := shipScheduledLoad(t, chain.app, options)
	shipLogJSON(t, map[string]any{"type": "measurement_end", "utc": time.Now().UTC().Format(time.RFC3339Nano)})
	expectedSettlements := 1
	if !scenario.hit {
		expectedSettlements += count
	}
	liveAssertSettlement(t, chain, expectedSettlements)
	shipLogJSON(t, map[string]any{"type": "hit_settlement", "scenario": scenario.name,
		"measured_requests": count, "billed_requests": expectedSettlements,
		"new_settlements": expectedSettlements - 1})
	if failures != 0 {
		t.Fatalf("scheduled load failed requests=%d; samples retained", failures)
	}
}

func configureControlledHit(t *testing.T, scenario controlledHitScenario) {
	t.Helper()
	if os.Getenv("PHASE29_TIMING_DISABLED") == "true" {
		t.Fatal("scheduled hit/bypass evidence requires timing")
	}
	configureControlledProfile(t, controlledProfile{mode: scenario.mode, capacity: "0", reuseMode: scenario.reuseMode})
	t.Setenv("PHASE29_SYSTEM", liveFAQSystem)
	t.Setenv("PHASE29_INTERVAL_MS", strconv.Itoa(controlledHitIntervalMS))
	t.Setenv("PHASE29_CLIENT_CONCURRENCY", strconv.Itoa(controlledHitConcurrency))
	t.Setenv("PHASE29_PURE_MISS", "true") // Disable index reuse; the fixed question defines these arms.
}

func controlledHitProfile(t *testing.T, chain liveChain, scenario controlledHitScenario) {
	t.Helper()
	profile := chain.app.Config.Cache
	if profile.Enabled != (scenario.mode == "on") || profile.EmbeddingMemoCapacity != 0 || len(profile.UseCases) == 0 {
		t.Fatal("scheduled cache profile mismatch")
	}
	if profile.UseCases[0].ReuseMode != scenario.reuseMode || profile.UseCases[0].SystemPrompt != liveFAQSystem {
		t.Fatal("scheduled reuse mode or allowed system prompt mismatch")
	}
	shipLogJSON(t, map[string]any{"type": "effective_cache_profile", "scenario": scenario.name, "mode": scenario.mode,
		"reuse_mode": profile.UseCases[0].ReuseMode, "memo_capacity": profile.EmbeddingMemoCapacity,
		"enabled": profile.Enabled, "threshold": profile.Threshold,
		"primary_model": chain.model, "embedding_model": profile.EmbeddingModel, "input_prefix": profile.EmbeddingInputPrefix,
		"count": liveInteger(t, "PHASE29_COUNT"), "interval_ms": controlledHitIntervalMS, "max_concurrency": controlledHitConcurrency,
		"bypass_system_mismatch": scenario.bypass})
}

func controlledHitSamples(t *testing.T, samples []shipSample, expected controlledHitExpectation) {
	t.Helper()
	counts := countControlledHitSamples(samples, expected)
	count := liveInteger(t, "PHASE29_COUNT")
	expectedHits := 0
	if expected.scenario.hit {
		expectedHits = count
	}
	shipLogJSON(t, map[string]any{"type": "scenario_metadata", "scenario": expected.scenario.name, "count": len(samples),
		"failed": counts.failed, "hits": counts.hits, "hash_mismatches": counts.wrongHash, "unexpected_upstream": counts.upstream,
		"actual_client_concurrency": counts.peak, "expected_hits": expectedHits})
	if len(samples) != count || counts.failed != 0 || counts.hits != expectedHits || counts.wrongHash != 0 || counts.upstream != 0 {
		t.Errorf("scheduled scenario=%s count=%d failed=%d hits=%d expected=%d hash_mismatches=%d upstream=%d",
			expected.scenario.name, len(samples), counts.failed, counts.hits, expectedHits, counts.wrongHash, counts.upstream)
	}
	if counts.peak < 1 || counts.peak > controlledHitConcurrency {
		t.Errorf("scheduled concurrency ceiling violated: peak=%d limit=%d", counts.peak, controlledHitConcurrency)
	}
}

func countControlledHitSamples(samples []shipSample, expected controlledHitExpectation) controlledHitCounts {
	var failed, hits, wrongHash, upstream int
	var peak int64
	for _, sample := range samples {
		if !sample.OK {
			failed++
		}
		if sample.Hit {
			hits++
		}
		if expected.scenario.hit && sample.AnswerHash != expected.seed.AnswerHash {
			wrongHash++
		}
		if expected.scenario.hit {
			upstream += liveStageCounts(expected.timing, sample.RequestID)["upstream_headers"]
		}
		if sample.Concurrent > peak {
			peak = sample.Concurrent
		}
	}
	return controlledHitCounts{failed: failed, hits: hits, wrongHash: wrongHash, upstream: upstream, peak: peak}
}
