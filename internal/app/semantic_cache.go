package app

import (
	"context"
	"log/slog"
	"strings"
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
	store := strings.ToLower(strings.TrimSpace(cfg.SemanticCacheVectorStore))
	switch store {
	case "lancedb":
		adapter, _ := newLanceDBVectorAdapter(logger, true)
		return adapter
	case "qdrant":
		return newQdrantVectorAdapter(ctx, cfg, logger)
	case "pgvector":
		adapter, err := storage.NewPGVectorAdapter(ctx, cfg.ControlStateDSN, storage.PGVectorOptions{
			Dimension:          cfg.SemanticCacheVectorDimension,
			HNSWM:              cfg.PGVectorHNSWM,
			HNSWEFConstruction: cfg.PGVectorHNSWEFConstruction,
			SearchEF:           cfg.PGVectorSearchEF,
		})
		if err == nil {
			return adapter
		}
		logger.Warn("failed to initialize pgvector; vector capabilities degraded", "error", err)
		return storage.NewDegradedVectorAdapter()
	default:
		adapter, _ := newLanceDBVectorAdapter(logger, false)
		return adapter
	}
}

func newLanceDBVectorAdapter(logger *slog.Logger, explicit bool) (storage.VectorAdapter, bool) {
	adapter, err := storage.NewLanceDBVectorAdapter("data/lancedb")
	if err == nil {
		return adapter, true
	}
	if explicit {
		logger.Warn("failed to initialize LanceDB; vector capabilities degraded", "error", err)
		return storage.NewDegradedVectorAdapter(), true
	}
	return storage.NewNoopVectorAdapter(), false
}

func newQdrantVectorAdapter(ctx context.Context, cfg *config.Config, logger *slog.Logger) storage.VectorAdapter {
	adapter, err := storage.NewQdrantVectorAdapter(cfg.QdrantAddr, cfg.QdrantAPIKey)
	if err == nil {
		return adapter
	}
	logger.Warn("failed to initialize Qdrant; evaluating fallback", "error", err)
	if fallback, ok := newLanceDBVectorAdapter(logger, false); ok {
		logger.Info("activated LanceDB fallback for vector store")
		return fallback
	}
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
