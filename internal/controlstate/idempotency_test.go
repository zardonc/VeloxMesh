package controlstate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	gwErr "veloxmesh/internal/errors"
)

type mockIdempotencyRepo struct {
	IdempotencyRepository
	records map[string]*IdempotencyRecord
}

func (m *mockIdempotencyRepo) Get(ctx context.Context, key string) (*IdempotencyRecord, error) {
	if rec, ok := m.records[key]; ok {
		return rec, nil
	}
	return nil, nil
}

func (m *mockIdempotencyRepo) Save(ctx context.Context, record *IdempotencyRecord) error {
	m.records[record.Key] = record
	return nil
}

func TestRequestFingerprint(t *testing.T) {
	body1 := []byte(`{"name":"p1","api_key":"sk-1234"}`)
	body2 := []byte(`{"name":"p1","api_key":"sk-5678"}`)
	body3 := []byte(`{"name":"p2","api_key":"sk-1234"}`)

	fp1 := RequestFingerprint("POST", "/admin/v1/providers", body1)
	fp2 := RequestFingerprint("POST", "/admin/v1/providers", body2)

	if fp1 != fp2 {
		t.Errorf("expected api_key redaction to yield same fingerprint")
	}

	fp3 := RequestFingerprint("POST", "/admin/v1/providers", body3)
	if fp1 == fp3 {
		t.Errorf("expected different fingerprint for different content")
	}
}

func TestIdempotencyKeyFromRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Idempotency-Key", "test-key")

	if IdempotencyKeyFromRequest(req) != "test-key" {
		t.Errorf("expected test-key")
	}
}

func TestWithIdempotency_Success_And_Replay(t *testing.T) {
	idemRepo := &mockIdempotencyRepo{records: make(map[string]*IdempotencyRecord)}
	repo := &mockRepo{
		provRepo: &mockProviderRepo{},
		idemRepo: idemRepo,
	}

	svc := &AdminProviderService{
		repo: repo,
	}

	body := []byte(`{"name":"test"}`)
	callCount := 0

	executor := func(ctx context.Context) (interface{}, error) {
		callCount++
		return map[string]string{"result": "ok"}, nil
	}

	// First call
	res1, err := svc.WithIdempotency(context.Background(), "key1", "provider.create", "POST", "/admin", body, executor)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call")
	}
	if res1 == nil {
		t.Errorf("expected result")
	}

	// Replay with same key and body
	res2, err := svc.WithIdempotency(context.Background(), "key1", "provider.create", "POST", "/admin", body, executor)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected callCount to remain 1, got %d", callCount)
	}
	idemRes, ok := res2.(*IdempotencyResult)
	if !ok {
		t.Fatalf("expected IdempotencyResult")
	}
	if idemRes.Status != http.StatusCreated {
		t.Errorf("expected 201 status")
	}

	// Replay with same key but different body (Conflict)
	bodyDiff := []byte(`{"name":"different"}`)
	_, err = svc.WithIdempotency(context.Background(), "key1", "provider.create", "POST", "/admin", bodyDiff, executor)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if gwE, ok := err.(*gwErr.GatewayError); !ok || gwE.Code != "idempotency_key_conflict" {
		t.Errorf("expected idempotency_key_conflict")
	}
}
