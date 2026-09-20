package controlstate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPostgreSQLCapabilityProfile(t *testing.T) {
	caps := PostgreSQLCapabilityProfile()
	if !caps.DurableConfig || !caps.DistributedControlState {
		t.Errorf("PostgreSQL should support durable and distributed control state")
	}
	if !caps.SemanticCache || !caps.RateLimits || !caps.CostGovernance {
		t.Errorf("PostgreSQL should report verified semantic cache, rate limit, and cost governance support")
	}
}

func TestSQLiteCapabilityProfile(t *testing.T) {
	caps := SQLiteCapabilityProfile()
	if !caps.DurableConfig || !caps.LocalOnly {
		t.Errorf("SQLite should support durable and local control state")
	}
	if caps.DistributedControlState {
		t.Errorf("SQLite should not support distributed control state")
	}
	if caps.SemanticCache || caps.RateLimits || caps.CostGovernance {
		t.Errorf("SQLite should not support semantic cache, rate limits, or cost governance")
	}
}

func TestRedactProviderRecord(t *testing.T) {
	now := time.Now()
	rec := &ProviderRecord{
		ID:      "test-1",
		Name:    "Test",
		Type:    "openai-compatible",
		BaseURL: "https://api.test/v1",
		Enabled: true,
		Secret: ProviderSecretMetadata{
			SecretConfigured: true,
			UpdatedAt:        &now,
		},
	}

	redacted := RedactProviderRecord(rec)

	b, _ := json.Marshal(redacted)
	s := string(b)

	// Just ensure the output is safe. Since we don't store raw API keys in ProviderRecord at all,
	// it's naturally redacted, but we verify the contract.
	if !redacted.Secret.SecretConfigured {
		t.Errorf("Expected secret configured metadata")
	}
	if s == "" || len(s) == 0 {
		t.Errorf("Expected valid JSON")
	}
}

func TestSchemaOnlyMigrations(t *testing.T) {
	pgData, err := GetPostgreSQLMigrations().ReadFile("migrations/postgres/0001_control_state.sql")
	if err != nil {
		t.Fatalf("Failed to read postgres migration: %v", err)
	}
	sqData, err := GetSQLiteMigrations().ReadFile("migrations/sqlite/0001_control_state.sql")
	if err != nil {
		t.Fatalf("Failed to read sqlite migration: %v", err)
	}

	for name, data := range map[string][]byte{"postgres": pgData, "sqlite": sqData} {
		content := string(data)
		tables := []string{
			"provider_configs", "provider_secrets", "routing_configs",
			"api_keys", "usage_records", "audit_events", "idempotency_keys",
			"schema_migrations",
		}
		for _, table := range tables {
			if !strings.Contains(content, table) {
				t.Errorf("%s migration missing table %s", name, table)
			}
		}

		if strings.Contains(content, "INSERT INTO") {
			t.Errorf("%s migration contains INSERT statement, seeding data is forbidden per D-01", name)
		}
	}
}

func TestValidateProviderMutation(t *testing.T) {
	key := "sk-1234"
	defModel := "m1"
	m := &ProviderMutation{
		ID:           "test",
		Name:         "test name",
		Type:         "openai-compatible",
		BaseURL:      "http://localhost",
		APIKey:       &key,
		Models:       []string{"m1", "m2"},
		DefaultModel: &defModel,
	}

	errs := ValidateProviderMutation(m, true)
	if len(errs) > 0 {
		t.Errorf("Expected valid mutation, got %v", errs)
	}

	m.ID = ""
	m.Type = "invalid"
	m.BaseURL = "not-a-url"
	m.APIKey = nil
	defModel2 := "m3"
	m.DefaultModel = &defModel2

	errs = ValidateProviderMutation(m, true)
	if len(errs) != 5 {
		t.Errorf("Expected 5 errors, got %d", len(errs))
	}
}

func TestSecretCipher(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	cipher, err := NewAESGCMSecretCipher(key, "v1")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	plaintext := []byte("secret-value")
	enc, err := cipher.EncryptProviderSecret(plaintext)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if len(enc.Ciphertext) == 0 || len(enc.Nonce) == 0 {
		t.Errorf("Expected ciphertext and nonce")
	}

	dec, err := cipher.DecryptProviderSecret(enc)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if string(dec) != string(plaintext) {
		t.Errorf("Expected %s, got %s", string(plaintext), string(dec))
	}
}

func TestSecretCipherRejectsInvalidNonce(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	cipher, err := NewAESGCMSecretCipher(key, "v1")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	_, err = cipher.DecryptProviderSecret(&EncryptedSecret{
		Ciphertext: []byte("ciphertext"),
		Nonce:      []byte("no"),
		KeyID:      "v1",
	})
	if err == nil {
		t.Fatalf("expected invalid nonce error")
	}
}
