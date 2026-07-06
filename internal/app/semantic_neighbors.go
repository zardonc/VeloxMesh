package app

import (
	"context"
	"log/slog"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/storage"
)

func newSemanticNeighborService(ctx context.Context, cfg *config.Config, logger *slog.Logger, m *controlstate.RuntimeProviderManager, repo controlstate.Repository) *scheduler.SemanticNeighborService {
	if !cfg.Scheduler.SemanticNeighborsEnabled {
		return nil
	}
	if repo == nil || cfg.SemanticCacheProvider == "" {
		logger.Warn("semantic neighbors disabled; durable samples or embedding provider unavailable")
		return nil
	}
	vector := newVectorAdapter(ctx, cfg, logger)
	if ensurer, ok := vector.(storage.VectorCollectionEnsurer); ok {
		err := ensurer.EnsureCollection(ctx, scheduler.SemanticNeighborCollection, cfg.Cache.VectorDimension)
		if err != nil {
			logger.Warn("semantic neighbors disabled; vector collection ensure failed", "reason", "startup_ensure", "error", err)
			observability.DefaultMetrics.IncSemanticNeighborError("startup_ensure")
			observability.DefaultMetrics.IncSemanticNeighborFallback("error")
			return nil
		}
	} else if cfg.SemanticCacheVectorStore == "qdrant" || cfg.SemanticCacheVectorStore == "pgvector" {
		logger.Warn("semantic neighbors disabled; vector collection ensure unavailable", "reason", "startup_ensure", "store", cfg.SemanticCacheVectorStore)
		observability.DefaultMetrics.IncSemanticNeighborError("startup_ensure")
		observability.DefaultMetrics.IncSemanticNeighborFallback("error")
		return nil
	}
	return &scheduler.SemanticNeighborService{
		Config: scheduler.SemanticNeighborConfig{
			Enabled:        true,
			MinCount:       cfg.Scheduler.SemanticNeighborsMinCount,
			InputMaxChars:  cfg.Scheduler.SemanticNeighborsInputMaxChars,
			EmbeddingModel: cfg.Scheduler.SemanticNeighborsEmbeddingModel,
		},
		Embedder: semanticNeighborEmbedder(m, cfg.SemanticCacheProvider),
		Vector:   vector,
		Repo:     repo.SchedulerTrainingSamples(),
		Metrics:  observability.DefaultMetrics,
	}
}

func semanticNeighborEmbedder(m *controlstate.RuntimeProviderManager, providerID string) func() providers.EmbedAdapter {
	return func() providers.EmbedAdapter {
		snapshot := m.Snapshot()
		if snapshot == nil || snapshot.Registry == nil {
			return nil
		}
		adapter, err := snapshot.Registry.Get(providerID)
		if err != nil {
			return nil
		}
		embedder, _ := adapter.(providers.EmbedAdapter)
		return embedder
	}
}
