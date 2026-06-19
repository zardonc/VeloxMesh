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
)

// A dummy control state repo using memory for tests
type memoryRepository struct {
	controlstate.Repository
	provRepo *memoryProviderRepo
}

func (m *memoryRepository) Providers() controlstate.ProviderRepository {
	return m.provRepo
}

type memoryProviderRepo struct {
	controlstate.ProviderRepository
	records []*controlstate.ProviderRecord
	secrets map[string]string
}

func (m *memoryProviderRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	var res []*controlstate.ProviderRecord
	for _, rec := range m.records {
		if filter.Enabled != nil && *filter.Enabled != rec.Enabled {
			continue
		}
		res = append(res, rec)
	}
	return res, nil
}

func (m *memoryProviderRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return []byte("enc"), []byte("nonce"), "key", nil
}

type memoryCipher struct {
	controlstate.SecretCipher
	secrets map[string]string
}

func (m *memoryCipher) DecryptProviderSecret(secret *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("test-secret"), nil
}

func TestDurableRuntimeIntegration(t *testing.T) {
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
						"content": "Hello from durable",
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

	admissionCtrl := admission.NewPassThroughController()
	gatewaySvc := gateway.NewService(a.RuntimeProviderManager, admissionCtrl, a.HealthStore(), a.Config.FallbackEnabled, a.Config.MaxAttempts)
	a.Router = router.NewRouter(a.Config, gatewaySvc)

	provRepo := &memoryProviderRepo{
		records: []*controlstate.ProviderRecord{},
	}
	repo := &memoryRepository{provRepo: provRepo}
	cipher := &memoryCipher{}

	// 1. Initial reload with empty repo
	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("ReloadProviders empty failed: %v", err)
	}

	server := httptest.NewServer(a.Router)
	defer server.Close()

	// Chat Completion should return 503 no_active_provider_config
	reqBody := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")

	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}

	var errResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&errResp)
	if code, _ := errResp["code"].(string); code != "no_active_provider_config" {
		t.Errorf("expected no_active_provider_config, got %v", errResp)
	}

	// 2. Add an active provider
	provRepo.records = append(provRepo.records, &controlstate.ProviderRecord{
		ID:      "durable-openai",
		Type:    "openai-compatible",
		Enabled: true,
		BaseURL: fakeUpstream.URL,
		Models:  []string{"gpt-4o"},
		Secret:  controlstate.ProviderSecretMetadata{SecretConfigured: true},
	})

	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("ReloadProviders failed: %v", err)
	}

	// Models should now have gpt-4o
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	var modelsResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&modelsResp)
	
	data, ok := modelsResp["data"].([]interface{})
	if !ok || len(data) != 1 {
		t.Errorf("expected models data to contain 1 element, got %v", modelsResp)
	} else if item, ok := data[0].(map[string]interface{}); !ok || item["id"] != "gpt-4o" {
		t.Errorf("expected model id to be gpt-4o, got %v", data[0])
	}

	// Chat completion should succeed
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// 3. Disable provider
	provRepo.records[0].Enabled = false
	err = a.ReloadProviders(context.Background(), repo, cipher)
	if err != nil {
		t.Fatalf("ReloadProviders failed: %v", err)
	}

	// Chat Completion should fail again with 503
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-dev-key")
	resp, _ = http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}
