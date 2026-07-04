package sqlite

import (
	"context"
	"database/sql"
	"embed"
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

	files := []string{"migrations/sqlite/0001_control_state.sql", "migrations/sqlite/0002_combos.sql", "migrations/sqlite/0003_semantic_rules.sql", "migrations/sqlite/0004_limit_rules.sql", "migrations/sqlite/0005_session_blacklist.sql", "migrations/sqlite/0006_routing_composite.sql", "migrations/sqlite/0007_scheduler_training_samples.sql", "migrations/sqlite/0008_scheduler_quality_rollups.sql"}

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
			if importIdx == -1 {
				importIdx = strings.Index(sqlStr, "-- +migrate Down")
			}
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
	if err := ensureSQLiteSchedulerTrainingSamples(ctx, tx, fs); err != nil {
		return err
	}
	if err := ensureSQLiteSchedulerQualityRollups(ctx, tx, fs); err != nil {
		return err
	}

	return tx.Commit()
}

func ensureSQLiteSchedulerTrainingSamples(ctx context.Context, tx *sql.Tx, fs embed.FS) error {
	var exists bool
	err := tx.QueryRowContext(ctx, "SELECT count(*) > 0 FROM sqlite_master WHERE type='table' AND name='scheduler_training_samples'").Scan(&exists)
	if err != nil || exists {
		return err
	}
	return executeSQLiteMigration(ctx, tx, fs, "migrations/sqlite/0007_scheduler_training_samples.sql")
}

func ensureSQLiteSchedulerQualityRollups(ctx context.Context, tx *sql.Tx, fs embed.FS) error {
	var exists bool
	err := tx.QueryRowContext(ctx, "SELECT count(*) > 0 FROM sqlite_master WHERE type='table' AND name='scheduler_quality_rollups'").Scan(&exists)
	if err != nil || exists {
		return err
	}
	return executeSQLiteMigration(ctx, tx, fs, "migrations/sqlite/0008_scheduler_quality_rollups.sql")
}

func executeSQLiteMigration(ctx context.Context, tx *sql.Tx, fs embed.FS, file string) error {
	data, err := fs.ReadFile(file)
	if err != nil {
		return err
	}
	upSQL := string(data)
	if idx := strings.Index(upSQL, "-- +goose Down"); idx != -1 {
		upSQL = upSQL[:idx]
	}
	for _, stmt := range strings.Split(upSQL, ";") {
		if err := execSQLiteStatement(ctx, tx, file, stmt); err != nil {
			return err
		}
	}
	return nil
}

func execSQLiteStatement(ctx context.Context, tx *sql.Tx, file, stmt string) error {
	stmt = strings.TrimSpace(stmt)
	if stmt == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, stmt)
	if err != nil {
		return fmt.Errorf("failed to execute sqlite migration statement %s: %w", file, err)
	}
	return nil
}
