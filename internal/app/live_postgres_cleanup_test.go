//go:build phase29preflight

package app

import (
	"context"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
)

func TestLivePostgresDiscardable(t *testing.T) {
	env := liveEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	repo, err := postgres.Open(ctx, liveVectorSchema(t, env["POSTGRES_TEST_DSN"]))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cache := repo.SemanticCache()
	cleanup, ok := cache.(controlstate.SemanticCacheCleanupRepository)
	if !ok {
		t.Fatal("actual PostgreSQL cleanup contract unavailable")
	}
	source := liveComponentEntry(t, env)
	now := time.Now().UTC()
	old := source
	old.ID, old.Enabled, old.CreatedAt = source.Scope+"-pending", false, now.Add(-time.Minute)
	expired := source
	expired.ID, expired.CreatedAt, expired.ExpiresAt = source.Scope+"-expired", now.Add(-time.Minute), now.Add(-time.Second)
	active := source
	active.ID, active.CreatedAt = source.Scope+"-active", now.Add(-time.Minute)
	for _, entry := range []controlstate.SemanticCacheEntry{old, expired, active} {
		if err := cache.Store(ctx, &entry); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := cleanup.ListDiscardable(ctx, controlstate.CacheCleanupQuery{PendingBefore: now.Add(-time.Second), ExpiredBefore: now, Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	liveAssertPostgresCleanup(t, liveCleanupCheck{ctx: ctx, repo: cache, cleanup: cleanup, entries: entries, source: source, active: active})
}

type liveCleanupCheck struct {
	ctx            context.Context
	repo           controlstate.SemanticCacheRepository
	cleanup        controlstate.SemanticCacheCleanupRepository
	entries        []controlstate.CacheGarbage
	source, active controlstate.SemanticCacheEntry
}

func liveAssertPostgresCleanup(t *testing.T, check liveCleanupCheck) {
	var removed int
	for _, entry := range check.entries {
		if entry.Scope != check.source.Scope {
			continue
		}
		if entry.ID == check.active.ID {
			t.Fatal("cleanup selected a still-readable row")
		}
		if err := check.cleanup.RemoveDiscardable(check.ctx, entry); err != nil {
			t.Fatal(err)
		}
		removed++
	}
	if removed != 2 {
		t.Fatalf("actual PostgreSQL cleanup removed %d test-owned rows, expected two", removed)
	}
	liveAssertActivePreserved(t, check)
	shipLogJSON(t, map[string]any{"type": "postgres_cleanup", "removed_pending_and_expired": 2, "active_preserved": true})
}

func liveAssertActivePreserved(t *testing.T, check liveCleanupCheck) {
	read, err := check.repo.GetCandidate(check.ctx, check.active.ID, check.active.Scope, check.active.Model)
	if err != nil || read == nil {
		t.Fatalf("cleanup damaged active cache row: %v", err)
	}
	garbage := controlstate.CacheGarbage{ID: read.ID, Scope: read.Scope, Model: read.Model, CreatedAt: read.CreatedAt, ExpiresAt: read.ExpiresAt}
	if err := check.cleanup.RemoveDiscardable(check.ctx, garbage); err != nil {
		t.Fatal(err)
	}
	read, err = check.repo.GetCandidate(check.ctx, check.active.ID, check.active.Scope, check.active.Model)
	if err != nil || read == nil {
		t.Fatal("conditional cleanup removed a row activated before deletion")
	}
}
