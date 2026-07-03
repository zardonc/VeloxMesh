package postgres

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

type Migrator struct {
	pool *pgxpool.Pool
}

func NewMigrator(pool *pgxpool.Pool) controlstate.Migrator {
	return &Migrator{pool: pool}
}

func (m *Migrator) Migrate(ctx context.Context) error {
	fs := controlstate.GetPostgreSQLMigrations()
	initialFiles := []string{
		"migrations/postgres/0001_control_state.sql",
		"migrations/postgres/0002_combos.sql",
	}
	initialVersions := []int64{1, 2}
	versionedFiles := []versionedMigration{
		{version: 3, file: "migrations/postgres/0003_limits_sessions.sql"},
		{version: 4, file: "migrations/postgres/0004_pgvector_semantic_cache.sql"},
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'schema_migrations')").Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		for _, file := range initialFiles {
			if err := executeMigration(ctx, tx, fs, file); err != nil {
				return err
			}
		}
		for _, version := range initialVersions {
			if err := recordMigrationVersion(ctx, tx, version); err != nil {
				return err
			}
		}
	}

	for _, migration := range versionedFiles {
		if applied, err := migrationApplied(ctx, tx, migration.version); err != nil {
			return err
		} else if applied {
			continue
		}
		if err := executeMigration(ctx, tx, fs, migration.file); err != nil {
			return err
		}
		if err := recordMigrationVersion(ctx, tx, migration.version); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

type versionedMigration struct {
	version int64
	file    string
}

func recordMigrationVersion(ctx context.Context, tx pgx.Tx, version int64) error {
	_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES ($1, false)", version)
	return err
}

func migrationApplied(ctx context.Context, tx pgx.Tx, version int64) (bool, error) {
	var applied bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1 AND dirty = false)", version).Scan(&applied)
	return applied, err
}

func executeMigration(ctx context.Context, tx pgx.Tx, fs embed.FS, file string) error {
	data, err := fs.ReadFile(file)
	if err != nil {
		return err
	}
	sqlStr := string(data)
	upSQL := sqlStr
	importIdx := strings.Index(sqlStr, "-- +goose Down")
	if importIdx != -1 {
		upSQL = sqlStr[:importIdx]
	}
	if _, err := tx.Exec(ctx, upSQL); err != nil {
		return fmt.Errorf("failed to execute postgres migration %s: %w", file, err)
	}
	return nil
}
