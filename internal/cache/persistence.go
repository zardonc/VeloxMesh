package cache

import (
	"context"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/storage"
)

func (s *SemanticCacheService) persist(ctx context.Context, entry *controlstate.SemanticCacheEntry, vector []float32) error {
	if s.vector == nil {
		return s.storeEntry(ctx, entry, "repo_write")
	}
	pending := *entry
	pending.Enabled = false
	if err := s.storeEntry(ctx, &pending, "repo_write"); err != nil {
		return err
	}
	metadata := map[string]interface{}{"id": entry.ID}
	if entry.UsageID != nil {
		metadata["usage_id"] = *entry.UsageID
	}
	write := storage.CacheVectorWrite{Collection: vectorCollection(entry.Scope, entry.Model), ID: entry.ID, Vector: vector, Metadata: metadata}
	started := time.Now()
	finish := observability.Stage(ctx, "vector_insert")
	err := s.insertVector(ctx, write)
	finish()
	measureOperation("vector_insert", started, err)
	if err != nil {
		return s.fault("store", "vector_error", err)
	}
	return s.storeEntry(ctx, entry, "repo_activate")
}

func (s *SemanticCacheService) storeEntry(ctx context.Context, entry *controlstate.SemanticCacheEntry, stage string) error {
	started := time.Now()
	finish := observability.Stage(ctx, stage)
	err := s.repo.Store(ctx, entry)
	finish()
	measureOperation(stage, started, err)
	if err != nil {
		return s.fault("store", "repository_error", err)
	}
	return nil
}

func (s *SemanticCacheService) insertVector(ctx context.Context, write storage.CacheVectorWrite) error {
	if writer, ok := s.vector.(storage.CacheVectorWriter); ok {
		return writer.InsertCacheEntry(ctx, write)
	}
	return s.vector.Insert(ctx, write.Collection, [][]float32{write.Vector}, []map[string]interface{}{write.Metadata})
}
