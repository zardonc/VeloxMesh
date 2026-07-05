package app

import (
	"context"
	"log/slog"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/scheduler"
)

func newSemanticNeighborService(ctx context.Context, cfg *config.Config, logger *slog.Logger, m *controlstate.RuntimeProviderManager, repo controlstate.Repository) *scheduler.SemanticNeighborService {
	if !cfg.Scheduler.SemanticNeighborsEnabled {
		return nil
	}
	if repo == nil || cfg.SemanticCacheProvider == "" {
		logger.Warn("semantic neighbors disabled; durable samples or embedding provider unavailable")
		return nil
	}
	return &scheduler.SemanticNeighborService{
		Config: scheduler.SemanticNeighborConfig{
			Enabled:  true,
			MinCount: cfg.Scheduler.SemanticNeighborsMinCount,
		},
		Embedder: semanticNeighborEmbedder(m, cfg.SemanticCacheProvider),
		Vector:   newVectorAdapter(ctx, cfg, logger),
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
