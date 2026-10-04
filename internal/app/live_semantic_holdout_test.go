//go:build phase29preflight

package app

import (
	"net/http"
	"os"
	"testing"
	"time"
)

type liveGoldQuestion struct {
	question string
	answer   int // -1 means no seeded answer is safe to reuse.
}

// Frozen independent phrasings; neither suffix-expanded nor selected by scores.
func liveHoldoutGold() []liveGoldQuestion {
	return []liveGoldQuestion{
		{"State the number of days in the complimentary trial period.", 0},
		{"What is the time limit on trying the service at no charge?", 0},
		{"After starting my free evaluation, how many days until it ends?", 0},
		{"Tell me the standard trial length, expressed in days.", 0},
		{"What is the maximum number of days after payment to obtain a refund?", 1},
		{"State the reimbursement window in days.", 1},
		{"How soon must a customer submit their money-back request?", 1},
		{"How long after paying am I still eligible for my money back?", 1},
		{"If I cancel my subscription now, when will my paid access expire?", 2},
		{"Does cancelling stop access today or at the end of my billing cycle?", 2},
		{"For a cancelled subscription, describe when access finally ends.", 2},
		{"Until what point can I keep using paid features after cancelling?", 2},
		{"Can I receive a refund on day ten after paying?", -1},
		{"Is a refund request on the eighth day permitted?", -1},
		{"Is my trial already expired on day fifteen?", -1},
		{"Is the free trial exactly seven days long?", -1},
		{"Can I extend the trial from fourteen to twenty-one days?", -1},
		{"Are refunds available for fourteen days?", -1},
		{"Can I cancel without losing my money?", -1},
		{"Does cancellation give me a refund automatically?", -1},
		{"How do I send a cancellation request?", -1},
		{"How can I apply for a refund?", -1},
		{"What is the subscription price per month?", -1},
		{"Is a payment card needed for the complimentary trial?", -1},
		{"Am I allowed to start another free trial?", -1},
		{"Will the free trial automatically turn into a paid subscription?", -1},
		{"Are enterprise trials different from the standard plan?", -1},
		{"Which features are excluded during evaluation?", -1},
		{"Do you return all charges or only some charges?", -1},
		{"Can I pause the billing cycle instead of cancelling?", -1},
		{"What happens to stored files when access ends?", -1},
		{"Where do I update my payment method?", -1},
	}
}

func TestLiveSemanticIndependentHoldout(t *testing.T) {
	env := liveEnvironment(t)
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	t.Setenv("PHASE29_MODE", "on")
	chain := newLiveChainWithEnvironment(t, env)
	options := liveRequestOptions{client: &http.Client{Timeout: liveHTTPTimeout}, url: chain.url, token: chain.token,
		model: chain.model, system: liveAcceptanceSystem, origin: time.Now()}
	seeds := liveHoldoutSeeds(t, chain, options)
	chain.app.semanticCache.Close()
	var hits, falseHits, failures, positives, billed int
	for index, entry := range liveHoldoutGold() {
		options.index, options.question = index, entry.question
		sample := shipRequest(options)
		shipLogJSON(t, sample)
		expected := ""
		if entry.answer >= 0 {
			positives++
			expected = seeds[entry.answer]
		}
		hit, unsafe, failed, charge := liveHoldoutResult(sample, expected)
		hits, falseHits, failures, billed = hits+hit, falseHits+unsafe, failures+failed, billed+charge
		if unsafe > 0 {
			shipLogJSON(t, map[string]any{"type": "holdout_false_hit", "question": entry.question, "index": index})
		}
	}
	liveAssertSettlement(t, chain, len(seeds)+billed)
	shipLogJSON(t, map[string]any{"type": "independent_holdout", "positive_count": positives, "positive_hits": hits,
		"negative_count": len(liveHoldoutGold()) - positives, "false_hits": falseHits, "failed": failures,
		"input_prefix": os.Getenv("PHASE29_INPUT_PREFIX"), "threshold": os.Getenv("PHASE29_THRESHOLD"),
		"embedding_model": os.Getenv("PHASE29_MODEL"), "main_model_fact_quality_checked": false})
	if failures != 0 || falseHits != 0 {
		t.Fatalf("holdout HTTP failures=%d unsafe hits=%d", failures, falseHits)
	}
}

func liveHoldoutResult(sample shipSample, expected string) (hit, unsafe, failed, billed int) {
	if !sample.OK {
		failed = 1
	}
	if !sample.Hit {
		billed = 1
		return
	}
	if expected == "" || sample.AnswerHash != expected {
		unsafe = 1
		return
	}
	hit = 1
	return
}

func liveHoldoutSeeds(t *testing.T, chain liveChain, options liveRequestOptions) []string {
	t.Helper()
	var hashes []string
	for index, entry := range liveBusinessFAQCases() {
		options.index, options.question = index, entry.question
		seed := shipRequest(options)
		shipLogJSON(t, map[string]any{"type": "holdout_seed", "sample": seed})
		if !seed.OK || seed.Hit {
			t.Fatal("independent holdout seed failed or reused different FAQ")
		}
		hashes = append(hashes, seed.AnswerHash)
		liveWaitStoreCount(t, chain.timing, index+1)
	}
	return hashes
}
