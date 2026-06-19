package sqlite

import (
	"context"
	"testing"
	"veloxmesh/internal/controlstate"
)

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
}
