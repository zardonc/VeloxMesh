package postgres

import (
	"context"
	"time"

	"veloxmesh/internal/controlstate"
)

func (s *semanticCacheRepo) ListDiscardable(ctx context.Context, query controlstate.CacheCleanupQuery) ([]controlstate.CacheGarbage, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, scope, model, created_at, expires_at FROM semantic_cache_entries
		WHERE created_at < $1 AND (enabled = false OR expires_at <= $2) ORDER BY created_at LIMIT $3`, query.PendingBefore, query.ExpiredBefore, query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []controlstate.CacheGarbage
	for rows.Next() {
		var entry controlstate.CacheGarbage
		if err := rows.Scan(&entry.ID, &entry.Scope, &entry.Model, &entry.CreatedAt, &entry.ExpiresAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *semanticCacheRepo) RemoveDiscardable(ctx context.Context, entry controlstate.CacheGarbage) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM semantic_cache_entries WHERE id = $1 AND scope = $2 AND model = $3
		AND created_at = $4 AND expires_at = $5 AND (enabled = false OR expires_at <= $6)`, entry.ID, entry.Scope, entry.Model, entry.CreatedAt, entry.ExpiresAt, time.Now().UTC())
	return err
}
