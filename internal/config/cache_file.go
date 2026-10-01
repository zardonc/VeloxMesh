package config

func cacheConfigFromEnv() CacheConfig {
	return CacheConfig{
		Enabled:         getEnv("SEMANTIC_CACHE_ENABLED", "false") == "true",
		Provider:        getEnv("SEMANTIC_CACHE_PROVIDER", ""),
		EmbeddingModel:  getEnv("SEMANTIC_CACHE_EMBEDDING_MODEL", ""),
		VectorStore:     getEnv("SEMANTIC_CACHE_VECTOR_STORE", ""),
		VectorDimension: getEnvInt("SEMANTIC_CACHE_VECTOR_DIMENSION", defaultSemanticCacheVectorDimension),
		TTL:             getEnv("SEMANTIC_CACHE_TTL", ""),
		Threshold:       float32(getEnvFloat("SEMANTIC_CACHE_THRESHOLD", 0)),
		MaxCandidates:   getEnvInt("SEMANTIC_CACHE_MAX_CANDIDATES", 0),
		ReadTimeout:     getEnv("SEMANTIC_CACHE_READ_TIMEOUT", ""),
		ReadConcurrency: getEnvInt("SEMANTIC_CACHE_READ_CONCURRENCY", 0),
		WriteTimeout:    getEnv("SEMANTIC_CACHE_WRITE_TIMEOUT", ""),
		WriteWorkers:    getEnvInt("SEMANTIC_CACHE_WRITE_WORKERS", 0),
		QueueCapacity:   getEnvInt("SEMANTIC_CACHE_QUEUE_CAPACITY", 0),
		ShutdownGrace:   getEnv("SEMANTIC_CACHE_SHUTDOWN_GRACE", ""),
		PGVector: PGVectorConfig{
			IndexType:       getEnv("PGVECTOR_INDEX_TYPE", defaultPGVectorIndexType),
			HNSWM:           getEnvInt("PGVECTOR_HNSW_M", defaultPGVectorHNSWM),
			HNSWEFConstruct: getEnvInt("PGVECTOR_HNSW_EF_CONSTRUCTION", defaultPGVectorHNSWEFConstruction),
			SearchEF:        getEnvInt("PGVECTOR_SEARCH_EF", defaultPGVectorSearchEF),
		},
		Qdrant: QdrantConfig{
			Addr:   getEnv("QDRANT_ADDR", ""),
			APIKey: getEnv("QDRANT_API_KEY", ""),
		},
	}
}

func mergeCacheConfig(dst *CacheConfig, src *cacheFileConfig) {
	if src == nil {
		return
	}
	if src.Enabled != nil {
		dst.Enabled = *src.Enabled
	}
	if src.Provider != "" {
		dst.Provider = src.Provider
	}
	if src.EmbeddingModel != "" {
		dst.EmbeddingModel = src.EmbeddingModel
	}
	if src.VectorStore != "" {
		dst.VectorStore = src.VectorStore
	}
	if src.VectorDimension != nil {
		dst.VectorDimension = *src.VectorDimension
	}
	if src.TTL != "" {
		dst.TTL = src.TTL
	}
	if src.Threshold != nil {
		dst.Threshold = *src.Threshold
	}
	if src.MaxCandidates != nil {
		dst.MaxCandidates = *src.MaxCandidates
	}
	if src.UseCases != nil {
		dst.UseCases = *src.UseCases
	}
	mergeCacheBounds(dst, src)
	mergePGVectorConfig(&dst.PGVector, src.PGVector)
	mergeQdrantConfig(&dst.Qdrant, src.Qdrant)
}

func mergePGVectorConfig(dst *PGVectorConfig, src PGVectorConfig) {
	if src.IndexType != "" {
		dst.IndexType = src.IndexType
	}
	if src.HNSWM != 0 {
		dst.HNSWM = src.HNSWM
	}
	if src.HNSWEFConstruct != 0 {
		dst.HNSWEFConstruct = src.HNSWEFConstruct
	}
	if src.SearchEF != 0 {
		dst.SearchEF = src.SearchEF
	}
}

func mergeQdrantConfig(dst *QdrantConfig, src QdrantConfig) {
	if src.Addr != "" {
		dst.Addr = src.Addr
	}
	if src.APIKey != "" {
		dst.APIKey = src.APIKey
	}
}
