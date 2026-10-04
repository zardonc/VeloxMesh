//go:build phase29preflight

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/postgres"
)

type postgresReadinessCheck struct {
	entry    controlstate.SemanticCacheEntry
	expected bool
}

// Validate the same readiness lifecycle against real PostgreSQL, including
// activation, expiry, isolation and closed-repository failure.
func TestLivePostgresScopeReadiness(t *testing.T) {
	env := liveEnvironment(t)
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	repo, err := postgres.Open(ctx, env["POSTGRES_TEST_DSN"])
	if err != nil {
		t.Fatal("PostgreSQL connection failed")
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cache := repo.SemanticCache()
	readiness, ok := cache.(controlstate.SemanticCacheReadinessRepository)
	if !ok {
		t.Fatal("PostgreSQL readiness unavailable")
	}
	scope := fmt.Sprintf("readiness-%d", time.Now().UnixNano())
	entry := controlstate.SemanticCacheEntry{ID: scope, Scope: scope, Model: "test-model", Vector: []byte{}, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	active := entry
	active.Enabled = true
	expired := active
	expired.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	assertPostgresReadiness(t, readiness, postgresReadinessCheck{entry: entry})
	for _, check := range []postgresReadinessCheck{{entry: entry}, {entry: active, expected: true}, {entry: expired}} {
		if err := cache.Store(ctx, &check.entry); err != nil {
			t.Fatal(err)
		}
		assertPostgresReadiness(t, readiness, check)
	}
	foreign := active
	foreign.Model = "other-model"
	assertPostgresReadiness(t, readiness, postgresReadinessCheck{entry: foreign})
	repo.Close()
	if _, err := readiness.HasCandidates(ctx, scope, entry.Model); err == nil {
		t.Fatal("closed repository error swallowed")
	}
	t.Log("PostgreSQL absent/pending/active/foreign/expired/error readiness passed")
}

func assertPostgresReadiness(t *testing.T, repo controlstate.SemanticCacheReadinessRepository, check postgresReadinessCheck) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), liveHTTPTimeout)
	defer cancel()
	ready, err := repo.HasCandidates(ctx, check.entry.Scope, check.entry.Model)
	if err != nil || ready != check.expected {
		t.Fatalf("readiness expected=%t got=%t error=%v", check.expected, ready, err)
	}
}
