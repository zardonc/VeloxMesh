package cache

import (
	"context"
	"time"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
)

func (s *SemanticCacheService) scopeReady(ctx context.Context, query CacheLookup) (bool, error) {
	repo, ok := s.repo.(controlstate.SemanticCacheReadinessRepository)
	if !ok {
		return true, nil
	}
	finish := observability.Stage(ctx, "scope_readiness")
	defer finish()
	started := time.Now()
	ready, err := repo.HasCandidates(ctx, query.Scope, query.Model)
	measureOperation("scope_readiness", started, err)
	if err != nil {
		return false, s.fault("lookup", "repository_error", err)
	}
	if !ready {
		recordCacheOutcome("lookup", "scope_not_ready")
	}
	return ready, nil
}
