//go:build phase29preflight

package app

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLiveSemanticDiagnosticPositive(t *testing.T) { liveSemanticDiagnostic(t, true) }
func TestLiveSemanticDiagnosticNegative(t *testing.T) { liveSemanticDiagnostic(t, false) }

func liveSemanticDiagnostic(t *testing.T, positive bool) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	t.Setenv("PHASE29_THRESHOLD", "0.92")
	t.Setenv("PHASE29_MODE", "on")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token,
		model: chain.model, system: liveAcceptanceSystem, question: "How long is the free trial?", origin: time.Now()}
	seed := shipRequest(options)
	if !seed.OK || seed.Hit {
		t.Fatal("real semantic seed failed")
	}
	chain.app.semanticCache.Close()
	var failures, misses, falseHits int
	upstreamRequests := 1
	for index, question := range liveSemanticQuestions(positive) {
		options.index, options.question = index, question
		sample := shipRequest(options)
		shipLogJSON(t, sample)
		failed, missed, falseHit := liveSemanticResult(sample, seed.AnswerHash, positive)
		failures, misses, falseHits = failures+failed, misses+missed, falseHits+falseHit
		if !sample.Hit {
			upstreamRequests++
		}
		if falseHit > 0 {
			t.Logf("different-answer false hit: %s", question)
		}
	}
	liveAssertSettlement(t, chain, upstreamRequests)
	shipLogJSON(t, map[string]any{"type": "semantic_diagnostic", "positive": positive, "misses": misses,
		"false_hits": falseHits, "failed": failures, "embedding_model": os.Getenv("PHASE29_MODEL"), "threshold": 0.92})
	if failures != 0 || misses != 0 || falseHits != 0 {
		t.Fatalf("semantic diagnostic failures=%d misses=%d false_hits=%d", failures, misses, falseHits)
	}
}

func liveSemanticResult(sample shipSample, seed string, positive bool) (failure, miss, falseHit int) {
	if !sample.OK {
		failure = 1
	}
	if positive && (!sample.Hit || sample.AnswerHash != seed) {
		miss = 1
	}
	if !positive && sample.Hit {
		falseHit = 1
	}
	return
}

func liveSemanticQuestions(positive bool) []string {
	questions := []string{
		"How long is the free trial?", "What is the duration of the free trial?", "How many days does the free trial last?",
		"What is the length of the free trial period?", "How long can I use the free trial?", "When does the free trial period end?",
		"For how many days is the free trial available?", "How many days are included in the free trial?",
		"What is the free trial's duration in days?", "How long does the trial last before it expires?",
	}
	if !positive {
		questions = []string{"How long is the refund window?", "When can I request a refund?", "What is the refund deadline?",
			"How many days do I have to request a refund?", "How long does paid access last after cancellation?",
			"When does paid access stop after I cancel?", "What happens to paid access after cancellation?",
			"Can I get a refund after eight days?", "Does cancellation terminate paid access immediately?",
			"Is the refund period fourteen days?", "What is the monthly price?", "Which payment methods are accepted?",
			"How do I change my password?", "Can I extend the free trial to thirty days?", "Can I have a second free trial?",
			"Do I need a credit card to start the free trial?", "How do I cancel the free trial?", "Is the free trial paid?",
			"Are enterprise accounts allowed a free trial?", "Does the free trial include all paid features?"}
	}
	var expanded []string
	for _, suffix := range []string{"", " Please clarify.", " Please answer briefly.", " I would like to know.", " Explain this policy."} {
		for _, question := range questions {
			expanded = append(expanded, strings.TrimSpace(question+suffix))
		}
	}
	return expanded
}
