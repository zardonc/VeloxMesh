package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
)

const exactScopePrefix = "exact-v1:"

func isExactScope(scope string) bool { return strings.HasPrefix(scope, exactScopePrefix) }

// Full question bytes and opaque scope participate; no whitespace/case folding.
func exactAnswerID(query CacheLookup) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{query.Scope, query.Model, query.Text}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func (s *SemanticCacheService) lookupExact(ctx context.Context, query CacheLookup) (*controlstate.SemanticCacheEntry, error) {
	if s.repo == nil {
		return nil, nil
	}
	defer observability.Stage(ctx, "repo_read")()
	entry, err := s.repo.GetCandidate(ctx, exactAnswerID(query), query.Scope, query.Model)
	if err != nil {
		return nil, s.fault("lookup", "repository_error", err)
	}
	if !validExactEntry(entry, query) {
		recordCacheOutcome("lookup", "miss")
		return nil, nil
	}
	if err := s.repo.RecordHit(ctx, entry.ID); err != nil {
		return nil, s.fault("lookup", "repository_error", err)
	}
	recordCacheOutcome("lookup", "hit")
	return entry, nil
}

func validExactEntry(entry *controlstate.SemanticCacheEntry, query CacheLookup) bool {
	return entry != nil && entry.ID == exactAnswerID(query) && entry.Scope == query.Scope && entry.Model == query.Model &&
		entry.Enabled && entry.ExpiresAt.After(time.Now()) && validCachedChoices(entry.Response)
}

func (s *SemanticCacheService) storeExact(ctx context.Context, write CacheWrite) error {
	if !validCachedChoices(write.Response) {
		return s.fault("store", "invalid_entry", nil)
	}
	now := time.Now().UTC()
	entry := &controlstate.SemanticCacheEntry{
		ID:    exactAnswerID(CacheLookup{Scope: write.Scope, Model: write.Model, Text: write.Text}),
		Scope: write.Scope, Model: write.Model, Vector: []byte{}, Response: write.Response,
		UsageID: write.UsageID, Enabled: true, CreatedAt: now, ExpiresAt: now.Add(s.config.TTL),
	}
	return s.storeEntry(ctx, entry, "repo_write")
}
