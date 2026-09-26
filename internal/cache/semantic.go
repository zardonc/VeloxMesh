package cache

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/storage"
)

type SemanticCacheConfig struct {
	Enabled         bool
	Threshold       float32
	MaxCandidates   int
	TTL             time.Duration
	EmbeddingModel  string
	VectorDimension int
	UseCases        []SemanticCacheUseCase
}

type SemanticCacheUseCase struct {
	APIKeyIDs        []string
	UseCaseID        string
	KnowledgeVersion string
	TargetModel      string
	SystemPrompt     string
}

type SemanticCacheService struct {
	config  SemanticCacheConfig
	repo    controlstate.SemanticCacheRepository
	vector  storage.VectorAdapter
	adapter providers.EmbedAdapter
}

func NewSemanticCacheService(config SemanticCacheConfig, repo controlstate.SemanticCacheRepository, vector storage.VectorAdapter, adapter providers.EmbedAdapter) *SemanticCacheService {
	return &SemanticCacheService{
		config:  config,
		repo:    repo,
		vector:  vector,
		adapter: adapter,
	}
}

func (s *SemanticCacheService) Eligible(identityID, role string, req *llm.LLMRequest) (string, bool) {
	if !s.config.Enabled || role == "admin" || identityID == "" || !validCacheRequest(req) {
		return "", false
	}
	for _, useCase := range s.config.UseCases {
		if useCase.TargetModel == req.Model && useCase.SystemPrompt == req.Messages[0].Content && contains(useCase.APIKeyIDs, identityID) {
			return cacheScope(identityID, useCase, s.config), true
		}
	}
	return "", false
}

func validCacheRequest(req *llm.LLMRequest) bool {
	if req == nil || req.CacheUnsafe || req.Stream || req.RouteOverride != "" || req.ToolRequirements.UsesProtocol() || len(req.Tools) != 0 || req.ToolChoice != nil {
		return false
	}
	if req.Temperature == nil || *req.Temperature != 0 || req.MaxTokens == nil || *req.MaxTokens != 256 || len(req.Messages) != 2 {
		return false
	}
	return req.Messages[0].Role == llm.RoleSystem && req.Messages[0].Content != "" && len(req.Messages[0].MultiContent) == 0 && len(req.Messages[0].ToolCalls) == 0 && req.Messages[0].ToolCallID == "" && req.Messages[1].Role == llm.RoleUser && req.Messages[1].Content != "" && len(req.Messages[1].MultiContent) == 0 && len(req.Messages[1].ToolCalls) == 0 && req.Messages[1].ToolCallID == ""
}

func cacheScope(identityID string, useCase SemanticCacheUseCase, config SemanticCacheConfig) string {
	identity := strings.Join([]string{identityID, useCase.UseCaseID, useCase.KnowledgeVersion, useCase.TargetModel, useCase.SystemPrompt, config.EmbeddingModel, fmt.Sprint(config.VectorDimension), "temperature=0", "max_tokens=256"}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s *SemanticCacheService) Lookup(ctx context.Context, scope, model string, text string) (*controlstate.SemanticCacheEntry, error) {
	if !s.config.Enabled || s.repo == nil || s.adapter == nil {
		return nil, nil // Miss
	}

	// 1. Embed input text
	req := &llm.EmbeddingRequest{
		Model: s.config.EmbeddingModel,
		Input: []string{text},
	}
	// Let the adapter define the default model if needed, or we pass a generic one
	resp, err := s.adapter.Embed(ctx, req)
	if err != nil || resp == nil || len(resp.Data) == 0 {
		return nil, err // Miss due to error
	}
	inputVector := resp.Data[0].Embedding
	if len(inputVector) != s.config.VectorDimension {
		return nil, nil
	}

	// 2. If vector adapter is configured, use it for search
	if s.vector != nil {
		results, err := s.vector.Search(ctx, vectorCollection(scope, model), inputVector, s.config.MaxCandidates)
		if err != nil {
			// Log error (in a real app via observability/logger), degrade gracefully to miss
			return nil, nil
		}
		entry, err := s.lookupVectorResult(ctx, scope, model, results)
		if err != nil || entry != nil {
			return entry, err
		}
	}

	// 3. Fallback to SQLite (original behavior)
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

	// Compute similarities
	var bestMatch *controlstate.SemanticCacheEntry
	var bestScore float32 = -1.0

	for _, cand := range candidates {
		candVector := bytesToFloats(cand.Vector)
		if len(candVector) != s.config.VectorDimension {
			continue
		}
		score := cosineSimilarity(inputVector, candVector)
		if score > bestScore {
			bestScore = score
			bestMatch = cand
		}
	}

	if bestScore >= s.config.Threshold && bestMatch != nil && validCachedChoices(bestMatch.Response) {
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
		Model: s.config.EmbeddingModel,
		Input: []string{text},
	}
	resp, err := s.adapter.Embed(ctx, req)
	if err != nil || resp == nil || len(resp.Data) == 0 {
		return err
	}
	vector := resp.Data[0].Embedding
	if len(vector) != s.config.VectorDimension {
		return nil
	}

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

	if s.vector != nil {
		meta := map[string]interface{}{
			"id": id,
		}
		if usageID != nil {
			meta["usage_id"] = *usageID
		}

		if err := s.repo.Store(ctx, entry); err != nil {
			return err
		}
		err := s.vector.Insert(ctx, vectorCollection(scope, model), [][]float32{vector}, []map[string]interface{}{meta})
		if err != nil {
			// Log error but do not fail the store operation if degraded
			return nil
		}
		return nil
	}

	return s.repo.Store(ctx, entry)
}

func (s *SemanticCacheService) lookupVectorResult(ctx context.Context, scope, model string, results []map[string]interface{}) (*controlstate.SemanticCacheEntry, error) {
	for _, result := range results {
		score, hasScore := result["score"].(float64)
		if !hasScore || float32(score) < s.config.Threshold {
			continue
		}
		id, _ := result["id"].(string)
		if id == "" {
			continue
		}
		entry, err := s.repo.GetCandidate(ctx, id, scope, model)
		if err != nil {
			return nil, err
		}
		if entry != nil && validCachedChoices(entry.Response) {
			_ = s.repo.RecordHit(ctx, entry.ID)
			return entry, nil
		}
	}
	return nil, nil
}

func validCachedChoices(response string) bool {
	var choices []llm.Choice
	if json.Unmarshal([]byte(response), &choices) != nil || len(choices) == 0 {
		return false
	}
	for _, choice := range choices {
		if choice.Message.Role != llm.RoleAssistant {
			return false
		}
	}
	return true
}

func vectorCollection(scope, model string) string {
	digest := sha256.Sum256([]byte(scope + "\x00" + model))
	return fmt.Sprintf("semantic_cache:%s", hex.EncodeToString(digest[:]))
}

func safeCollectionPart(value string) string {
	return strings.NewReplacer(":", "_", "\n", "_", "\r", "_").Replace(value)
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
