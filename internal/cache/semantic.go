package cache

import (
	"context"
	"encoding/binary"
	"math"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
)

type SemanticCacheConfig struct {
	Enabled       bool
	Threshold     float32
	MaxCandidates int
	TTL           time.Duration
}

type SemanticCacheService struct {
	config  SemanticCacheConfig
	repo    controlstate.SemanticCacheRepository
	adapter providers.EmbedAdapter
}

func NewSemanticCacheService(config SemanticCacheConfig, repo controlstate.SemanticCacheRepository, adapter providers.EmbedAdapter) *SemanticCacheService {
	return &SemanticCacheService{
		config:  config,
		repo:    repo,
		adapter: adapter,
	}
}

func (s *SemanticCacheService) Lookup(ctx context.Context, scope, model string, text string) (*controlstate.SemanticCacheEntry, error) {
	if !s.config.Enabled || s.repo == nil || s.adapter == nil {
		return nil, nil // Miss
	}

	// 1. Get candidates
	candidates, err := s.repo.ListCandidates(ctx, scope, model)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil // Miss
	}

	if len(candidates) > s.config.MaxCandidates {
		candidates = candidates[:s.config.MaxCandidates]
	}

	// 2. Embed input text
	req := &llm.EmbeddingRequest{
		Model: "text-embedding-3-small", // or whatever default model we use for embeddings, but this is provider specific. We should let adapter decide if it's not set.
		Input: []string{text},
	}
	// Let the adapter define the default model if needed, or we pass a generic one
	resp, err := s.adapter.Embed(ctx, req)
	if err != nil || len(resp.Data) == 0 {
		return nil, err // Miss due to error
	}
	inputVector := resp.Data[0].Embedding

	// 3. Compute similarities
	var bestMatch *controlstate.SemanticCacheEntry
	var bestScore float32 = -1.0

	for _, cand := range candidates {
		candVector := bytesToFloats(cand.Vector)
		score := cosineSimilarity(inputVector, candVector)
		if score > bestScore {
			bestScore = score
			bestMatch = cand
		}
	}

	if bestScore >= s.config.Threshold {
		// Record hit
		_ = s.repo.RecordHit(ctx, bestMatch.ID)
		return bestMatch, nil
	}

	return nil, nil
}

func (s *SemanticCacheService) Store(ctx context.Context, id, scope, model string, text string, response string, usageID *string) error {
	if !s.config.Enabled || s.repo == nil || s.adapter == nil {
		return nil
	}

	req := &llm.EmbeddingRequest{
		Model: "text-embedding-3-small",
		Input: []string{text},
	}
	resp, err := s.adapter.Embed(ctx, req)
	if err != nil || len(resp.Data) == 0 {
		return err
	}
	vector := resp.Data[0].Embedding

	entry := &controlstate.SemanticCacheEntry{
		ID:        id,
		Scope:     scope,
		Model:     model,
		Vector:    floatsToBytes(vector),
		Response:  response,
		UsageID:   usageID,
		HitCount:  0,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(s.config.TTL).UTC(),
	}

	return s.repo.Store(ctx, entry)
}

func cosineSimilarity(a, b []float32) float32 {
	var dotProduct, normA, normB float32
	for i := 0; i < len(a) && i < len(b); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / float32(math.Sqrt(float64(normA))*math.Sqrt(float64(normB)))
}

func floatsToBytes(floats []float32) []byte {
	bytes := make([]byte, len(floats)*4)
	for i, f := range floats {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(f))
	}
	return bytes
}

func bytesToFloats(b []byte) []float32 {
	floats := make([]float32, len(b)/4)
	for i := range floats {
		floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return floats
}
