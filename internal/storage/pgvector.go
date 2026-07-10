package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"veloxmesh/internal/postgresconn"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PGVectorOptions struct {
	Dimension          int
	HNSWM              int
	HNSWEFConstruction int
	SearchEF           int
}

type PGVectorAdapter struct {
	pool      *pgxpool.Pool
	dimension int
	searchEF  int
	opts      PGVectorOptions
}

func NewPGVectorAdapter(ctx context.Context, dsn string, opts PGVectorOptions) (*PGVectorAdapter, error) {
	if opts.Dimension < 1 {
		return nil, errors.New("pgvector dimension must be >= 1")
	}
	cfg, err := postgresconn.PoolConfig(dsn)
	if err != nil {
		return nil, err
	}
	postgresconn.WarnPlaintextCredentials(nil, "pgvector", cfg)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	adapter := &PGVectorAdapter{
		pool:      pool,
		dimension: opts.Dimension,
		searchEF:  opts.SearchEF,
		opts:      opts,
	}
	if err := adapter.ensureSchema(ctx, opts); err != nil {
		pool.Close()
		return nil, err
	}
	return adapter, nil
}

func (p *PGVectorAdapter) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

func (p *PGVectorAdapter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	for _, vector := range vectors {
		if err := p.validateVector(vector); err != nil {
			return err
		}
	}
	for i, vector := range vectors {
		meta := map[string]interface{}{}
		if i < len(metadata) {
			meta = safePGVectorMetadata(metadata[i])
		}
		id, _ := meta["id"].(string)
		if id == "" {
			return errors.New("pgvector metadata requires id")
		}
		data, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		_, err = p.pool.Exec(ctx, `
			INSERT INTO semantic_cache_vectors (id, collection, embedding, metadata)
			VALUES ($1, $2, $3::vector, $4::jsonb)
			ON CONFLICT(id) DO UPDATE SET
				collection=excluded.collection,
				embedding=excluded.embedding,
				metadata=excluded.metadata,
				updated_at=NOW()`,
			id, collection, vectorLiteral(vector), string(data))
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PGVectorAdapter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	if err := p.validateVector(query); err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 1
	}
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if p.searchEF > 0 {
		_, _ = conn.Exec(ctx, fmt.Sprintf("SET hnsw.ef_search = %d", p.searchEF))
	}
	rows, err := conn.Query(ctx, `
		SELECT metadata, 1 - (embedding <=> $1::vector) AS score
		FROM semantic_cache_vectors
		WHERE collection = $2
		ORDER BY embedding <=> $1::vector
		LIMIT $3`, vectorLiteral(query), collection, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []map[string]interface{}{}
	for rows.Next() {
		var raw []byte
		var score float64
		if err := rows.Scan(&raw, &score); err != nil {
			return nil, err
		}
		meta := map[string]interface{}{}
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, err
		}
		meta["score"] = score
		results = append(results, meta)
	}
	return results, rows.Err()
}

func (p *PGVectorAdapter) Delete(ctx context.Context, collection string, filter map[string]interface{}) error {
	id, _ := filter["id"].(string)
	if id == "" {
		return errors.New("pgvector delete requires id filter")
	}
	_, err := p.pool.Exec(ctx, `DELETE FROM semantic_cache_vectors WHERE collection = $1 AND id = $2`, collection, id)
	return err
}

func (p *PGVectorAdapter) ensureSchema(ctx context.Context, opts PGVectorOptions) error {
	m := max(opts.HNSWM, 1)
	ef := max(opts.HNSWEFConstruction, 1)
	_, err := p.pool.Exec(ctx, fmt.Sprintf(`
		CREATE EXTENSION IF NOT EXISTS vector;
		CREATE TABLE IF NOT EXISTS semantic_cache_vectors (
			id TEXT PRIMARY KEY,
			collection TEXT NOT NULL,
			embedding vector(%d) NOT NULL,
			metadata JSONB NOT NULL DEFAULT '{}',
			updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_semantic_cache_vectors_collection
			ON semantic_cache_vectors (collection);
		CREATE INDEX IF NOT EXISTS idx_semantic_cache_vectors_embedding_hnsw
			ON semantic_cache_vectors USING hnsw (embedding vector_cosine_ops)
			WITH (m = %d, ef_construction = %d);`, opts.Dimension, m, ef))
	return err
}

func (p *PGVectorAdapter) EnsureCollection(ctx context.Context, collection string, dimension int) error {
	if dimension < 1 {
		return errors.New("pgvector collection dimension must be >= 1")
	}
	if dimension != p.dimension {
		return fmt.Errorf("pgvector dimension mismatch: got %d, want %d", dimension, p.dimension)
	}
	return p.ensureSchema(ctx, p.opts)
}

func (p *PGVectorAdapter) validateVector(vector []float32) error {
	if len(vector) != p.dimension {
		return fmt.Errorf("pgvector dimension mismatch: got %d, want %d", len(vector), p.dimension)
	}
	return nil
}

func safePGVectorMetadata(meta map[string]interface{}) map[string]interface{} {
	safe := map[string]interface{}{}
	for _, key := range []string{"id", "scope", "model", "usage_id", "response"} {
		if value, ok := meta[key]; ok {
			safe[key] = value
		}
	}
	return safe
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
