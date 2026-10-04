package cache

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
)

const semanticCleanupInterval = 10 * time.Second
const semanticCleanupBatch = 32

// A single existing write worker reaps cache-only rows. Pending entries receive
// an extra full interval beyond the write deadline, avoiding active writers.
func (s *SemanticCacheService) cleanup() {
	repo, ok := s.repo.(controlstate.SemanticCacheCleanupRepository)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(s.writeCtx, s.config.WriteTimeout)
	defer cancel()
	defer observability.Stage(ctx, "cache_cleanup")()
	now := time.Now().UTC()
	entries, err := repo.ListDiscardable(ctx, controlstate.CacheCleanupQuery{PendingBefore: now.Add(-s.config.WriteTimeout - semanticCleanupInterval), ExpiredBefore: now, Limit: semanticCleanupBatch})
	if err != nil {
		_ = s.fault("cleanup", "repository_error", err)
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			_ = s.fault("cleanup", "timeout", ctx.Err())
			return
		}
		if err := s.discard(ctx, repo, entry); err != nil {
			continue
		}
		recordCacheOutcome("cleanup", "removed")
	}
}

func (s *SemanticCacheService) discard(ctx context.Context, repo controlstate.SemanticCacheCleanupRepository, entry controlstate.CacheGarbage) error {
	if s.vector != nil && !isExactScope(entry.Scope) {
		err := s.vector.Delete(ctx, vectorCollection(entry.Scope, entry.Model), map[string]interface{}{"id": entry.ID})
		// A missing collection confirms that no points remain for this entry.
		if err != nil && status.Code(err) != codes.NotFound {
			return s.fault("cleanup", "vector_error", err)
		}
	}
	if err := repo.RemoveDiscardable(ctx, entry); err != nil {
		return s.fault("cleanup", "repository_error", err)
	}
	return nil
}
