package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"
	"veloxmesh/internal/controlstate"
)

func TestOpenConfiguresSQLitePragmas(t *testing.T) {
	dsn := "file:pragma-test?mode=memory&cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	checkPragma(t, repo.db, "foreign_keys", "1")
	checkPragma(t, repo.db, "busy_timeout", "5000")
	checkPragma(t, repo.db, "synchronous", "1")
}

func checkPragma(t *testing.T, db *sql.DB, name, expected string) {
	t.Helper()
	var got string
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("Failed to read pragma %s: %v", name, err)
	}
	if got != expected {
		t.Fatalf("Expected pragma %s=%s, got %s", name, expected, got)
	}
}

func TestSQLiteRepository(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 1. Run migrations
	migrator := NewMigrator(repo.db)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// 2. Providers should be empty initially
	provs, err := repo.Providers().List(ctx, controlstate.ProviderFilter{})
	if err != nil {
		t.Fatalf("List providers failed: %v", err)
	}
	if len(provs) != 0 {
		t.Errorf("Expected 0 providers initially, got %d", len(provs))
	}

	// 3. Create a provider
	m := &controlstate.ProviderMutation{
		ID:      "test-1",
		Name:    "Test Provider",
		Type:    "openai-compatible",
		BaseURL: "https://api.test/v1",
		Enabled: true,
	}

	rec, err := repo.Providers().Create(ctx, m)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if rec.ID != m.ID {
		t.Errorf("Expected ID %s, got %s", m.ID, rec.ID)
	}
	if rec.Revision != 1 {
		t.Errorf("Expected revision 1, got %d", rec.Revision)
	}

	// 4. Update a provider
	rev := int64(1)
	m.Revision = &rev
	m.Name = "Test Provider Updated"
	rec, err = repo.Providers().Update(ctx, m)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if rec.Name != "Test Provider Updated" {
		t.Errorf("Expected updated name")
	}
	if rec.Revision != 2 {
		t.Errorf("Expected revision 2, got %d", rec.Revision)
	}

	// 5. Stale update should fail
	m.Name = "Stale Update"
	_, err = repo.Providers().Update(ctx, m)
	if err == nil {
		t.Errorf("Expected stale update to fail")
	}

	// 6. Routing - Get not found
	_, err = repo.Routing().Get(ctx)
	if err != controlstate.ErrRoutingConfigNotFound {
		t.Fatalf("Expected ErrRoutingConfigNotFound, got %v", err)
	}

	// 7. Routing - Save new config
	rCfg := &controlstate.RoutingConfig{
		Strategy:        "priority",
		DefaultProvider: "test-1",
		FallbackEnabled: true,
		MaxAttempts:     3,
	}
	if err := repo.Routing().Save(ctx, rCfg); err != nil {
		t.Fatalf("Failed to save routing config: %v", err)
	}

	// 8. Routing - Get saved config
	savedRCfg, err := repo.Routing().Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get routing config: %v", err)
	}
	if savedRCfg.Strategy != "priority" {
		t.Errorf("Expected strategy 'priority', got '%s'", savedRCfg.Strategy)
	}
	if savedRCfg.MaxAttempts != 3 {
		t.Errorf("Expected 3 max attempts, got %d", savedRCfg.MaxAttempts)
	}

	// 9. Routing - Upsert existing config
	savedRCfg.MaxAttempts = 5
	if err := repo.Routing().Save(ctx, savedRCfg); err != nil {
		t.Fatalf("Failed to upsert routing config: %v", err)
	}
	updatedRCfg, err := repo.Routing().Get(ctx)
	if err != nil {
		t.Fatalf("Failed to get updated routing config: %v", err)
	}
	if updatedRCfg.MaxAttempts != 5 {
		t.Errorf("Expected 5 max attempts, got %d", updatedRCfg.MaxAttempts)
	}
	if updatedRCfg.Revision <= savedRCfg.Revision {
		t.Errorf("Expected revision to increment, got %d vs %d", updatedRCfg.Revision, savedRCfg.Revision)
	}
}

