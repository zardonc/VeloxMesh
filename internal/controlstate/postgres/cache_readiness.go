package postgres

import (
	"context"
	"time"
)

func (s *semanticCacheRepo) HasCandidates(ctx context.Context, scope, model string) (bool, error) {
	var ready bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semantic_cache_entries
		WHERE scope = $1 AND model = $2 AND enabled = true AND expires_at > $3)`, scope, model, time.Now().UTC()).Scan(&ready)
	return ready, err
}
