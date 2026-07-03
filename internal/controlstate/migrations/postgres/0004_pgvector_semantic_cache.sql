CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS semantic_cache_vectors (
    id TEXT PRIMARY KEY,
    collection TEXT NOT NULL,
    embedding vector NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_semantic_cache_vectors_collection
    ON semantic_cache_vectors (collection);

CREATE INDEX IF NOT EXISTS idx_semantic_cache_vectors_embedding_hnsw
    ON semantic_cache_vectors USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
