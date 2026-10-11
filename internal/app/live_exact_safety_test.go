//go:build phase29preflight

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This is a seen regression set, not a new independent semantic-quality set.
// Exact reuse must reject paraphrases as well as different-answer traps.
func TestLiveExactCacheSafetyRegression(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_REUSE_MODE", "exact")
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url,
		token: chain.token, model: chain.model, system: liveAcceptanceSystem, origin: time.Now()}
	hashes := liveHoldoutSeeds(t, chain, options)
	chain.app.semanticCache.Close()
	for index, entry := range liveBusinessFAQCases() {
		request := options
		request.question = entry.question
		sample := shipRequest(request)
		if !sample.OK || !sample.Hit || sample.AnswerHash != hashes[index] {
			t.Fatal("same question did not preserve its exact seeded answer")
		}
	}
	questions := liveExactRegressionQuestions()
	for _, question := range questions {
		request := options
		request.question = question
		sample := shipRequest(request)
		shipLogJSON(t, map[string]any{"type": "exact_regression", "question": question, "expected_reuse": false, "sample": sample})
		if !sample.OK || sample.Hit {
			t.Fatal("changed question reused an exact answer")
		}
	}
	if calls := livePolicyEmbeddingCalls(chain.timing); calls != 0 {
		t.Fatalf("exact answer path invoked embedding: %d", calls)
	}
	liveAssertSettlement(t, chain, len(hashes)+len(questions))
	shipLogJSON(t, map[string]any{"type": "exact_safety_regression", "changed_questions": len(questions), "false_hits": 0,
		"identical_hits": len(hashes), "independent_semantic_quality": false})
}

func liveExactRegressionQuestions() []string {
	questions := make([]string, 0, len(liveHoldoutGold())+9)
	for _, entry := range liveHoldoutGold() {
		questions = append(questions, entry.question)
	}
	return append(questions, "Are refunds unavailable within seven days?", "Can I get a refund after seven weeks?",
		"Can I get a refund before seven hours?", "Does the enterprise product offer a seven-day refund?",
		"Is a refund guaranteed if I used all paid features?", "Is the fourteen-day trial also the refund period?",
		"七天之后可以退款吗？", "第八天仍然允许退款吗？", "退款不是七天之内才可以吗？")
}

func TestLiveExactCacheVersionIsolation(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_REUSE_MODE", "exact")
	repo, dsn, keyID := liveRepository(t)
	database := liveDatabase{dsn: dsn, keyID: keyID}
	var scopes []string
	for index, version := range []string{"exact-faq-v1", "exact-faq-v2", "exact-faq-v1"} {
		t.Setenv("PHASE29_KNOWLEDGE_VERSION", version)
		application, _ := liveApplicationWithDatabase(t, env, database)
		recorder := installLiveTiming(t, application, time.Now())
		server := httptest.NewServer(application.Router)
		chain := liveChain{app: application, repo: repo, url: server.URL, token: keyID,
			model: env["SANS_PRIMARY_DEFAULT_MODEL"], timing: recorder}
		scopes = append(scopes, liveEmbeddingLifecycle(t, chain, index == 2))
		if livePolicyEmbeddingCalls(recorder) != 0 {
			t.Fatal("exact scope lifecycle called embedding")
		}
		server.Close()
		application.Close()
		recorder.dump(t)
	}
	if scopes[0] == scopes[1] || scopes[0] != scopes[2] || liveUsageCount(t, liveChain{repo: repo, token: keyID}) != 2 {
		t.Fatal("exact version scope or seed billing did not isolate and restore")
	}
	shipLogJSON(t, map[string]any{"type": "exact_version_isolation", "isolated": true, "restored": true, "settlements": 2})
}
