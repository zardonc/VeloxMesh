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

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/storage"
)

type SemanticCacheConfig struct {
	Enabled               bool
	Threshold             float32
	MaxCandidates         int
	TTL                   time.Duration
	EmbeddingModel        string
	EmbeddingInputPrefix  string
	EmbeddingMemoCapacity int
	EmbeddingMemoTTL      time.Duration
	EmbeddingProvider     string
	VectorDimension       int
	UseCases              []SemanticCacheUseCase
	ReadTimeout           time.Duration
	ReadConcurrency       int
	WriteTimeout          time.Duration
	WriteWorkers          int
	QueueCapacity         int
	ShutdownGrace         time.Duration
}

type SemanticCacheUseCase struct {
	ReuseMode        string
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
	embeddings   *embeddingMemo
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
		config:     config,
		repo:       repo,
		vector:     vector,
		adapter:    adapter,
		embeddings: newEmbeddingMemo(config),
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
		if config.EffectiveCacheReuseMode(useCase.ReuseMode) == config.CacheReuseDisabled {
			continue
		}
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
	if config.EmbeddingInputPrefix != "" {
		identity += "\x00embedding-input=prefix-user-text-v2\x00" + config.EmbeddingInputPrefix
	}
	identity += "\x00cache-reuse=" + useCase.ReuseMode + "-v1"
	digest := sha256.Sum256([]byte(identity))
	scope := hex.EncodeToString(digest[:])
	if useCase.ReuseMode == "exact" {
		return exactScopePrefix + scope
	}
	return scope
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
	result, err := s.LookupWithVector(ctx, CacheLookup{Scope: scope, Model: model, Text: text})
	return result.Entry, err
}

type CacheLookup struct {
	Scope, Model, Text string
}

type CacheLookupResult struct {
	Entry  *controlstate.SemanticCacheEntry
	Vector []float32
}

// LookupWithVector retains a completed embedding, including on a search fault.
// An embedding deadline or admission failure returns no vector. A later search
// deadline can still reuse a vector published before that deadline.
func (s *SemanticCacheService) LookupWithVector(ctx context.Context, query CacheLookup) (CacheLookupResult, error) {
	defer observability.Stage(ctx, "cache_read")()
	if !s.config.Enabled {
		return CacheLookupResult{}, nil
	}
	if s.reads == nil || s.config.ReadTimeout <= 0 {
		return s.lookup(ctx, query, nil)
	}
	select {
	case s.reads <- struct{}{}:
	default:
		return CacheLookupResult{}, s.fault("lookup", "concurrency_full", nil)
	}
	readCtx, cancel := context.WithTimeout(ctx, s.config.ReadTimeout)
	defer cancel()
	type result struct {
		lookup CacheLookupResult
		err    error
	}
	done := make(chan result, 1)
	vectors := make(chan []float32, 1)
	go func() {
		defer func() { <-s.reads }()
		lookup, err := s.lookup(readCtx, query, vectors)
		done <- result{lookup, err}
	}()
	select {
	case <-readCtx.Done():
		return s.readTimeout(readCtx, vectors)
	case result := <-done:
		if readCtx.Err() != nil {
			return s.readTimeout(readCtx, vectors)
		}
		return result.lookup, result.err
	}
}

func (s *SemanticCacheService) readTimeout(ctx context.Context, vectors <-chan []float32) (CacheLookupResult, error) {
	var vector []float32
	select {
	case vector = <-vectors:
	default:
	}
	return CacheLookupResult{Vector: vector}, s.fault("lookup", "timeout", ctx.Err())
}

