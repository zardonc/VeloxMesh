package sqlite_test

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
)

func TestSessionBlacklist_SQLite(t *testing.T) {
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

	bl := repo.SessionBlacklist()

	// 1. Not blacklisted initially
	blacklisted, err := bl.IsBlacklisted(ctx, "hash1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if blacklisted {
		t.Fatalf("expected false, got true")
	}

	// 2. Blacklist a session
	expiresAt := time.Now().Add(1 * time.Hour).UTC()
	err = bl.Blacklist(ctx, &controlstate.SessionBlacklistRecord{
		SessionHash: "hash1",
		Reason:      "logout",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 3. Check it is blacklisted
	blacklisted, err = bl.IsBlacklisted(ctx, "hash1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !blacklisted {
		t.Fatalf("expected true, got false")
	}

	// 4. Blacklist an already expired session
	pastExpiresAt := time.Now().Add(-1 * time.Hour).UTC()
	err = bl.Blacklist(ctx, &controlstate.SessionBlacklistRecord{
		SessionHash: "hash2",
		Reason:      "logout",
		ExpiresAt:   pastExpiresAt,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// 5. Check it is NOT blacklisted because it's expired
	blacklisted, err = bl.IsBlacklisted(ctx, "hash2")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if blacklisted {
		t.Fatalf("expected false, got true")
	}

	// 6. Purge expired
	purged, err := bl.PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if purged != 1 { // Should purge hash2
		t.Fatalf("expected 1 purged row, got %d", purged)
	}
}
