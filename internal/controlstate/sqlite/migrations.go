package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"veloxmesh/internal/controlstate"
)

type Migrator struct {
	db *sql.DB
}

func NewMigrator(db *sql.DB) controlstate.Migrator {
	return &Migrator{db: db}
}

func (m *Migrator) Migrate(ctx context.Context) error {
	fs := controlstate.GetSQLiteMigrations()

	files := []string{"migrations/sqlite/0001_control_state.sql", "migrations/sqlite/0002_combos.sql"}

	// A real migrator would use a library like goose, but for this milestone we
	// can do a simple split or just execute the whole file. Wait, we should only
	// execute up to `-- +goose Down`.
	// For simplicity in Phase 3, we execute the full up migration.

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Just a simple create tables script if schema_migrations doesn't exist
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT count(*) > 0 FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&exists)
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

			statements := strings.Split(upSQL, ";")
			for _, stmt := range statements {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" {
					continue
				}
				if _, err := tx.ExecContext(ctx, stmt); err != nil {
					return fmt.Errorf("failed to execute sqlite migration statement %s: %w", file, err)
				}
			}
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES (1, 0)"); err != nil {
			return err
		}
	}

	return tx.Commit()
}
