package cache

// Failure cases: absent/pending/expired/foreign scope triggers remote lookup;
// negative readiness persists after activation; active scope loses its vector
// collection; repository errors are hidden. Use the real SQLite lifecycle.
import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/llm"
)

func readinessFixture(t *testing.T) (*sqlite.Repository, *boundedEmbed, *SemanticCacheService) {
	t.Helper()
	repo, err := sqlite.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	adapter := &boundedEmbed{response: &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}}
	service := NewSemanticCacheService(boundsConfig(), repo.SemanticCache(), failingVector{}, adapter)
	t.Cleanup(service.Close)
	return repo, adapter, service
}

func TestSemanticCacheUnreadyScopesSkipRemoteRead(t *testing.T) {
	cases := map[string]controlstate.SemanticCacheEntry{
		"absent":        {},
		"pending":       {ID: "pending", Scope: "scope", Model: "model"},
		"expired":       {ID: "expired", Scope: "scope", Model: "model", Enabled: true, ExpiresAt: time.Now().UTC().Add(-time.Hour)},
		"foreign scope": {ID: "foreign", Scope: "other", Model: "model", Enabled: true},
		"foreign model": {ID: "foreign", Scope: "scope", Model: "other", Enabled: true},
	}
	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			repo, adapter, service := readinessFixture(t)
			entry := original
			entry.Vector = floatsToBytes([]float32{1, 0})
			if entry.ExpiresAt.IsZero() {
				entry.ExpiresAt = time.Now().UTC().Add(time.Hour)
			}
			if entry.ID != "" {
				if err := repo.SemanticCache().Store(context.Background(), &entry); err != nil {
					t.Fatal(err)
				}
			}
			result, err := service.LookupWithVector(context.Background(), CacheLookup{Scope: "scope", Model: "model", Text: "question"})
			if err != nil || result.Entry != nil || result.Vector != nil || adapter.calls.Load() != 0 {
				t.Fatalf("unready scope remote read: calls=%d error=%v", adapter.calls.Load(), err)
			}
		})
	}
}

func TestSemanticCacheScopeActivationIsNotNegativelyCached(t *testing.T) {
	repo, adapter, service := readinessFixture(t)
	query := CacheLookup{Scope: "scope", Model: "model", Text: "question"}
	if _, err := service.LookupWithVector(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	entry := controlstate.SemanticCacheEntry{ID: "ready", Scope: query.Scope, Model: query.Model, Vector: floatsToBytes([]float32{1, 0}), Enabled: true, ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := repo.SemanticCache().Store(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	if _, err := service.LookupWithVector(context.Background(), query); err == nil || adapter.calls.Load() != 1 {
		t.Fatalf("active scope missing vector must surface: calls=%d error=%v", adapter.calls.Load(), err)
	}
}

func TestSemanticCacheReadinessRepositoryFailureSurfaces(t *testing.T) {
	repo, adapter, service := readinessFixture(t)
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.LookupWithVector(context.Background(), CacheLookup{Scope: "scope", Model: "model", Text: "question"}); err == nil || adapter.calls.Load() != 0 {
		t.Fatalf("repository failure must precede remote I/O: calls=%d error=%v", adapter.calls.Load(), err)
	}
}
