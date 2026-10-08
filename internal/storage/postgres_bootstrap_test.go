//go:build phase29preflight

package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestPostgresBootstrapConcurrentExtension(t *testing.T) {
	for _, kind := range []string{"migration", "adapter"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newBootstrapFixture(t)
			dsn := fixture.schemaDSN(t, "waiter,public")
			blocker := fixture.holdExtension(t)
			pending := startBootstrapCall(t, fixture.ctx, bootstrapCall{kind: kind, dsn: dsn})
			// Release A before cleanup waits for B or closes either pool.
			t.Cleanup(func() { rollbackBootstrap(t, blocker) })
			fixture.waitForBlocked(t, pending)
			if err := blocker.Commit(fixture.ctx); err != nil {
				t.Fatal(err)
			}
			if err := pending.result(t); err != nil {
				t.Fatalf("concurrent %s initialization failed: %v", kind, err)
			}
			fixture.assertExtensionNamespace(t, "public")
			fixture.assertPrivateTable(t, "waiter")
		})
	}
}

func TestPostgresBootstrapRepeatedInitialization(t *testing.T) {
	const attempts = 3
	for attempt := range attempts {
		t.Run(fmt.Sprint(attempt), func(t *testing.T) {
			fixture := newBootstrapFixture(t)
			first := fixture.schemaDSN(t, "first_app,public")
			second := fixture.schemaDSN(t, "second_app,public")
			gate := make(chan struct{})
			calls := []bootstrapCall{
				{kind: "migration", dsn: first, start: gate},
				{kind: "migration", dsn: first, start: gate},
				{kind: "adapter", dsn: second, start: gate},
			}
			pending := make([]bootstrapPending, 0, len(calls))
			for _, call := range calls {
				pending = append(pending, startBootstrapCall(t, fixture.ctx, call))
			}
			close(gate)
			for index, result := range pending {
				if err := result.result(t); err != nil {
					t.Errorf("concurrent caller %d: %v", index, err)
				}
			}
			fixture.assertExtensionNamespace(t, "public")
			fixture.assertPrivateTable(t, "first_app")
			fixture.assertPrivateTable(t, "second_app")
		})
	}
}

func TestPostgresBootstrapCancellation(t *testing.T) {
	for _, kind := range []string{"migration", "adapter"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newBootstrapFixture(t)
			dsn := fixture.schemaDSN(t, "waiter,public")
			blocker := fixture.holdExtension(t)
			pending := startBootstrapCall(t, fixture.ctx, bootstrapCall{kind: kind, dsn: dsn})
			t.Cleanup(func() { rollbackBootstrap(t, blocker) })
			fixture.waitForBlocked(t, pending)
			pending.cancel()
			if err := pending.result(t); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled initialization must preserve context error: %v", err)
			}
			rollbackBootstrap(t, blocker)
			fixture.assertNoWaiterTransaction(t)
			if err := (bootstrapCall{kind: kind, dsn: dsn}).run(fixture.ctx); err != nil {
				t.Fatalf("initialization after cancellation: %v", err)
			}
		})
	}
}

func TestPostgresBootstrapRollback(t *testing.T) {
	fixture := newBootstrapFixture(t)
	dsn := fixture.schemaDSN(t, "broken_app,public")
	fixture.exec(t, "CREATE TABLE broken_app.provider_configs (id text)")
	err := (bootstrapCall{kind: "migration", dsn: dsn}).run(fixture.ctx)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "42P07" {
		t.Fatalf("expected original duplicate-table SQL error, got %v", err)
	}
	var clean bool
	err = fixture.pool.QueryRow(fixture.ctx, `SELECT
		NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')
		AND to_regclass('broken_app.schema_migrations') IS NULL`).Scan(&clean)
	if err != nil || !clean {
		t.Fatalf("failed initialization retained partial extension/schema state: clean=%v err=%v", clean, err)
	}
	fixture.assertLockAvailable(t)
	recovery := fixture.schemaDSN(t, "recovery_app,public")
	if err := (bootstrapCall{kind: "migration", dsn: recovery}).run(fixture.ctx); err != nil {
		t.Fatalf("initialization after SQL rollback: %v", err)
	}
}

func TestPostgresBootstrapExtensionNamespace(t *testing.T) {
	for _, visible := range []bool{true, false} {
		t.Run(fmt.Sprint(visible), func(t *testing.T) {
			fixture := newBootstrapFixture(t)
			fixture.exec(t, "CREATE SCHEMA extensions; CREATE EXTENSION vector WITH SCHEMA extensions")
			searchPath := "namespace_app,public"
			if visible {
				searchPath = "namespace_app,extensions,public"
			}
			dsn := fixture.schemaDSN(t, searchPath)
			err := (bootstrapCall{kind: "adapter", dsn: dsn}).run(fixture.ctx)
			if visible && err != nil {
				t.Fatalf("visible existing extension must support pooled insert/search: %v", err)
			}
			if !visible && (err == nil || !strings.Contains(err.Error(), "extensions") || !strings.Contains(err.Error(), "search_path")) {
				t.Fatalf("invisible extension needs actionable namespace/search_path error: %v", err)
			}
			fixture.assertExtensionNamespace(t, "extensions")
		})
	}
}

func TestPostgresBootstrapReadinessMigration(t *testing.T) {
	fixture := newBootstrapFixture(t)
	dsn := fixture.schemaDSN(t, "readiness_app,public")
	if err := (bootstrapCall{kind: "migration", dsn: dsn}).run(fixture.ctx); err != nil {
		t.Fatal(err)
	}
	var indexReady, versionRecorded bool
	err := fixture.pool.QueryRow(fixture.ctx, `SELECT
		EXISTS (SELECT 1 FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'readiness_app' AND c.relname = 'idx_semantic_cache_readiness'
			AND i.indrelid = 'readiness_app.semantic_cache_entries'::regclass
			AND i.indisvalid AND i.indpred IS NOT NULL),
		EXISTS (SELECT 1 FROM readiness_app.schema_migrations WHERE version = 10 AND NOT dirty)`).Scan(&indexReady, &versionRecorded)
	if err != nil {
		t.Fatal(err)
	}
	if !indexReady || !versionRecorded {
		t.Fatalf("migration 0010 not applied: valid partial index=%v recorded version=%v", indexReady, versionRecorded)
	}
}
