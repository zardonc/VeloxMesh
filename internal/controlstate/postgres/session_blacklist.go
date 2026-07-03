package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

type sessionBlacklistRepo struct {
	pool *pgxpool.Pool
}

func (r *sessionBlacklistRepo) IsBlacklisted(ctx context.Context, sessionHash string) (bool, error) {
	row := r.pool.QueryRow(ctx, `SELECT expires_at FROM session_blacklist WHERE session_hash = $1`, sessionHash)
	var expiresAt time.Time
	if err := row.Scan(&expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return time.Now().UTC().Before(expiresAt), nil
}

func (r *sessionBlacklistRepo) Blacklist(ctx context.Context, record *controlstate.SessionBlacklistRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO session_blacklist (session_hash, reason, expires_at, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT(session_hash) DO UPDATE SET
			reason=excluded.reason,
			expires_at=excluded.expires_at,
			created_at=excluded.created_at`,
		record.SessionHash, record.Reason, record.ExpiresAt, record.CreatedAt)
	return err
}

func (r *sessionBlacklistRepo) PurgeExpired(ctx context.Context) (int64, error) {
	res, err := r.pool.Exec(ctx, `DELETE FROM session_blacklist WHERE expires_at < $1`, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}
