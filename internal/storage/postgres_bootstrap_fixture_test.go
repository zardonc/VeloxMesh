//go:build phase29preflight

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate/postgres"
	"veloxmesh/internal/postgresconn"
	"veloxmesh/internal/testenv"
)

const (
	// The external transaction deliberately follows the production locking contract.
	// Both bootstrap callers must use this database-wide key before inspecting catalogs.
	bootstrapAdvisoryKey     int64 = 0x56454c4f584d4553
	bootstrapDeadline              = 10 * time.Second
	bootstrapCleanupDeadline       = 3 * time.Second
	bootstrapPollInterval          = 10 * time.Millisecond
	bootstrapDimension             = 1536
	bootstrapHNSWM                 = 16
	bootstrapHNSWEF                = 64
	bootstrapDatabasePrefix        = "vm_bootstrap_"
)

type bootstrapFixture struct {
	ctx  context.Context
	pool *pgxpool.Pool
	dsn  string
}

// These opt-in tests require CREATE DATABASE in addition to normal migration privileges.
func newBootstrapFixture(t *testing.T) bootstrapFixture {
	t.Helper()
	testenv.Load()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Fatal("POSTGRES_TEST_DSN is required for PostgreSQL bootstrap regression tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapDeadline)
	t.Cleanup(cancel)
	config, err := postgresconn.PoolConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := bootstrapDatabasePrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatalf("create isolated PostgreSQL database (CREATE DATABASE privilege required): %v", err)
	}
	t.Cleanup(func() { dropBootstrapDatabase(t, admin, name) })
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("bootstrap tests require a PostgreSQL URL DSN")
	}
	parsed.Path = "/" + name
	query := parsed.Query()
	query.Set("search_path", "public")
	query.Set("application_name", "bootstrap_observer")
	parsed.RawQuery = query.Encode()
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var extensionExists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')").Scan(&extensionExists); err != nil {
		t.Fatal(err)
	}
	if extensionExists {
		t.Fatal("new template0 database unexpectedly contains vector; fresh-database oracle is invalid")
	}
	return bootstrapFixture{ctx: ctx, pool: pool, dsn: parsed.String()}
}

func dropBootstrapDatabase(t *testing.T, admin *pgxpool.Pool, name string) {
	t.Helper()
	if !strings.HasPrefix(name, bootstrapDatabasePrefix) || len(name) != len(bootstrapDatabasePrefix)+32 {
		t.Errorf("refusing cleanup of unexpected database identifier %q", name)
		return
	}
	if _, err := uuid.Parse(strings.TrimPrefix(name, bootstrapDatabasePrefix)); err != nil {
		t.Errorf("refusing cleanup of invalid database identifier: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapCleanupDeadline)
	defer cancel()
	if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Errorf("drop owned PostgreSQL database %s after connection cleanup: %v", name, err)
	}
}

func (f bootstrapFixture) schemaDSN(t *testing.T, searchPath string) string {
	t.Helper()
	schema := strings.Split(searchPath, ",")[0]
	f.exec(t, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	parsed, err := url.Parse(f.dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", searchPath)
	query.Set("application_name", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (f bootstrapFixture) exec(t *testing.T, sql string) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql); err != nil {
		t.Fatal(err)
	}
}

func (f bootstrapFixture) holdExtension(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rollbackBootstrap(t, tx) })
	if _, err := tx.Exec(f.ctx, "SELECT pg_advisory_xact_lock($1)", bootstrapAdvisoryKey); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(f.ctx, "CREATE EXTENSION vector WITH SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	return tx
}

func rollbackBootstrap(t *testing.T, tx pgx.Tx) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapCleanupDeadline)
	defer cancel()
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Errorf("rollback bootstrap blocker: %v", err)
	}
}

type bootstrapCall struct {
	kind  string
	dsn   string
	start <-chan struct{}
}

