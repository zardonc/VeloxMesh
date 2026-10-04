package sqlite

import (
	"context"
	"time"
)

func (s *semanticCacheRepo) HasCandidates(ctx context.Context, scope, model string) (bool, error) {
	var ready bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM semantic_cache_entries
		WHERE scope = ? AND model = ? AND enabled = 1 AND expires_at > ?)`, scope, model, time.Now().UTC()).Scan(&ready)
	return ready, err
}