func (s *SemanticCacheService) lookup(ctx context.Context, query CacheLookup, vectors chan<- []float32) (CacheLookupResult, error) {
	if isExactScope(query.Scope) {
		entry, err := s.lookupExact(ctx, query)
		return CacheLookupResult{Entry: entry}, err
	}
	if s.repo == nil || s.adapter == nil {
		return CacheLookupResult{}, nil
	}
	if ready, err := s.scopeReady(ctx, query); err != nil || !ready {
		return CacheLookupResult{}, err
	}

	inputVector, err := s.embed(ctx, query, "lookup")
	if err != nil {
		return CacheLookupResult{}, err
	}
	result := CacheLookupResult{Vector: inputVector}
	if vectors != nil {
		vectors <- inputVector
	}

	if vectorResult, complete, err := s.searchVector(ctx, query, result); complete {
		return vectorResult, err
	}

	entry, err := s.lookupRepository(ctx, CacheWrite{Scope: query.Scope, Model: query.Model}, inputVector)
	return CacheLookupResult{Entry: entry, Vector: inputVector}, err
}

func (s *SemanticCacheService) lookupRepository(ctx context.Context, identity CacheWrite, inputVector []float32) (*controlstate.SemanticCacheEntry, error) {
	defer observability.Stage(ctx, "repo_read")()
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

func (s *SemanticCacheService) Store(ctx context.Context, id, scope, model string, text string, response string, usageID *string) error {
	return s.StoreWrite(ctx, CacheWrite{ID: id, Scope: scope, Model: model, Text: text, Response: response, UsageID: usageID})
}

func (s *SemanticCacheService) StoreWrite(ctx context.Context, write CacheWrite) (storeErr error) {
	defer observability.Stage(ctx, "store_total")()
	if !s.config.Enabled || s.repo == nil {
		return nil
	}
	storeStarted := time.Now()
	defer func() { measureOperation("store_total", storeStarted, storeErr) }()
	if isExactScope(write.Scope) {
		return s.storeExact(ctx, write)
	}
	if s.adapter == nil {
		return nil
	}

	vector, err := s.writeVector(ctx, write)
	if err != nil {
		return err
	}

	entry := &controlstate.SemanticCacheEntry{
		ID:        write.ID,
		Scope:     write.Scope,
		Model:     write.Model,
		Vector:    floatsToBytes(vector),
		Response:  write.Response,
		UsageID:   write.UsageID,
		HitCount:  0,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(s.config.TTL).UTC(),
	}

	return s.persist(ctx, entry, vector)
}

func (s *SemanticCacheService) writeVector(ctx context.Context, write CacheWrite) ([]float32, error) {
	if write.Vector == nil {
		return s.embed(ctx, CacheLookup{Scope: write.Scope, Model: write.Model, Text: write.Text}, "store")
	}
	if !validVector(write.Vector, s.config.VectorDimension) {
		return nil, s.fault("store", "invalid_embedding", nil)
	}
	return write.Vector, nil
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
		finish := observability.Stage(ctx, "repo_read")
		entry, err := s.repo.GetCandidate(ctx, id, scope, model)
		finish()
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

func (s *SemanticCacheService) embed(ctx context.Context, query CacheLookup, operation string) ([]float32, error) {
	defer observability.Stage(ctx, operation+"_embedding")()
	started := time.Now()
	if err := ctx.Err(); err != nil {
		return nil, s.fault(operation, "embedding_error", err)
	}
	if vector, ok := s.embeddings.get(query); ok {
		recordCacheOutcome("embedding_memo", "hit")
		measureOperation(operation+"_embedding", started, nil)
		return vector, nil
	}
	if s.embeddings != nil {
		recordCacheOutcome("embedding_memo", "miss")
	}
	response, err := s.adapter.Embed(ctx, &llm.EmbeddingRequest{Model: s.config.EmbeddingModel, Input: []string{s.config.EmbeddingInputPrefix + query.Text}})
	measureOperation(operation+"_embedding", started, err)
	if err != nil || response == nil || len(response.Data) != 1 {
		return nil, s.fault(operation, "embedding_error", err)
	}
	vector := response.Data[0].Embedding
	if !validVector(vector, s.config.VectorDimension) {
		return nil, s.fault(operation, "invalid_embedding", nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, s.fault(operation, "embedding_error", err)
	}
	s.embeddings.put(query, vector)
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
