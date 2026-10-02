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
	"sync"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/storage"
)

type SemanticCacheConfig struct {
	Enabled           bool
	Threshold         float32
	MaxCandidates     int
	TTL               time.Duration
	EmbeddingModel    string
	EmbeddingProvider string
	VectorDimension   int
	UseCases          []SemanticCacheUseCase
	ReadTimeout       time.Duration
	ReadConcurrency   int
	WriteTimeout      time.Duration
	WriteWorkers      int
	QueueCapacity     int
	ShutdownGrace     time.Duration
}

type SemanticCacheUseCase struct {
	APIKeyIDs        []string
	UseCaseID        string
	KnowledgeVersion string
	TargetModel      string
	SystemPrompt     string
}

type SemanticCacheService struct {
	config       SemanticCacheConfig
	repo         controlstate.SemanticCacheRepository
	vector       storage.VectorAdapter
	adapter      providers.EmbedAdapter
	reads        chan struct{}
	writes       chan CacheWrite
	writeCtx     context.Context
	cancelWrites context.CancelFunc
	workers      sync.WaitGroup
	queueMu      sync.RWMutex
	closed       bool
	closeOnce    sync.Once
}

func NewSemanticCacheService(config SemanticCacheConfig, repo controlstate.SemanticCacheRepository, vector storage.VectorAdapter, adapter providers.EmbedAdapter) *SemanticCacheService {
	profiles := make([]SemanticCacheUseCase, len(config.UseCases))
	for index, profile := range config.UseCases {
		profiles[index] = profile
		profiles[index].APIKeyIDs = append([]string(nil), profile.APIKeyIDs...)
	}
	config.UseCases = profiles
	service := &SemanticCacheService{
		config:  config,
		repo:    repo,
		vector:  vector,
		adapter: adapter,
	}
	if config.Enabled && config.ReadConcurrency > 0 {
		service.reads = make(chan struct{}, config.ReadConcurrency)
	}
	if config.Enabled && config.WriteWorkers > 0 && config.QueueCapacity > 0 && config.WriteTimeout > 0 {
		service.startWorkers()
	}
	return service
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
	identity := strings.Join([]string{identityID, useCase.UseCaseID, useCase.KnowledgeVersion, useCase.TargetModel, useCase.SystemPrompt, config.EmbeddingProvider, config.EmbeddingModel, fmt.Sprint(config.VectorDimension), "temperature=0", "max_tokens=256", "embedding-input=user-text-v1"}, "\x00")
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
	if !s.config.Enabled {
		return nil, nil
	}
	if s.reads == nil || s.config.ReadTimeout <= 0 {
		return s.lookup(ctx, scope, model, text)
	}
	select {
	case s.reads <- struct{}{}:
	default:
		return nil, s.fault("lookup", "concurrency_full", nil)
	}
	readCtx, cancel := context.WithTimeout(ctx, s.config.ReadTimeout)
	defer cancel()
	type result struct {
		entry *controlstate.SemanticCacheEntry
		err   error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-s.reads }()
		entry, err := s.lookup(readCtx, scope, model, text)
		done <- result{entry, err}
	}()
	select {
	case <-readCtx.Done():
		return nil, s.fault("lookup", "timeout", readCtx.Err())
	case result := <-done:
		if readCtx.Err() != nil {
			return nil, s.fault("lookup", "timeout", readCtx.Err())
		}
		return result.entry, result.err
	}
}

func (s *SemanticCacheService) lookup(ctx context.Context, scope, model string, text string) (*controlstate.SemanticCacheEntry, error) {
	if !s.config.Enabled || s.repo == nil || s.adapter == nil {
		return nil, nil // Miss
	}

	inputVector, err := s.embed(ctx, text, "lookup")
	if err != nil {
		return nil, err
	}

	// 2. If vector adapter is configured, use it for search
	if s.vector != nil {
		started := time.Now()
		results, err := s.vector.Search(ctx, vectorCollection(scope, model), inputVector, s.config.MaxCandidates)
		measureOperation("vector_search", started, err)
		if err != nil {
			return nil, s.fault("lookup", "vector_error", err)
		}
		entry, err := s.lookupVectorResult(ctx, scope, model, results)
		if err != nil || entry != nil {
			return entry, err
		}
		if _, offlineFallback := s.vector.(*storage.NoopVectorAdapter); !offlineFallback {
			recordCacheOutcome("lookup", "miss")
			return nil, nil
		}
	}

	return s.lookupRepository(ctx, CacheWrite{Scope: scope, Model: model}, inputVector)
}

