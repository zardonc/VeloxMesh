package sqlite

import (
	"context"
	"time"

	"veloxmesh/internal/controlstate"
)

func (s *semanticCacheRepo) ListDiscardable(ctx context.Context, query controlstate.CacheCleanupQuery) ([]controlstate.CacheGarbage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, scope, model, created_at, expires_at FROM semantic_cache_entries
		WHERE created_at < ? AND (enabled = 0 OR expires_at <= ?) ORDER BY created_at LIMIT ?`, query.PendingBefore, query.ExpiredBefore, query.Limit)
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM semantic_cache_entries WHERE id = ? AND scope = ? AND model = ?
		AND created_at = ? AND expires_at = ? AND (enabled = 0 OR expires_at <= ?)`, entry.ID, entry.Scope, entry.Model, entry.CreatedAt, entry.ExpiresAt, time.Now().UTC())
	return err
}
