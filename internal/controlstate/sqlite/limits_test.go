package sqlite_test

import (
	"context"
	"testing"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
)

func TestLimitRule_SQLite(t *testing.T) {
	dsn := "file::memory:?cache=shared"
	repo, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	if err := repo.Migrate(ctx); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	limitRepo := repo.LimitRules()

	rule := &controlstate.LimitRule{
		ID:        "rule-1",
		Scope:     controlstate.ScopeAPIKey,
		TargetID:  "key-1",
		Dimension: controlstate.DimensionRPM,
		Window:    controlstate.Window1M,
		Limit:     100,
		Enabled:   true,
	}

	if err := limitRepo.Save(ctx, rule); err != nil {
		t.Fatalf("Failed to save limit rule: %v", err)
	}

	rules, err := limitRepo.ListByTarget(ctx, controlstate.ScopeAPIKey, "key-1")
	if err != nil {
		t.Fatalf("Failed to list limit rules: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}
	if rules[0].ID != "rule-1" || rules[0].Limit != 100 {
		t.Errorf("Unexpected rule data: %+v", rules[0])
	}

	// Test unsupported dimension
	badRule := &controlstate.LimitRule{
		ID:        "rule-bad",
		Scope:     controlstate.ScopeAPIKey,
		TargetID:  "key-1",
		Dimension: controlstate.DimensionProviderBalance,
		Window:    controlstate.Window1M,
		Limit:     100,
		Enabled:   true,
	}
	if err := limitRepo.Save(ctx, badRule); err == nil {
		t.Errorf("Expected saving unsupported dimension to fail")
	}

	// Test delete
	if err := limitRepo.Delete(ctx, "rule-1"); err != nil {
		t.Fatalf("Failed to delete limit rule: %v", err)
	}

	rules, _ = limitRepo.ListByTarget(ctx, controlstate.ScopeAPIKey, "key-1")
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules, got %d", len(rules))
	}
}

