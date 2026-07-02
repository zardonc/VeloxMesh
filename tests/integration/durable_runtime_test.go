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
	"veloxmesh/internal/pipeline"
)

// A dummy control state repo using memory for tests
type dummyAuditRepo struct{ controlstate.AuditRepository }

func (d *dummyAuditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error { return nil }

type dummyIdemRepo struct {
	controlstate.IdempotencyRepository
}

func (d *dummyIdemRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	return nil, nil
}
func (d *dummyIdemRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	return nil
}

type memoryRepository struct {
	controlstate.Repository
	provRepo  *memoryProviderRepo
	rateRepo  *memoryRateRepo
	idemRepo  controlstate.IdempotencyRepository
	auditRepo controlstate.AuditRepository
	usageRepo controlstate.UsageRepository
}



type memoryUsageRepo struct {
	records []*controlstate.UsageRecord
}

func (m *memoryUsageRepo) Log(ctx context.Context, record *controlstate.UsageRecord) error {
	m.records = append(m.records, record)
	return nil
}

type memoryRateRepo struct {
	controlstate.RateRepository
	rates map[string]*controlstate.ProviderModelRate
}

func (m *memoryRateRepo) Save(ctx context.Context, rate *controlstate.ProviderModelRate) error {
	if m.rates == nil {
		m.rates = make(map[string]*controlstate.ProviderModelRate)
	}
	key := rate.ProviderID + ":" + rate.Model
	m.rates[key] = rate
	return nil
}

func (m *memoryRateRepo) Get(ctx context.Context, providerID, model string) (*controlstate.ProviderModelRate, error) {
	if m.rates == nil {
		return nil, nil
	}
	key := providerID + ":" + model
	if rate, ok := m.rates[key]; ok {
		return rate, nil
	}
	return nil, nil
}

func (m *memoryRateRepo) Delete(ctx context.Context, providerID, model string) error {
	if m.rates != nil {
		key := providerID + ":" + model
		delete(m.rates, key)
	}
	return nil
}

func (m *memoryRepository) Providers() controlstate.ProviderRepository      { return m.provRepo }
func (m *memoryRepository) Rates() controlstate.RateRepository              { return m.rateRepo }
func (m *memoryRepository) Usage() controlstate.UsageRepository             { return m.usageRepo }
func (m *memoryRepository) Idempotency() controlstate.IdempotencyRepository { return m.idemRepo }
func (m *memoryRepository) Audit() controlstate.AuditRepository             { return m.auditRepo }
func (m *memoryRepository) Routing() controlstate.RoutingRepository         { return &dummyRoutingRepo{} }
func (m *memoryRepository) APIKeys() controlstate.APIKeyRepository          { return nil }
func (m *memoryRepository) Combos() controlstate.ComboRepository            { return nil }
func (m *memoryRepository) SemanticRules() controlstate.SemanticRuleStore   { return nil }

type dummyRoutingRepo struct{}

func (d *dummyRoutingRepo) Get(ctx context.Context) (*controlstate.RoutingConfig, error) {
	return nil, controlstate.ErrRoutingConfigNotFound
}

func (d *dummyRoutingRepo) Save(ctx context.Context, config *controlstate.RoutingConfig) error {
	return nil
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

func (m *memoryProviderRepo) Create(ctx context.Context, p *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	rec := &controlstate.ProviderRecord{
		ID:      p.ID,
		Name:    p.Name,
		Type:    p.Type,
		BaseURL: p.BaseURL,
		Enabled: p.Enabled,
		Models:  p.Models,
		Secret:  controlstate.ProviderSecretMetadata{SecretConfigured: true},
	}
	m.records = append(m.records, rec)
	return rec, nil
}

func (m *memoryProviderRepo) Update(ctx context.Context, p *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	for _, r := range m.records {
		if r.ID == p.ID {
			r.Name = p.Name
			r.Enabled = p.Enabled
			r.Models = p.Models
			return r, nil
		}
	}
	return nil, nil
}

func (m *memoryProviderRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	for _, r := range m.records {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (m *memoryProviderRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return []byte("enc"), []byte("nonce"), "key", nil
}

func (m *memoryProviderRepo) PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error {
	m.secrets[id] = string(ciphertext)
	return nil
}

func (m *memoryRepository) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	return &mockTx{}, nil
}

func (m *memoryRepository) Settle(ctx context.Context, usage *controlstate.UsageRecord) error {
	repo, ok := m.usageRepo.(*memoryUsageRepo)
	if ok {
		repo.records = append(repo.records, usage)
	}
	return nil
}

type mockTx struct{}

func (m *mockTx) Commit() error   { return nil }
func (m *mockTx) Rollback() error { return nil }

type memoryCipher struct {
	controlstate.SecretCipher
	secrets map[string]string
}

func (m *memoryCipher) DecryptProviderSecret(secret *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("test-secret"), nil
}

func (m *memoryCipher) EncryptProviderSecret(plaintext []byte) (*controlstate.EncryptedSecret, error) {
	return &controlstate.EncryptedSecret{Ciphertext: plaintext, Nonce: []byte("n"), KeyID: "k"}, nil
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

	provRepo := &memoryProviderRepo{
		records: []*controlstate.ProviderRecord{},
	}
	repo := &memoryRepository{
		provRepo:  provRepo,
		auditRepo: &dummyAuditRepo{},
		idemRepo:  &dummyIdemRepo{},
		usageRepo: &memoryUsageRepo{},
	}
	cipher := &memoryCipher{}

	admissionCtrl := admission.NewPassThroughController()
	gatewaySvc := gateway.NewService(a.RuntimeProviderManager, admissionCtrl, a.HealthStore(), a.Config.FallbackEnabled, a.Config.MaxAttempts, repo, nil, pipeline.DefaultRegistry(), nil, nil)
	a.Router = router.NewRouter(a.Config, gatewaySvc, nil, nil, nil, nil, repo, nil, nil)

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
