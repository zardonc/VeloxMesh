package sqlite

import (
	"context"
	"database/sql"
	"time"

	"veloxmesh/internal/controlstate"
)

type sqliteSessionBlacklistRepo struct {
	db *sql.DB
}

func (r *sqliteSessionBlacklistRepo) IsBlacklisted(ctx context.Context, sessionHash string) (bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT expires_at FROM session_blacklist WHERE session_hash = ?`, sessionHash)
	var expiresAt time.Time
	if err := row.Scan(&expiresAt); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	if time.Now().After(expiresAt) {
		return false, nil // Expired
	}
	return true, nil
}

func (r *sqliteSessionBlacklistRepo) Blacklist(ctx context.Context, record *controlstate.SessionBlacklistRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO session_blacklist (session_hash, reason, expires_at, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(session_hash) DO UPDATE SET
			reason=excluded.reason,
			expires_at=excluded.expires_at,
			created_at=excluded.created_at`,
		record.SessionHash, record.Reason, record.ExpiresAt, record.CreatedAt,
	)
	return err
}

func (r *sqliteSessionBlacklistRepo) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM session_blacklist WHERE expires_at < ?`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
