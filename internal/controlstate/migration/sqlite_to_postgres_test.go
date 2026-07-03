package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate/postgres"
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

func TestMigrationToLivePostgres(t *testing.T) {
	postgresDSN := os.Getenv("POSTGRES_TEST_DSN")
	if postgresDSN == "" {
		t.Skip("Skipping live postgres migration test because POSTGRES_TEST_DSN is not set")
	}
	postgresDSN = isolatedMigrationPostgresDSN(t, postgresDSN, "migration_live_success_test")
	sqliteDSN := filepath.Join(t.TempDir(), "source.db")
	source := migratedSQLiteFile(t, sqliteDSN)
	insertFixture(t, source)
	source.Close()

	target, err := postgres.Open(context.Background(), postgresDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := target.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
	target.Close()

	report, err := Migrate(context.Background(), Options{
		SQLiteDSN:   sqliteDSN,
		PostgresDSN: postgresDSN,
	})
	if err != nil {
		t.Fatalf("live postgres migration: %v report=%+v", err, report)
	}
	if !contains(report.CompletedTables, "session_blacklist") {
		t.Fatalf("expected all migration tables completed, got %+v", report.CompletedTables)
	}
}

func TestMigrationLivePostgresStopsWithReport(t *testing.T) {
	postgresDSN := os.Getenv("POSTGRES_TEST_DSN")
	if postgresDSN == "" {
		t.Skip("Skipping live postgres migration failure test because POSTGRES_TEST_DSN is not set")
	}
	postgresDSN = isolatedMigrationPostgresDSN(t, postgresDSN, "migration_live_failure_test")
	sqliteDSN := filepath.Join(t.TempDir(), "bad-source.db")
	source := migratedSQLiteFile(t, sqliteDSN)
	insertFixture(t, source)
	insertBadProviderSecret(t, source)
	source.Close()
	prepareLivePostgresTarget(t, postgresDSN)

	report, err := Migrate(context.Background(), Options{
		SQLiteDSN:   sqliteDSN,
		PostgresDSN: postgresDSN,
	})
	if err == nil {
		t.Fatalf("expected live postgres migration failure")
	}
	if report.FailedTable != "provider_secrets" || report.FailedRecord != "provider_id=missing-provider" {
		t.Fatalf("unexpected live failure report: %+v", report)
	}
	if strings.Contains(report.RootError, "secret-value") {
		t.Fatalf("report leaked secret material")
	}
}

func prepareLivePostgresTarget(t *testing.T, postgresDSN string) {
	t.Helper()
	target, err := postgres.Open(context.Background(), postgresDSN)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer target.Close()
	if err := target.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}
}

func isolatedMigrationPostgresDSN(t *testing.T, dsn, schema string) string {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres for schema setup: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+quotePostgresIdent(schema)); err != nil {
		t.Fatalf("create postgres test schema: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	return parsed.String()
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

func migratedSQLiteFile(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewMigrator(db).Migrate(context.Background()); err != nil {
		t.Fatalf("migrate sqlite file: %v", err)
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

func insertBadProviderSecret(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO provider_secrets (provider_id, ciphertext, nonce, key_id)
		VALUES ('missing-provider', x'0909', x'0808', 'bad-key');
	`)
	if err != nil {
		t.Fatalf("insert bad provider secret: %v", err)
	}
}

func tableName(query string) string {
	fields := strings.Fields(query)
	if len(fields) >= 3 {
		return strings.Trim(fields[2], `"`)
	}
	return ""
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func toKey(args ...interface{}) string {
	return fmt.Sprint(args...)
}
