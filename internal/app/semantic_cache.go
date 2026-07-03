package app

import (
	"context"
	"log/slog"
	"time"

	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/storage"
)

func newSemanticCacheService(ctx context.Context, cfg *config.Config, logger *slog.Logger, m *controlstate.RuntimeProviderManager, repo controlstate.Repository) *cache.SemanticCacheService {
	if !cfg.SemanticCacheEnabled || repo == nil || cfg.SemanticCacheProvider == "" {
		return nil
	}
	snapshot := m.Snapshot()
	if snapshot == nil || snapshot.Registry == nil {
		logger.Warn("cannot initialize semantic cache: provider registry not ready")
		return nil
	}
	adapter, err := snapshot.Registry.Get(cfg.SemanticCacheProvider)
	if err != nil {
		logger.Warn("semantic cache provider not found", "provider", cfg.SemanticCacheProvider)
		return nil
	}
	embedAdapter, ok := adapter.(providers.EmbedAdapter)
	if !ok {
		logger.Warn("semantic cache provider is not an embed adapter", "provider", cfg.SemanticCacheProvider)
		return nil
	}
	return cache.NewSemanticCacheService(cache.SemanticCacheConfig{
		Enabled:       true,
		Threshold:     0.9,
		MaxCandidates: 10,
		TTL:           24 * time.Hour,
	}, repo.SemanticCache(), newVectorAdapter(ctx, cfg, logger), embedAdapter)
}

func newVectorAdapter(ctx context.Context, cfg *config.Config, logger *slog.Logger) storage.VectorAdapter {
	switch cfg.SemanticCacheVectorStore {
	case "lancedb":
		adapter, err := storage.NewLanceDBVectorAdapter("data/lancedb")
		if err == nil {
			return adapter
		}
		logger.Warn("failed to initialize LanceDB (Plan 3 Edge only); vector capabilities degraded", "error", err)
		return storage.NewDegradedVectorAdapter()
	case "qdrant":
		return newQdrantVectorAdapter(ctx, cfg, logger)
	case "pgvector":
		logger.Warn("pgvector vector adapter pending; vector capabilities degraded")
		return storage.NewDegradedVectorAdapter()
	default:
		return storage.NewNoopVectorAdapter()
	}
}

func newQdrantVectorAdapter(ctx context.Context, cfg *config.Config, logger *slog.Logger) storage.VectorAdapter {
	adapter, err := storage.NewQdrantVectorAdapter(cfg.QdrantAddr, cfg.QdrantAPIKey)
	if err == nil {
		return adapter
	}
	logger.Warn("failed to initialize Qdrant; evaluating fallback", "error", err)
	if !cfg.RedisEnabled {
		return storage.NewDegradedVectorAdapter()
	}
	redisAdapter, err := storage.NewRedisVSSVectorAdapter(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisNamespace)
	if err != nil {
		logger.Warn("failed to initialize Redis VSS fallback; vector capabilities degraded", "error", err)
		return storage.NewDegradedVectorAdapter()
	}
	logger.Info("activated Redis VSS fallback for vector store")
	return redisAdapter
}
