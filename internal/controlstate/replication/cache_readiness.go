package replication

import (
	"context"
	"veloxmesh/internal/controlstate"
)

func (s *semanticCacheRepo) HasCandidates(ctx context.Context, scope, model string) (bool, error) {
	if repo, ok := s.underlying.(controlstate.SemanticCacheReadinessRepository); ok {
		return repo.HasCandidates(ctx, scope, model)
	}
	// Unknown repository implementations retain the existing search path.
	return true, nil
}
