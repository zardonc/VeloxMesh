package postgres

import (
	"context"
	"fmt"
	"strings"

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
	files := []string{"migrations/postgres/0001_control_state.sql", "migrations/postgres/0002_combos.sql"}

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
		for _, file := range files {
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
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES (1, false)"); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
