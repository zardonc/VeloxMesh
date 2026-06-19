package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
)

// A simplified mock repo for handlers test
type mockAdminRepo struct {
	controlstate.Repository
	provRepo  *mockProvRepo
	idemRepo  controlstate.IdempotencyRepository
	auditRepo controlstate.AuditRepository
}

func (m *mockAdminRepo) Providers() controlstate.ProviderRepository      { return m.provRepo }
func (m *mockAdminRepo) Idempotency() controlstate.IdempotencyRepository { return m.idemRepo }
func (m *mockAdminRepo) Audit() controlstate.AuditRepository             { return m.auditRepo }
func (m *mockAdminRepo) BeginTx(ctx context.Context) (controlstate.Transaction, error) {
	return &mockTx{}, nil
}

type mockTx struct{}

func (m *mockTx) Commit() error   { return nil }
func (m *mockTx) Rollback() error { return nil }

type mockIdempotencyRepo struct {
	controlstate.IdempotencyRepository
	records map[string]*controlstate.IdempotencyRecord
}

func (m *mockIdempotencyRepo) Get(ctx context.Context, key string) (*controlstate.IdempotencyRecord, error) {
	if rec, ok := m.records[key]; ok {
		return rec, nil
	}
	return nil, nil
}

func (m *mockIdempotencyRepo) Save(ctx context.Context, record *controlstate.IdempotencyRecord) error {
	if m.records == nil {
		m.records = make(map[string]*controlstate.IdempotencyRecord)
	}
	m.records[record.Key] = record
	return nil
}

type mockAuditRepo struct {
	controlstate.AuditRepository
}

func (m *mockAuditRepo) Log(ctx context.Context, event *controlstate.AuditEvent) error {
	return nil
}

type mockProvRepo struct {
	controlstate.ProviderRepository
	records []*controlstate.ProviderRecord
}

func (m *mockProvRepo) Get(ctx context.Context, id string) (*controlstate.ProviderRecord, error) {
	for _, r := range m.records {
		if r.ID == id {
			return r, nil
		}
	}
	// Return empty record for tests where id isn't set up
	return &controlstate.ProviderRecord{ID: id, Enabled: true, Type: "openai-compatible"}, nil
}

func (m *mockProvRepo) Create(ctx context.Context, p *controlstate.ProviderMutation) (*controlstate.ProviderRecord, error) {
	rec := &controlstate.ProviderRecord{
		ID:      p.ID,
		Name:    p.Name,
		Type:    p.Type,
		BaseURL: p.BaseURL,
		Enabled: p.Enabled,
		Models:  p.Models,
	}
	m.records = append(m.records, rec)
	return rec, nil
}
func (m *mockProvRepo) PutEncryptedSecret(ctx context.Context, id string, ciphertext, nonce []byte, keyID string) error {
	return nil
}
func (m *mockProvRepo) GetEncryptedSecret(ctx context.Context, id string) ([]byte, []byte, string, error) {
	return []byte("enc"), []byte("n"), "k", nil
}
func (m *mockProvRepo) List(ctx context.Context, filter controlstate.ProviderFilter) ([]*controlstate.ProviderRecord, error) {
	return m.records, nil
}

type mockAdminCipher struct {
	controlstate.SecretCipher
}

func (m *mockAdminCipher) EncryptProviderSecret(plaintext []byte) (*controlstate.EncryptedSecret, error) {
	return &controlstate.EncryptedSecret{}, nil
}
func (m *mockAdminCipher) DecryptProviderSecret(s *controlstate.EncryptedSecret) ([]byte, error) {
	return []byte("key"), nil
}

func TestAdminProvidersHandler_Create(t *testing.T) {
	repo := &mockAdminRepo{provRepo: &mockProvRepo{}, idemRepo: &mockIdempotencyRepo{}, auditRepo: &mockAuditRepo{}}
	cipher := &mockAdminCipher{}
	manager := controlstate.NewRuntimeProviderManager(&config.Config{}, nil, nil)
	svc := controlstate.NewAdminProviderService(repo, cipher, manager)
	handler := NewAdminProvidersHandler(svc)

	r := chi.NewRouter()
	r.Post("/admin/v1/providers", handler.Create)

	reqBody := `{"id": "p1", "name": "p1", "type": "openai-compatible", "base_url": "http://a", "api_key": "sk", "models": ["gpt"]}`
	req := httptest.NewRequest("POST", "/admin/v1/providers", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminProvidersHandler_Create_ValidationFail(t *testing.T) {
	repo := &mockAdminRepo{provRepo: &mockProvRepo{}, idemRepo: &mockIdempotencyRepo{}, auditRepo: &mockAuditRepo{}}
	cipher := &mockAdminCipher{}
	manager := controlstate.NewRuntimeProviderManager(&config.Config{}, nil, nil)
	svc := controlstate.NewAdminProviderService(repo, cipher, manager)
	handler := NewAdminProvidersHandler(svc)

	r := chi.NewRouter()
	r.Post("/admin/v1/providers", handler.Create)

	// Missing api_key
	reqBody := `{"id": "p1", "name": "p1", "type": "openai-compatible", "base_url": "http://a", "models": ["gpt"]}`
	req := httptest.NewRequest("POST", "/admin/v1/providers", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["code"] != "validation_failed" {
		t.Errorf("expected validation_failed, got %v", resp["code"])
	}
}

func TestAdminProvidersHandler_TestConnection(t *testing.T) {
	repo := &mockAdminRepo{auditRepo: &mockAuditRepo{}, idemRepo: &mockIdempotencyRepo{}, provRepo: &mockProvRepo{
		records: []*controlstate.ProviderRecord{
			{
				ID:      "p1",
				Type:    "openai-compatible",
				BaseURL: "http://example.com",
				Enabled: true,
				Models:  []string{"gpt-4"},
			},
		},
	}}
	cipher := &mockAdminCipher{}
	manager := controlstate.NewRuntimeProviderManager(&config.Config{}, nil, nil)
	svc := controlstate.NewAdminProviderService(repo, cipher, manager)
	handler := NewAdminProvidersHandler(svc)

	r := chi.NewRouter()
	r.Post("/admin/v1/providers/{id}/test-connection", handler.TestConnection)

	req := httptest.NewRequest("POST", "/admin/v1/providers/p1/test-connection", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if _, ok := resp["ok"]; !ok {
		t.Errorf("expected ok field in response")
	}
}