func (call bootstrapCall) run(ctx context.Context) error {
	if call.start != nil {
		select {
		case <-call.start:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if call.kind == "migration" {
		repo, err := postgres.Open(ctx, call.dsn)
		if err != nil {
			return err
		}
		defer repo.Close()
		return repo.Migrate(ctx)
	}
	adapter, err := NewPGVectorAdapter(ctx, call.dsn, PGVectorOptions{
		Dimension: bootstrapDimension, HNSWM: bootstrapHNSWM, HNSWEFConstruction: bootstrapHNSWEF,
	})
	if err != nil {
		return err
	}
	defer adapter.pool.Close()
	if err := adapter.EnsureCollection(ctx, "bootstrap", bootstrapDimension); err != nil {
		return err
	}
	vector := pgVectorTestEmbedding(bootstrapDimension)
	if err := adapter.Insert(ctx, "bootstrap", [][]float32{vector}, []map[string]interface{}{{"id": "bootstrap-entry"}}); err != nil {
		return err
	}
	results, err := adapter.Search(ctx, "bootstrap", vector, 1)
	if err != nil {
		return err
	}
	if len(results) != 1 || results[0]["id"] != "bootstrap-entry" {
		return fmt.Errorf("unexpected vector round-trip result: %v", results)
	}
	return nil
}

type bootstrapPending struct {
	done   <-chan error
	cancel context.CancelFunc
}

func startBootstrapCall(t *testing.T, ctx context.Context, call bootstrapCall) bootstrapPending {
	t.Helper()
	callCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- call.run(callCtx)
		close(done)
	}()
	pending := bootstrapPending{done: done, cancel: cancel}
	t.Cleanup(func() {
		cancel()
		pending.result(t)
	})
	return pending
}

func (p bootstrapPending) result(t *testing.T) error {
	t.Helper()
	select {
	case err := <-p.done:
		return err
	case <-time.After(bootstrapCleanupDeadline):
		t.Fatal("bootstrap caller did not finish within bounded join")
		return context.DeadlineExceeded
	}
}

func (f bootstrapFixture) waitForBlocked(t *testing.T, pending bootstrapPending) {
	t.Helper()
	ticker := time.NewTicker(bootstrapPollInterval)
	defer ticker.Stop()
	for {
		var event string
		err := f.pool.QueryRow(f.ctx, `SELECT COALESCE((SELECT wait_event
			FROM pg_stat_activity WHERE datname = current_database()
			AND application_name = 'waiter' AND wait_event_type = 'Lock' LIMIT 1), '')`).Scan(&event)
		if err != nil {
			t.Fatal(err)
		}
		if event != "" {
			t.Logf("observed initializer waiting on PostgreSQL lock: %s", event)
			return
		}
		select {
		case err := <-pending.done:
			t.Fatalf("initializer finished before held transaction released: %v", err)
		case <-f.ctx.Done():
			t.Fatal("initializer never reached observable PostgreSQL lock wait")
		case <-ticker.C:
		}
	}
}

func (f bootstrapFixture) assertExtensionNamespace(t *testing.T, expected string) {
	t.Helper()
	var namespace string
	err := f.pool.QueryRow(f.ctx, `SELECT n.nspname FROM pg_extension e
		JOIN pg_namespace n ON n.oid = e.extnamespace WHERE e.extname = 'vector'`).Scan(&namespace)
	if err != nil || namespace != expected {
		t.Errorf("extension namespace: got %q want %q err=%v", namespace, expected, err)
	}
}

func (f bootstrapFixture) assertPrivateTable(t *testing.T, schema string) {
	t.Helper()
	var private, public bool
	err := f.pool.QueryRow(f.ctx, `SELECT to_regclass($1) IS NOT NULL,
		to_regclass('public.semantic_cache_vectors') IS NOT NULL`, schema+".semantic_cache_vectors").Scan(&private, &public)
	if err != nil || !private || public {
		t.Errorf("application table placement: schema=%s private=%v public=%v err=%v", schema, private, public, err)
	}
}

func (f bootstrapFixture) assertNoWaiterTransaction(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(f.ctx, bootstrapCleanupDeadline)
	defer cancel()
	ticker := time.NewTicker(bootstrapPollInterval)
	defer ticker.Stop()
	for {
		var active bool
		err := f.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND application_name = 'waiter' AND xact_start IS NOT NULL)`).Scan(&active)
		if err != nil {
			t.Fatal(err)
		}
		if !active {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("canceled initializer retained a PostgreSQL transaction")
		case <-ticker.C:
		}
	}
	f.assertLockAvailable(t)
}

func (f bootstrapFixture) assertLockAvailable(t *testing.T) {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackBootstrap(t, tx)
	var available bool
	if err := tx.QueryRow(f.ctx, "SELECT pg_try_advisory_xact_lock($1)", bootstrapAdvisoryKey).Scan(&available); err != nil || !available {
		t.Fatalf("bootstrap advisory lock leaked: available=%v err=%v", available, err)
	}
}