func TestSQLiteAPIKeyCredit(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	migrator := NewMigrator(repo.db)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	key := &controlstate.APIKeyRecord{
		ID:            "key-1",
		Prefix:        "vx-",
		Hash:          "hash123",
		Name:          "Test Key",
		Role:          "admin",
		Enabled:       true,
		CreditBalance: 1000,
	}

	err = repo.APIKeys().Create(ctx, key)
	if err != nil {
		t.Fatalf("Create API key failed: %v", err)
	}

	// Get by hash
	fetched, err := repo.APIKeys().GetByHash(ctx, "hash123")
	if err != nil {
		t.Fatalf("GetByHash failed: %v", err)
	}
	if fetched == nil {
		t.Fatalf("Expected key to be found")
	}
	if fetched.CreditBalance != 1000 {
		t.Errorf("Expected CreditBalance 1000, got %d", fetched.CreditBalance)
	}

	// Update balance
	fetched.CreditBalance = 500
	err = repo.APIKeys().Update(ctx, fetched)
	if err != nil {
		t.Fatalf("Update API key failed: %v", err)
	}

	// List keys
	keys, err := repo.APIKeys().List(ctx)
	if err != nil {
		t.Fatalf("List API keys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}
	if keys[0].CreditBalance != 500 {
		t.Errorf("Expected updated CreditBalance 500, got %d", keys[0].CreditBalance)
	}

	// Delete key
	err = repo.APIKeys().Delete(ctx, "key-1")
	if err != nil {
		t.Fatalf("Delete API key failed: %v", err)
	}
	keys, _ = repo.APIKeys().List(ctx)
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys after delete, got %d", len(keys))
	}
}

func TestSQLiteRateAndUsage(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	migrator := NewMigrator(repo.db)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	_, err = repo.Providers().Create(ctx, &controlstate.ProviderMutation{
		ID: "p-1", Name: "P", Type: "openai", BaseURL: "http", Enabled: true,
	})
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	rate := &controlstate.ProviderModelRate{
		ProviderID:       "p-1",
		Model:            "m-1",
		InputCreditRate:  10,
		OutputCreditRate: 20,
	}

	if err := repo.Rates().Save(ctx, rate); err != nil {
		t.Fatalf("Failed to save rate: %v", err)
	}

	gotRate, err := repo.Rates().Get(ctx, "p-1", "m-1")
	if err != nil {
		t.Fatalf("Failed to get rate: %v", err)
	}
	if gotRate == nil || gotRate.InputCreditRate != 10 {
		t.Fatalf("Expected input rate 10, got %+v", gotRate)
	}

	if err := repo.Rates().Delete(ctx, "p-1", "m-1"); err != nil {
		t.Fatalf("Failed to delete rate: %v", err)
	}
	gotRate, _ = repo.Rates().Get(ctx, "p-1", "m-1")
	if gotRate != nil {
		t.Fatalf("Expected rate to be deleted")
	}

	usage := &controlstate.UsageRecord{
		ID:             "u-1",
		ProviderID:     "p-1",
		Model:          "m-1",
		PromptTokens:   100,
		ResponseTokens: 50,
		TotalTokens:    150,
		DurationMs:     200,
		Status:         controlstate.SettlementStatusUnsettled,
	}

	if err := repo.Usage().Log(ctx, usage); err != nil {
		t.Fatalf("Failed to log usage: %v", err)
	}
}

func TestSQLiteSemanticCache(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	migrator := NewMigrator(repo.db)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	cacheRepo := repo.SemanticCache()

	entry := &controlstate.SemanticCacheEntry{
		ID:        "sc-1",
		Scope:     "hash123",
		Model:     "gpt-4",
		Vector:    []byte{0x01, 0x02, 0x03},
		Response:  `{"choices": []}`,
		Enabled:   true,
		HitCount:  0,
		ExpiresAt: time.Now().Add(1 * time.Hour).UTC(),
	}

	if err := cacheRepo.Store(ctx, entry); err != nil {
		t.Fatalf("Failed to store cache entry: %v", err)
	}

	candidates, err := cacheRepo.ListCandidates(ctx, "hash123", "gpt-4")
	if err != nil {
		t.Fatalf("Failed to list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("Expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ID != "sc-1" {
		t.Errorf("Expected ID sc-1, got %s", candidates[0].ID)
	}
	if len(candidates[0].Vector) != 3 || candidates[0].Vector[0] != 0x01 {
		t.Errorf("Unexpected vector data")
	}

	if err := cacheRepo.RecordHit(ctx, "sc-1"); err != nil {
		t.Fatalf("Failed to record hit: %v", err)
	}

	if err := cacheRepo.Disable(ctx, "sc-1"); err != nil {
		t.Fatalf("Failed to disable entry: %v", err)
	}

	candidates, err = cacheRepo.ListCandidates(ctx, "hash123", "gpt-4")
	if err != nil {
		t.Fatalf("Failed to list candidates after disable: %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("Expected 0 candidates after disable, got %d", len(candidates))
	}
}

func TestSQLiteFallbackLog(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	migrator := NewMigrator(repo.db)
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	record := &controlstate.FallbackLogRecord{
		ID:      "log-1",
		Payload: `{"key": "value"}`,
		Status:  "pending",
	}

	if err := repo.FallbackLog().Insert(ctx, record); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	pending, err := repo.FallbackLog().ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending record, got %d", len(pending))
	}
	if pending[0].ID != "log-1" {
		t.Errorf("Expected ID 'log-1', got %s", pending[0].ID)
	}

	if err := repo.FallbackLog().UpdateStatus(ctx, "log-1", "processed"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	pending, _ = repo.FallbackLog().ListPending(ctx, 10)
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending records after update, got %d", len(pending))
	}
}
