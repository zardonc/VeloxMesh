//go:build phase29preflight

package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers/openai"
)

func TestLiveCacheHitSettlementLoad(t *testing.T) { liveHitBillingLoad(t, true) }
func TestLiveCacheHitBaseline(t *testing.T) {
	t.Setenv("PHASE29_MODE", "off")
	liveHitBillingLoad(t, false)
}

func liveHitBillingLoad(t *testing.T, cacheEnabled bool) {
	chain := newLiveChain(t)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token,
		model: chain.model, question: "How long is the trial for plan 101?", origin: time.Now()}
	seed := shipRequest(options)
	if !seed.OK || seed.Hit {
		t.Fatal("hit settlement seed failed")
	}
	scenario, billed := "hit", 1
	if cacheEnabled {
		chain.app.semanticCache.Close()
	} else {
		scenario = "hit_baseline"
	}
	options.origin = time.Now()
	samples := liveBurstRequests(t, options)
	liveScenarioResults(t, samples, scenario)
	if cacheEnabled {
		for _, sample := range samples {
			if sample.AnswerHash != seed.AnswerHash {
				t.Fatal("hit settlement changed real answer")
			}
		}
	} else {
		billed += len(samples)
	}
	liveAssertSettlement(t, chain, billed)
	shipLogJSON(t, map[string]any{"type": "hit_settlement", "cached_requests": len(samples), "billed_requests": billed, "cache_enabled": cacheEnabled})
}

func TestLiveBusinessFAQResponses(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	t.Setenv("PHASE29_THRESHOLD", "0.92")
	t.Setenv("PHASE29_MODE", "on")
	chain := newLiveChainWithEnvironment(t, env)
	cases := liveBusinessFAQCases()
	for index, entry := range cases {
		request := liveFAQPayload(chain.model, entry.question)
		request = llm.ChatCompletionRequest{Model: request.Model, Temperature: request.Temperature, MaxTokens: request.MaxTokens,
			Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveAcceptanceSystem}, request.Messages[1]}}
		response := liveHTTP(t, chain, request)
		if response.Header.Get("X-Cache-Hit") == "true" {
			t.Fatal("different FAQ answer incorrectly reused")
		}
		seed := liveDecodeChat(t, response)
		answer := seed.Choices[0].Message.Content
		if !strings.Contains(strings.ToLower(answer), entry.answerFragment) {
			t.Fatalf("real model business fact mismatch: question=%s answer=%s", entry.question, answer)
		}
		liveWaitStoreCount(t, chain.timing, index+1)
		hit := liveHTTP(t, chain, request)
		if hit.Header.Get("X-Cache-Hit") != "true" {
			t.Fatal("business FAQ seed failed to hit")
		}
		cached := liveDecodeChat(t, hit)
		if cached.Choices[0].Message.Content != answer || cached.Usage.TotalTokens != 0 {
			t.Fatal("business FAQ hit changed answer or usage")
		}
	}
	liveAssertSettlement(t, chain, len(cases))
	shipLogJSON(t, map[string]any{"type": "business_faq", "facts": len(cases), "cache_pairs": len(cases), "settlement": true})
}

type liveBusinessFAQCase struct{ question, answerFragment string }

func liveBusinessFAQCases() []liveBusinessFAQCase {
	return []liveBusinessFAQCase{
		{"How many days does the free trial last?", "14"},
		{"Within how many days are refunds allowed?", "7"},
		{"When does paid access end after cancellation?", "billing period"},
	}
}

func TestLiveDirectBusinessFAQResponses(t *testing.T) {
	env := liveEnvironment(t)
	adapter := openai.NewAdapter("direct-faq", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], env["SANS_PRIMARY_DEFAULT_MODEL"])
	for index, entry := range liveBusinessFAQCases() {
		request := liveFAQPayload(env["SANS_PRIMARY_DEFAULT_MODEL"], entry.question)
		ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
		response, err := adapter.Complete(ctx, &llm.LLMRequest{Model: request.Model, Temperature: request.Temperature, MaxTokens: request.MaxTokens,
			Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveAcceptanceSystem}, request.Messages[1]}})
		cancel()
		if err != nil {
			t.Errorf("real direct FAQ: %v", err)
			continue
		}
		if response == nil || len(response.Choices) != 1 {
			t.Error("invalid direct FAQ response")
			continue
		}
		answer := response.Choices[0].Message.Content
		correct := strings.Contains(strings.ToLower(answer), entry.answerFragment)
		shipLogJSON(t, map[string]any{"type": "direct_business_faq", "index": index, "model": request.Model, "answer": answer, "correct": correct})
		if !correct {
			t.Errorf("direct model business fact mismatch: question=%s answer=%s", entry.question, answer)
		}
	}
}

func liveWaitStoreCount(t *testing.T, recorder *liveTiming, count int) {
	deadline := time.Now().Add(liveRecoveryWait)
	for liveOutcomeCount(recorder, "store/stored") < count && time.Now().Before(deadline) {
		time.Sleep(livePollInterval)
	}
	if liveOutcomeCount(recorder, "store/stored") != count {
		t.Fatal("expected business FAQ persistence missing")
	}
}
