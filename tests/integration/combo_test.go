package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	"veloxmesh/internal/hotstate"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/observability"
)

func TestComboRoutingAndAdmin(t *testing.T) {
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
						"content": "Hello",
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
		DevAPIKey:       "test-dev-key",
		AdminAPIKey:     "test-admin-key",
	}

	logger := observability.SetupLogger("info")
	healthStore := health.NewInMemoryStore()
	hotStateClient := hotstate.NewLocalHotState()
	m := controlstate.NewRuntimeProviderManager(cfg, logger, healthStore)
	
	ctx := context.Background()
	sqliteRepo, err := sqlite.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	var repo controlstate.Repository = sqliteRepo
	// Migrate DB
	migrator, ok := repo.(interface{ Migrate(context.Context) error })
	if ok {
		migrator.Migrate(ctx)
	}

	cipher := &memoryCipher{secrets: make(map[string]string)}

	// Seed dev API key
	keyHash := sha256.Sum256([]byte("test-dev-key"))
	repo.APIKeys().Create(ctx, &controlstate.APIKeyRecord{
		ID:            "dev-key-1",
		Hash:          hex.EncodeToString(keyHash[:]),
		Name:          "Dev Key",
		Role:          "admin",
		Enabled:       true,
		CreditBalance: 1000000,
	})

	adminProvSvc := controlstate.NewAdminProviderService(repo, cipher, m, hotStateClient)
	adminProvHandler := handlers.NewAdminProvidersHandler(adminProvSvc)

	adminComboSvc := controlstate.NewAdminComboService(repo, m, cipher, hotStateClient)
	adminCombosHandler := handlers.NewAdminCombosHandler(adminComboSvc)

	admissionCtrl := admission.NewPassThroughController()
	gatewaySvc := gateway.NewService(m, admissionCtrl, healthStore, false, 0, repo, nil)
	r := router.NewRouter(cfg, gatewaySvc, adminProvHandler, adminCombosHandler, hotStateClient, repo)

	server := httptest.NewServer(r)
	defer server.Close()

	// Seed some providers using HTTP
	p1ReqBody := map[string]interface{}{
		"id": "p1-only", "name": "p1", "type": "openai-compatible", "base_url": fakeUpstream.URL, "api_key": "sk-1", "models": []string{"p1-only"}, "default_model": "p1-only", "enabled": true,
	}
	p1b, _ := json.Marshal(p1ReqBody)
	req1, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBuffer(p1b))
	req1.Header.Set("Authorization", "Bearer test-admin-key")
	resp1, _ := http.DefaultClient.Do(req1)
	if resp1.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp1.Body)
		t.Fatalf("failed to create p1: %d %s", resp1.StatusCode, string(b))
	}
	resp1.Body.Close()

	p2ReqBody := map[string]interface{}{
		"id": "gpt-4o", "name": "p2", "type": "openai-compatible", "base_url": fakeUpstream.URL, "api_key": "sk-2", "models": []string{"gpt-4o"}, "default_model": "gpt-4o", "enabled": true,
	}
	p2b, _ := json.Marshal(p2ReqBody)
	req2, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/providers", bytes.NewBuffer(p2b))
	req2.Header.Set("Authorization", "Bearer test-admin-key")
	resp2, _ := http.DefaultClient.Do(req2)
	if resp2.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("failed to create p2: %d %s", resp2.StatusCode, string(b))
	}
	resp2.Body.Close()

	// 1. Create a Combo via Admin API (Round Robin)
	t.Run("Create Round-Robin Combo", func(t *testing.T) {
		reqBody := controlstate.ComboCreateRequest{
			ID:       "test-combo-rr",
			Name:     "smart-rr",
			Strategy: "round-robin",
			Members:  []string{"gpt-4o", "p1-only"},
			Enabled:  func() *bool { b := true; return &b }(),
		}
		b, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/combos", bytes.NewBuffer(b))
		req.Header.Set("Authorization", "Bearer test-admin-key")
		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create combo: %d", resp.StatusCode)
		}
	})

	// 2. Test Round Robin Routing
	t.Run("Test Round Robin Combo Routing", func(t *testing.T) {
		reqBody := `{"model": "smart-rr", "messages": [{"role": "user", "content": "Hello"}]}`
		
		req1, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
		req1.Header.Set("Authorization", "Bearer test-dev-key")
		resp1, _ := http.DefaultClient.Do(req1)
		defer resp1.Body.Close()

		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("req1 failed: %d", resp1.StatusCode)
		}
	})
	
	// 3. Create Fusion Combo
	t.Run("Create Fusion Combo", func(t *testing.T) {
		judge := "gpt-4o"
		reqBody := controlstate.ComboCreateRequest{
			ID:       "test-combo-fusion",
			Name:     "smart-fusion",
			Strategy: "fusion",
			Members:  []string{"gpt-4o", "p1-only"},
			Judge:    &judge,
			Enabled:  func() *bool { b := true; return &b }(),
		}
		b, _ := json.Marshal(reqBody)

		req, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/combos", bytes.NewBuffer(b))
		req.Header.Set("Authorization", "Bearer test-admin-key")
		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create combo: %d", resp.StatusCode)
		}
	})

	t.Run("Test Fusion Combo Routing", func(t *testing.T) {
		reqBody := `{"model": "smart-fusion", "messages": [{"role": "user", "content": "Hello"}]}`
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Authorization", "Bearer test-dev-key")
		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("req failed: %d", resp.StatusCode)
		}
	})

	// 4. Create Capacity Auto Switch Combo
	t.Run("Create Capacity Combo", func(t *testing.T) {
		reqBody := controlstate.ComboCreateRequest{
			ID:       "test-combo-capacity",
			Name:     "smart-capacity",
			Strategy: "capacity-auto-switch",
			Members:  []string{"p1-only", "gpt-4o"},
			Enabled:  func() *bool { b := true; return &b }(),
		}
		b, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/v1/combos", bytes.NewBuffer(b))
		req.Header.Set("Authorization", "Bearer test-admin-key")
		resp, _ := http.DefaultClient.Do(req)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create combo: %d", resp.StatusCode)
		}
	})
}
