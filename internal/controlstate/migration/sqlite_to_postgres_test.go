package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"veloxmesh/internal/controlstate/sqlite"
)

type fakeTarget struct {
	failTable string
	records   map[string]bool
}

func (f *fakeTarget) Exec(ctx context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	table := tableName(query)
	if table == f.failTable {
		return pgconn.CommandTag{}, errors.New("forced write failure")
	}
	if f.records == nil {
		f.records = map[string]bool{}
	}
	key := table + ":" + strings.TrimSpace(toKey(args...))
	f.records[key] = true
	return pgconn.CommandTag{}, nil
}

func TestMigrationIsIdempotent(t *testing.T) {
	source := migratedSQLite(t)
	insertFixture(t, source)
	target := &fakeTarget{}

	if _, err := Run(context.Background(), source, target); err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstCount := len(target.records)
	if _, err := Run(context.Background(), source, target); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(target.records) != firstCount {
		t.Fatalf("expected idempotent upserts, got %d then %d records", firstCount, len(target.records))
	}
}

func TestMigrationStopsWithReport(t *testing.T) {
	source := migratedSQLite(t)
	insertFixture(t, source)
	target := &fakeTarget{failTable: "api_keys"}

	report, err := Run(context.Background(), source, target)
	if err == nil {
		t.Fatalf("expected migration failure")
	}
	if report.FailedTable != "api_keys" || !strings.Contains(report.FailedRecord, "id=key-1") {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.CompletedTables) == 0 || report.CompletedTables[len(report.CompletedTables)-1] != "routing_configs" {
		t.Fatalf("expected stop after completed routing_configs, got %+v", report.CompletedTables)
	}
	if strings.Contains(report.RootError, "secret-value") {
		t.Fatalf("report leaked secret material")
	}
}

func migratedSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrator(db).Migrate(context.Background()); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func insertFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO provider_configs (id, name, type, base_url, enabled, models_json, revision)
		VALUES ('p1', 'Provider 1', 'openai-compatible', 'http://example.test', 1, '["gpt-4"]', 1);
		INSERT INTO provider_secrets (provider_id, ciphertext, nonce, key_id)
		VALUES ('p1', x'0102', x'0304', 'v1');
		INSERT INTO routing_configs (id, strategy, fallback_enabled, max_attempts, revision)
		VALUES ('default', 'least-latency', 1, 2, 1);
		INSERT INTO api_keys (id, prefix, hash, name, role, enabled, credit_balance)
		VALUES ('key-1', 'vx-', 'hash-1', 'Key 1', 'admin', 1, 100);
	`)
	if err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
}

func tableName(query string) string {
	fields := strings.Fields(query)
	if len(fields) >= 3 {
		return fields[2]
	}
	return ""
}

func toKey(args ...interface{}) string {
	return fmt.Sprint(args...)
}
