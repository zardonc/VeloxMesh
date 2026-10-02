//go:build phase29preflight

package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"veloxmesh/internal/llm"
)

const liveAcceptanceSystem = "Static FAQ: the free trial lasts 14 days. Refunds are allowed within 7 days. Cancellation keeps paid access until the billing period ends. Answer in one short sentence."

func liveWaitStored(t *testing.T, application *App, database liveDatabase, question string) {
	t.Helper()
	temperature, maxTokens := 0.0, 256
	req := &llm.LLMRequest{Model: os.Getenv("SANS_PRIMARY_DEFAULT_MODEL"), Temperature: &temperature, MaxTokens: &maxTokens, Messages: []llm.Message{{Role: llm.RoleSystem, Content: liveAcceptanceSystem}, {Role: llm.RoleUser, Content: question}}}
	scope, eligible := application.semanticCache.Eligible(database.keyID, "user", req)
	if !eligible {
		t.Fatal("acceptance request not eligible")
	}
	deadline := time.Now().Add(3 * time.Second)
	encoded, err := json.Marshal(req.Messages)
	if err != nil {
		t.Fatal(err)
	}
	for time.Now().Before(deadline) {
		entry, lookupErr := application.semanticCache.Lookup(context.Background(), scope, req.Model, string(encoded))
		if lookupErr == nil && entry != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("asynchronous vector/repository persistence not observed")
}

func TestPhase29LocalGatewayAcceptance(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("isolated opt-in acceptance")
	}
	env := liveEnvironment(t)
	t.Setenv("PHASE29_SYSTEM", liveAcceptanceSystem)
	t.Setenv("PHASE29_THRESHOLD", "0.92")
	t.Setenv("PHASE29_MODE", "on")
	_, dsn, keyID := liveRepository(t)
	database := liveDatabase{dsn: dsn, keyID: keyID}
	application, token := liveApplicationWithDatabase(t, env, database)
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: &http.Client{Timeout: 10 * time.Second}, url: server.URL, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], system: liveAcceptanceSystem, origin: time.Now()}
	options.question = "How long is the free trial?"
	first := liveRequest(options)
	if !first.OK || first.Hit {
		t.Fatalf("first response: %+v", first)
	}
	liveWaitStored(t, application, database, options.question)
	options.question = "What is the duration of the free trial?"
	positive := liveRequest(options)
	if !positive.OK || !positive.Hit || positive.AnswerHash != first.AnswerHash {
		t.Fatalf("paraphrase: %+v", positive)
	}
	t.Logf("positive paraphrase 1/1 hit, complete_response_ms=%.3f, identical_answer=true", positive.ElapsedMS)
	for _, question := range []string{"How long is the refund window?", "How long does paid access last after cancellation?"} {
		options.question = question
		negative := liveRequest(options)
		if !negative.OK || negative.Hit {
			t.Fatalf("negative: %+v", negative)
		}
	}
	t.Log("different-answer negatives 2/2 miss")
	application.Close()
	server.Close()
	liveVerifyVersionSwitch(t, env, database)
}

func liveVerifyVersionSwitch(t *testing.T, env map[string]string, database liveDatabase) {
	t.Helper()
	t.Setenv("PHASE29_KNOWLEDGE_VERSION", "faq-v2")
	versionTwo, _ := liveApplicationWithDatabase(t, env, database)
	secondServer := httptest.NewServer(versionTwo.Router)
	defer secondServer.Close()
	options := liveRequestOptions{client: &http.Client{Timeout: 10 * time.Second}, url: secondServer.URL, token: database.keyID, model: env["SANS_PRIMARY_DEFAULT_MODEL"], system: liveAcceptanceSystem, question: "How long is the free trial?", origin: time.Now()}
	switched := liveRequest(options)
	if !switched.OK || switched.Hit {
		t.Fatalf("version switch: %+v", switched)
	}
	t.Log("same authenticated key, model and repository: faq-v1 -> faq-v2 old entry missed immediately")
}
