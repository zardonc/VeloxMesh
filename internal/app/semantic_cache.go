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

func newSemanticCacheService(ctx context.Context, deps semanticCacheDeps, repo controlstate.Repository) *cache.SemanticCacheService {
	cfg := deps.cfg
	if !cfg.SemanticCacheEnabled || repo == nil || !cfg.Cache.HasAnswerReuse() {
		return nil
	}
	var embedAdapter providers.EmbedAdapter
	var vector storage.VectorAdapter
	if cfg.Cache.HasSemanticReuse() {
		embedAdapter, vector = semanticCacheAdapters(ctx, deps)
		if embedAdapter == nil {
			return nil
		}
	}
	return cache.NewSemanticCacheService(semanticCacheOptions(cfg), repo.SemanticCache(), vector, embedAdapter)
}

type semanticCacheDeps struct {
	cfg     *config.Config
	logger  *slog.Logger
	manager *controlstate.RuntimeProviderManager
}

func semanticCacheAdapters(ctx context.Context, deps semanticCacheDeps) (providers.EmbedAdapter, storage.VectorAdapter) {
	cfg, logger, m := deps.cfg, deps.logger, deps.manager
	snapshot := m.Snapshot()
	if snapshot == nil || snapshot.Registry == nil {
		logger.Warn("cannot initialize semantic cache: provider registry not ready")
		return nil, nil
	}
	adapter, err := snapshot.Registry.Get(cfg.SemanticCacheProvider)
	if err != nil {
		logger.Warn("semantic cache provider not found", "provider", cfg.SemanticCacheProvider)
		return nil, nil
	}
	embedAdapter, ok := adapter.(providers.EmbedAdapter)
	if !ok {
		logger.Warn("semantic cache provider is not an embed adapter", "provider", cfg.SemanticCacheProvider)
		return nil, nil
	}
	return embedAdapter, newVectorAdapter(ctx, cfg, logger)
}

func semanticCacheOptions(cfg *config.Config) cache.SemanticCacheConfig {
	return cache.SemanticCacheConfig{
		Enabled:               true,
		Threshold:             cfg.Cache.Threshold,
		MaxCandidates:         cfg.Cache.MaxCandidates,
		TTL:                   cacheTTL(cfg.Cache.TTL),
		EmbeddingModel:        cfg.Cache.EmbeddingModel,
		EmbeddingInputPrefix:  cfg.Cache.EmbeddingInputPrefix,
		EmbeddingMemoCapacity: cfg.Cache.EmbeddingMemoCapacity,
		EmbeddingMemoTTL:      cacheTTL(cfg.Cache.EmbeddingMemoTTL),
		EmbeddingProvider:     cfg.Cache.Provider,
		VectorDimension:       cfg.Cache.VectorDimension,
		UseCases:              cacheUseCases(cfg.Cache.UseCases),
		ReadTimeout:           cacheTTL(cfg.Cache.ReadTimeout), ReadConcurrency: cfg.Cache.ReadConcurrency,
		WriteTimeout: cacheTTL(cfg.Cache.WriteTimeout), WriteWorkers: cfg.Cache.WriteWorkers,
		QueueCapacity: cfg.Cache.QueueCapacity, ShutdownGrace: cacheTTL(cfg.Cache.ShutdownGrace),
	}
}

func cacheTTL(value string) time.Duration {
	ttl, _ := time.ParseDuration(value)
	return ttl
}

func cacheUseCases(useCases []config.CacheUseCaseConfig) []cache.SemanticCacheUseCase {
	result := make([]cache.SemanticCacheUseCase, 0, len(useCases))
	for _, useCase := range useCases {
		result = append(result, cache.SemanticCacheUseCase{
			ReuseMode: useCase.ReuseMode,
			APIKeyIDs: append([]string(nil), useCase.APIKeyIDs...), UseCaseID: useCase.UseCaseID,
			KnowledgeVersion: useCase.KnowledgeVersion, TargetModel: useCase.TargetModel, SystemPrompt: useCase.SystemPrompt,
		})
	}
	return result
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
