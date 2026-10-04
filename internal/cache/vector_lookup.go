package cache

import (
	"context"
	"time"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/storage"
)

func (s *SemanticCacheService) searchVector(ctx context.Context, query CacheLookup, result CacheLookupResult) (CacheLookupResult, bool, error) {
	if s.vector == nil {
		return result, false, nil
	}
	started := time.Now()
	finish := observability.Stage(ctx, "vector_search")
	results, err := s.vector.Search(ctx, vectorCollection(query.Scope, query.Model), result.Vector, s.config.MaxCandidates)
	finish()
	measureOperation("vector_search", started, err)
	if err != nil {
		return result, true, s.fault("lookup", "vector_error", err)
	}
	entry, err := s.lookupVectorResult(ctx, query.Scope, query.Model, results)
	if err != nil || entry != nil {
		return CacheLookupResult{Entry: entry, Vector: result.Vector}, true, err
	}
	if _, offlineFallback := s.vector.(*storage.NoopVectorAdapter); offlineFallback {
		return result, false, nil
	}
	recordCacheOutcome("lookup", "miss")
	return result, true, nil
}
