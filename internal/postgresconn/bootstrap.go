package postgresconn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	vectorBootstrapLockKey   int64 = 0x56454c4f584d4553
	bootstrapRollbackTimeout       = 3 * time.Second
)

// WithVectorBootstrap serializes database-wide vector and schema initialization.
// The callback must use the supplied transaction for all bootstrap statements.
func WithVectorBootstrap(ctx context.Context, pool *pgxpool.Pool, run func(pgx.Tx) error) (err error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL bootstrap: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), bootstrapRollbackTimeout)
		defer cancel()
		if rollbackErr := tx.Rollback(cleanupCtx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback PostgreSQL bootstrap: %w", rollbackErr))
		}
	}()
	// READ COMMITTED gives the next catalog query a fresh snapshot after this wait.
	if _, err := tx.Exec(ctx, "SELECT pg_catalog.pg_advisory_xact_lock($1)", vectorBootstrapLockKey); err != nil {
		return fmt.Errorf("lock PostgreSQL bootstrap: %w", err)
	}
	if err := ensureVectorExtension(ctx, tx); err != nil {
		return err
	}
	if err := run(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensureVectorExtension(ctx context.Context, tx pgx.Tx) error {
	var exists bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_extension WHERE extname = 'vector')").Scan(&exists)
	if err != nil {
		return fmt.Errorf("inspect vector extension: %w", err)
	}
	if !exists {
		if _, err := tx.Exec(ctx, "CREATE EXTENSION vector WITH SCHEMA public"); err != nil {
			return fmt.Errorf("create vector extension in public: %w", err)
		}
	}
	return validateVectorVisibility(ctx, tx)
}

func validateVectorVisibility(ctx context.Context, tx pgx.Tx) error {
	var namespace string
	var visible bool
	err := tx.QueryRow(ctx, `SELECT n.nspname, EXISTS (
		SELECT 1 FROM pg_catalog.pg_type t
		WHERE t.typnamespace = e.extnamespace AND t.typname = 'vector'
		AND pg_catalog.pg_type_is_visible(t.oid))
		FROM pg_catalog.pg_extension e
		JOIN pg_catalog.pg_namespace n ON n.oid = e.extnamespace
		WHERE e.extname = 'vector'`).Scan(&namespace, &visible)
	if err != nil {
		return fmt.Errorf("inspect vector type visibility: %w", err)
	}
	if !visible {
		return fmt.Errorf("vector extension in schema %q is not visible or is shadowed: include its schema in the PostgreSQL DSN search_path after the application schema, grant schema USAGE if needed, and remove any conflicting vector type", namespace)
	}
	return nil
}
