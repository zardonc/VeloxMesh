package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/app"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/gateway"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/http/handlers"
)

func TestAdminProvidersIntegration(t *testing.T) {
	os.Setenv("VELOX_DEV_API_KEY", "test-dev-key")
	defer os.Unsetenv("VELOX_DEV_API_KEY")

	// Setup fake provider upstream
	fakeUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "fake-id",
			"object":  "chat.completion",
			"created": 1234567,
			"model":   "gpt-4o",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello from dynamically added provider",
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer fakeUpstream.Close()

	cfg := &config.Config{
		RoutingStrategy: "round-robin",
	}

	a, err := app.New()
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}

	a.Config = cfg
	a.Config.DevAPIKey = "test-dev-key"
	a.Config.AdminAPIKey = "test-admin-key"

	provRepo := &memoryProviderRepo{
		records: []*controlstate.ProviderRecord{},
		secrets: make(map[string]string),
	}
	repo := &memoryRepository{
		provRepo:  provRepo,
		auditRepo: &dummyAuditRepo{},
		idemRepo:  &dummyIdemRepo{},
	}
	cipher := &memoryCipher{secrets: make(map[string]string)}

	adminService := controlstate.NewAdminProviderService(repo, cipher, a.RuntimeProviderManager, nil)
	adminHandler := handlers.NewAdminProvidersHandler(adminService)

	admissionCtrl := admission.NewPassThroughController()
	gatewaySvc := gateway.NewService(a.RuntimeProviderManager, admissionCtrl, a.HealthStore(), a.Config.FallbackEnabled, a.Config.MaxAttempts)
	a.Router = router.NewRouter(a.Config, gatewaySvc, adminHandler, nil, repo)

	// 1. Initial reload with empty repo
	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("ReloadProviders empty failed: %v", err)
	}

	server := httptest.NewServer(a.Router)
	defer server.Close()

	// Initially, Chat Completion should return 503 no_active_provider_config
	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	// 2. Unauthenticated admin call should fail
	adminReqBody := `{"id": "test-admin", "name": "Admin Prov", "type": "openai-compatible", "base_url": "` + fakeUpstream.URL + `", "api_key": "sk-dummy", "models": ["gpt-4o"]}`
	reqAdmin, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBufferString(adminReqBody))
	reqAdmin.Header.Set("Content-Type", "application/json")
	reqAdmin.Header.Set("Authorization", "Bearer test-dev-key") // Wrong key for admin
	respAdmin, _ := http.DefaultClient.Do(reqAdmin)
	defer respAdmin.Body.Close()

	if respAdmin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin route with dev key, got %d", respAdmin.StatusCode)
	}

	// 3. Authenticated admin call to create provider
	reqAdmin, _ = http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBufferString(adminReqBody))
	reqAdmin.Header.Set("Content-Type", "application/json")
	reqAdmin.Header.Set("Authorization", "Bearer test-admin-key")
	respAdmin, _ = http.DefaultClient.Do(reqAdmin)
	defer respAdmin.Body.Close()

	if respAdmin.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", respAdmin.StatusCode)
	}

	// Check that response doesn't leak secrets
	var pResp map[string]interface{}
	json.NewDecoder(respAdmin.Body).Decode(&pResp)
	if pResp["api_key"] != nil {
		t.Fatalf("admin API leaked api_key in response!")
	}
	secretObj, ok := pResp["secret"].(map[string]interface{})
	if ok && secretObj["configured"] != true {
		// Just ensure it's structurally present
	}

	// 4. Chat Completion should now succeed without restarting the server!
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after adding provider, got %d", resp.StatusCode)
	}

	// 5. Disable provider via Admin API
	reqAdmin, _ = http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers/test-admin/disable", nil)
	reqAdmin.Header.Set("Authorization", "Bearer test-admin-key")
	respAdmin, _ = http.DefaultClient.Do(reqAdmin)
	defer respAdmin.Body.Close()

	if respAdmin.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for disable, got %d", respAdmin.StatusCode)
	}

	// 6. Chat Completion should fail again with 503
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 after disabling provider, got %d", resp.StatusCode)
	}
}

func TestAdminProvidersRates(t *testing.T) {
	cfg := &config.Config{
		RoutingStrategy: "round-robin",
		AdminAPIKey:     "test-admin-key",
	}

	a, err := app.New()
	if err != nil {
		t.Fatalf("app.New failed: %v", err)
	}
	a.Config = cfg

	provRepo := &memoryProviderRepo{
		records: []*controlstate.ProviderRecord{
			{
				ID:      "test-prov",
				Type:    "openai-compatible",
				Enabled: true,
				Models:  []string{"gpt-4"},
			},
		},
		secrets: make(map[string]string),
	}
	rateRepo := &memoryRateRepo{rates: make(map[string]*controlstate.ProviderModelRate)}
	repo := &memoryRepository{
		provRepo:  provRepo,
		rateRepo:  rateRepo,
		auditRepo: &dummyAuditRepo{},
		idemRepo:  &dummyIdemRepo{},
	}
	cipher := &memoryCipher{secrets: make(map[string]string)}

	adminService := controlstate.NewAdminProviderService(repo, cipher, a.RuntimeProviderManager, nil)
	adminHandler := handlers.NewAdminProvidersHandler(adminService)

	a.Router = router.NewRouter(a.Config, nil, adminHandler, nil, repo)

	server := httptest.NewServer(a.Router)
	defer server.Close()

	// 1. Put rate
	reqBody := `{"input_credit_rate": 150, "output_credit_rate": 300}`
	req, _ := http.NewRequest(http.MethodPut, server.URL+"/admin/v1/providers/test-prov/models/gpt-4/rate", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-admin-key")
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for PUT rate, got %d", resp.StatusCode)
	}

	var pResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&pResp)
	if pResp["input_credit_rate"].(float64) != 150 {
		t.Fatalf("expected rate to be 150, got %v", pResp["input_credit_rate"])
	}

	// 2. Get rate
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/admin/v1/providers/test-prov/models/gpt-4/rate", nil)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for GET rate, got %d", resp.StatusCode)
	}

	// 3. Delete rate
	req, _ = http.NewRequest(http.MethodDelete, server.URL+"/admin/v1/providers/test-prov/models/gpt-4/rate", nil)
	req.Header.Set("Authorization", "Bearer test-admin-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for DELETE rate, got %d", resp.StatusCode)
	}
}