func (s *SemanticCacheService) lookupRepository(ctx context.Context, identity CacheWrite, inputVector []float32) (*controlstate.SemanticCacheEntry, error) {
	started := time.Now()
	scope, model := identity.Scope, identity.Model
	candidates, err := s.repo.ListCandidates(ctx, scope, model)
	measureOperation("repo_read", started, err)
	if err != nil {
		return nil, s.fault("lookup", "repository_error", err)
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
		if !s.validEntry(cand, identity) {
			continue
		}
		candVector := bytesToFloats(cand.Vector)
		score := cosineSimilarity(inputVector, candVector)
		if score > bestScore {
			bestScore = score
			bestMatch = cand
		}
	}

	if bestScore >= s.config.Threshold && bestMatch != nil && validCachedChoices(bestMatch.Response) {
		if err := s.repo.RecordHit(ctx, bestMatch.ID); err != nil {
			return nil, s.fault("lookup", "repository_error", err)
		}
		recordCacheOutcome("lookup", "hit")
		return bestMatch, nil
	}
	recordCacheOutcome("lookup", "miss")
	return nil, nil
}

func (s *SemanticCacheService) Store(ctx context.Context, id, scope, model string, text string, response string, usageID *string) (storeErr error) {
	if !s.config.Enabled || s.repo == nil || s.adapter == nil {
		return nil
	}
	storeStarted := time.Now()
	defer func() { measureOperation("store_total", storeStarted, storeErr) }()

	vector, err := s.embed(ctx, text, "store")
	if err != nil {
		return err
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

	return s.persist(ctx, entry, vector)
}

func (s *SemanticCacheService) persist(ctx context.Context, entry *controlstate.SemanticCacheEntry, vector []float32) error {
	started := time.Now()
	err := s.repo.Store(ctx, entry)
	measureOperation("repo_write", started, err)
	if err != nil {
		return s.fault("store", "repository_error", err)
	}
	if s.vector != nil {
		meta := map[string]interface{}{
			"id": entry.ID,
		}
		if entry.UsageID != nil {
			meta["usage_id"] = *entry.UsageID
		}
		started = time.Now()
		err = s.vector.Insert(ctx, vectorCollection(entry.Scope, entry.Model), [][]float32{vector}, []map[string]interface{}{meta})
		measureOperation("vector_insert", started, err)
		if err != nil {
			return s.fault("store", "vector_error", err)
		}
	}
	return nil
}

func (s *SemanticCacheService) lookupVectorResult(ctx context.Context, scope, model string, results []map[string]interface{}) (*controlstate.SemanticCacheEntry, error) {
	for _, result := range results {
		score, hasScore := result["score"].(float64)
		if !hasScore || math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, s.fault("lookup", "invalid_entry", nil)
		}
		if float32(score) < s.config.Threshold {
			continue
		}
		id, _ := result["id"].(string)
		if id == "" {
			return nil, s.fault("lookup", "invalid_entry", nil)
		}
		started := time.Now()
		entry, err := s.repo.GetCandidate(ctx, id, scope, model)
		measureOperation("repo_read", started, err)
		if err != nil {
			return nil, s.fault("lookup", "repository_error", err)
		}
		if s.validEntry(entry, CacheWrite{Scope: scope, Model: model}) {
			if err := s.repo.RecordHit(ctx, entry.ID); err != nil {
				return nil, s.fault("lookup", "repository_error", err)
			}
			recordCacheOutcome("lookup", "hit")
			return entry, nil
		}
	}
	return nil, nil
}

func (s *SemanticCacheService) embed(ctx context.Context, text, operation string) ([]float32, error) {
	started := time.Now()
	response, err := s.adapter.Embed(ctx, &llm.EmbeddingRequest{Model: s.config.EmbeddingModel, Input: []string{text}})
	measureOperation(operation+"_embedding", started, err)
	if err != nil || response == nil || len(response.Data) != 1 {
		return nil, s.fault(operation, "embedding_error", err)
	}
	vector := response.Data[0].Embedding
	if !validVector(vector, s.config.VectorDimension) {
		return nil, s.fault(operation, "invalid_embedding", nil)
	}
	return vector, nil
}

func (s *SemanticCacheService) validEntry(entry *controlstate.SemanticCacheEntry, identity CacheWrite) bool {
	return entry != nil && entry.Scope == identity.Scope && entry.Model == identity.Model && entry.Enabled && entry.ExpiresAt.After(time.Now()) && validVector(bytesToFloats(entry.Vector), s.config.VectorDimension) && validCachedChoices(entry.Response)
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
	return fmt.Sprintf("semantic_cache_%s", hex.EncodeToString(digest[:]))
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
