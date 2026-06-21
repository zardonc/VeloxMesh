package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/app"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/http/handlers"
)

func TestAdminProviderActions_TestConnectionIdempotencyAndAudit(t *testing.T) {
	var completionCalls int
	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			completionCalls++
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "fake-id",
			"object":  "chat.completion",
			"created": 1234567,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "ok",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer fakeUpstream.Close()

	repo, err := sqlite.Open("file:admin-actions?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite repo: %v", err)
	}
	defer repo.Close()
	if err := sqlite.NewMigrator(repo.DBForTest()).Migrate(context.Background()); err != nil {
		t.Fatalf("migrate sqlite repo: %v", err)
	}

	cfg := &config.Config{
		RoutingStrategy: "round-robin",
		DevAPIKey:       "test-dev-key",
		AdminAPIKey:     "test-admin-key",
	}
	a, err := app.New()
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}
	a.Config = cfg
	cipher, err := controlstate.NewAESGCMSecretCipher([]byte("0123456789abcdef0123456789abcdef"), "test-key")
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	adminService := controlstate.NewAdminProviderService(repo, cipher, a.RuntimeProviderManager, nil)
	adminHandler := handlers.NewAdminProvidersHandler(adminService)
	gatewaySvc := gateway.NewService(a.RuntimeProviderManager, admission.NewPassThroughController(), a.HealthStore(), false, 1)
	a.Router = router.NewRouter(a.Config, gatewaySvc, adminHandler, nil)
	if err := a.ReloadProviders(context.Background(), repo, cipher); err != nil {
		t.Fatalf("initial reload: %v", err)
	}

	server := httptest.NewServer(a.Router)
	defer server.Close()

	createBody := `{"id":"test-action","name":"Test Action","type":"openai-compatible","base_url":"` + fakeUpstream.URL + `","api_key":"sk-action","models":["gpt-4o"]}`
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBufferString(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer test-admin-key")
	createReq.Header.Set("Idempotency-Key", "create-key")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected create 201, got %d", createResp.StatusCode)
	}

	replayReq, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBufferString(createBody))
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("Authorization", "Bearer test-admin-key")
	replayReq.Header.Set("Idempotency-Key", "create-key")
	replayResp, err := http.DefaultClient.Do(replayReq)
	if err != nil {
		t.Fatalf("create replay request: %v", err)
	}
	defer replayResp.Body.Close()
	if replayResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected create replay 201, got %d", replayResp.StatusCode)
	}

	conflictBody := `{"id":"test-action-2","name":"Test Action","type":"openai-compatible","base_url":"` + fakeUpstream.URL + `","api_key":"sk-action","models":["gpt-4o"]}`
	conflictReq, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBufferString(conflictBody))
	conflictReq.Header.Set("Content-Type", "application/json")
	conflictReq.Header.Set("Authorization", "Bearer test-admin-key")
	conflictReq.Header.Set("Idempotency-Key", "create-key")
	conflictResp, err := http.DefaultClient.Do(conflictReq)
	if err != nil {
		t.Fatalf("conflict request: %v", err)
	}
	defer conflictResp.Body.Close()
	if conflictResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected idempotency conflict 409, got %d", conflictResp.StatusCode)
	}

	testReq, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers/test-action/test-connection", nil)
	testReq.Header.Set("Authorization", "Bearer test-admin-key")
	testReq.Header.Set("Idempotency-Key", "test-key")
	testResp, err := http.DefaultClient.Do(testReq)
	if err != nil {
		t.Fatalf("test connection request: %v", err)
	}
	defer testResp.Body.Close()
	if testResp.StatusCode != http.StatusOK {
		t.Fatalf("expected test connection 200, got %d", testResp.StatusCode)
	}
	var testBody map[string]interface{}
	if err := json.NewDecoder(testResp.Body).Decode(&testBody); err != nil {
		t.Fatalf("decode test response: %v", err)
	}
	if testBody["provider_id"] != "test-action" || testBody["ok"] != true {
		t.Fatalf("unexpected test connection body: %v", testBody)
	}
	if _, leaked := testBody["api_key"]; leaked {
		t.Fatalf("test connection leaked api_key: %v", testBody)
	}
	if completionCalls != 0 {
		t.Fatalf("test connection sent chat completion request")
	}

	events, err := repo.Audit().List(context.Background(), "test-action")
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected create and test audit events, got %d", len(events))
	}
	seenCreate, seenTest := false, false
	for _, event := range events {
		seenCreate = seenCreate || event.Action == "provider.create"
		seenTest = seenTest || event.Action == "provider.test_connection"
		if bytes.Contains(event.Metadata, []byte("sk-action")) {
			t.Fatalf("audit metadata leaked secret: %s", string(event.Metadata))
		}
	}
	if !seenCreate || !seenTest {
		t.Fatalf("expected create and test audit events, got %#v", events)
	}

	purged, err := repo.Audit().PurgeOld(context.Background(), "2999-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("purge audit: %v", err)
	}
	if purged < int64(len(events)) {
		t.Fatalf("expected purge to remove audit events, purged %d of %d", purged, len(events))
	}
	if rec, err := repo.Idempotency().Get(context.Background(), "create-key"); err != nil || rec == nil {
		t.Fatalf("audit purge should not remove idempotency record, rec=%v err=%v", rec, err)
	}
}
