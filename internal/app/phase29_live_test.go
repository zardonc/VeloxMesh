//go:build phase29preflight

package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/status"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers/openai"
)

const liveFAQSystem = "Test static FAQ: each numbered plan has a trial lasting its plan number in days. Answer the requested plan's trial length in one short sentence."

type liveSample struct {
	Type       string  `json:"type"`
	Name       string  `json:"name,omitempty"`
	ElapsedMS  float64 `json:"elapsed_ms"`
	StartMS    float64 `json:"start_ms"`
	OK         bool    `json:"ok"`
	Status     int     `json:"status,omitempty"`
	Hit        bool    `json:"hit,omitempty"`
	Concurrent int64   `json:"concurrent,omitempty"`
	AnswerHash string  `json:"answer_hash,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type liveRecorder struct {
	*observability.StubMetrics
	mu      sync.Mutex
	started time.Time
	samples []liveSample
}

func (r *liveRecorder) RecordCacheOperation(name string, elapsed time.Duration, err error) {
	category := ""
	if err != nil {
		category = status.Code(err).String()
	}
	r.add(liveSample{Type: "operation", Name: name, ElapsedMS: float64(elapsed.Microseconds()) / 1000, StartMS: float64(time.Since(r.started).Microseconds())/1000 - float64(elapsed.Microseconds())/1000, OK: err == nil, Error: category})
}

func (r *liveRecorder) RecordCacheOutcome(operation, reason string) {
	r.add(liveSample{Type: "outcome", Name: operation + "/" + reason, StartMS: float64(time.Since(r.started).Microseconds()) / 1000, OK: true})
}

func (r *liveRecorder) add(sample liveSample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, sample)
}

func liveJSON(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func liveEnvironment(t *testing.T) map[string]string {
	t.Helper()
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("real components require explicit opt-in")
	}
	var env map[string]string
	if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
		t.Fatal("secret input unavailable")
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	return env
}

func liveRepository(t *testing.T) (*sqlite.Repository, string, string) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "control.db")
	repo, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	ctx := context.Background()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("test-%d", time.Now().UnixNano())
	digest := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(digest[:])
	key := &controlstate.APIKeyRecord{ID: token, Hash: hash, Name: "isolated FAQ measurement", Role: "user", Enabled: true, CreditBalance: 1000000}
	if err := repo.APIKeys().Create(ctx, key); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.APIKeys().GetByHash(ctx, hash)
	if err != nil || loaded == nil {
		t.Fatal("cannot read test key from repository")
	}
	return repo, dsn, loaded.ID
}

func liveApplication(t *testing.T, env map[string]string) (*App, string) {
	t.Helper()
	_, dsn, keyID := liveRepository(t)
	return liveApplicationWithDatabase(t, env, liveDatabase{dsn: dsn, keyID: keyID})
}

type liveDatabase struct{ dsn, keyID string }

func liveApplicationWithDatabase(t *testing.T, env map[string]string, database liveDatabase) (*App, string) {
	t.Helper()
	model := os.Getenv("PHASE29_MODEL")
	embeddingProvider := "phase29-embedding"
	embeddingURL := os.Getenv("PHASE29_EMBEDDING_BASE_URL")
	embeddingKey := env["PHASE29_EMBEDDING_API_KEY"]
	if embeddingURL == "" {
		embeddingURL = env["SANS_BASE_URL"]
		embeddingKey = env["SANS_PRIMARY_API_KEY"]
		t.Setenv("PHASE29_EMBEDDING_API_KEY", embeddingKey)
	}
	adapter := openai.NewAdapter(embeddingProvider, embeddingURL, embeddingKey, model)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe, err := adapter.Embed(ctx, &llm.EmbeddingRequest{Model: model, Input: []string{"dimension probe"}})
	if err != nil || probe == nil || len(probe.Data) != 1 || len(probe.Data[0].Embedding) == 0 {
		t.Fatalf("dimension probe: %v", err)
	}
	providers := liveProviders(t, env, config.ProviderConfig{ID: embeddingProvider, Type: "openai-compatible", BaseURL: embeddingURL,
		Auth: &config.ProviderAuthConfig{APIKeyEnv: "PHASE29_EMBEDDING_API_KEY"}, Models: []string{model}})
	t.Setenv("CONTROL_STATE_BACKEND", "sqlite")
	t.Setenv("CONTROL_STATE_DSN", database.dsn)
	t.Setenv("CONTROL_STATE_MIGRATE_ON_STARTUP", "true")
	t.Setenv("CONTROL_STATE_LOCAL_SEED_ENABLED", "true")
	t.Setenv("CONTROL_STATE_ENCRYPTION_KEY", livePostgresTestEncryptionKey)
	t.Setenv("REDIS_ENABLED", "true")
	t.Setenv("REDIS_ADDR", liveRedisAddress())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("SCHEDULER_ENABLED", "false")
	t.Setenv("REDIS_NAMESPACE", "phase29-"+database.keyID)
	cacheConfig := liveCacheConfig(t, env, liveCacheInputs{database: database, dimension: len(probe.Data[0].Embedding)})
	cacheConfig = liveExpandCacheUseCases(cacheConfig, providers[2:])
	var protection map[string]config.ProviderProtectionConfig
	if value := os.Getenv("PHASE29_PROVIDER_PROTECTION"); value != "" {
		if err := json.Unmarshal([]byte(value), &protection); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CONFIG_FILE", liveJSON(t, map[string]any{"providers": providers, "default_provider": "sans-primary", "cache": cacheConfig, "provider_protection": protection}))
	application, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
	if cacheConfig.Enabled && cacheConfig.HasAnswerReuse() && application.semanticCache == nil {
		t.Fatal("real cache wiring unavailable")
	}
	t.Logf("model=%s dimension=%d primary=%s", model, cacheConfig.VectorDimension, env["SANS_PRIMARY_DEFAULT_MODEL"])
	return application, database.keyID
}

func liveRedisAddress() string {
	if address := os.Getenv("PHASE29_REDIS_ADDR"); address != "" {
		return address
	}
	return "127.0.0.1:6379"
}

func liveProviders(t *testing.T, env map[string]string, embedding config.ProviderConfig) []config.ProviderConfig {
	t.Helper()
	primaryType := env["SHIP_PROVIDER_TYPE"]
	if primaryType == "" {
		primaryType = "openai-compatible"
	}
	providers := []config.ProviderConfig{{ID: "sans-primary", Type: primaryType, BaseURL: env["SANS_BASE_URL"],
		Auth: &config.ProviderAuthConfig{APIKeyEnv: "SANS_PRIMARY_API_KEY"}, Models: []string{env["SANS_PRIMARY_DEFAULT_MODEL"]}}, embedding}
	if extra := env["PHASE29_EXTRA_PROVIDERS"]; extra != "" {
		var configured []config.ProviderConfig
		if err := json.Unmarshal([]byte(extra), &configured); err != nil {
			t.Fatal(err)
		}
		return append(providers, configured...)
	}
	return providers
}

func liveExpandCacheUseCases(original config.CacheConfig, providers []config.ProviderConfig) config.CacheConfig {
	result := original
	result.UseCases = append([]config.CacheUseCaseConfig(nil), original.UseCases...)
	seen := make(map[string]bool)
	for _, profile := range original.UseCases {
		seen[profile.TargetModel] = true
	}
	for _, provider := range providers {
		for _, model := range provider.Models {
			if seen[model] {
				continue
			}
			useCase := original.UseCases[0]
			useCase.TargetModel = model
			result.UseCases = append(result.UseCases, useCase)
			seen[model] = true
		}
	}
	return result
}

type liveCacheInputs struct {
	database  liveDatabase
	dimension int
}

func liveCacheConfig(t *testing.T, env map[string]string, inputs liveCacheInputs) config.CacheConfig {
	t.Helper()
	version, system := os.Getenv("PHASE29_KNOWLEDGE_VERSION"), os.Getenv("PHASE29_SYSTEM")
	if version == "" {
		version = "faq-v1"
	}
	if system == "" {
		system = liveFAQSystem
	}
	mode := os.Getenv("PHASE29_REUSE_MODE")
	if mode == "" {
		mode = config.CacheReuseSemantic // Explicit opt-in for these isolated real-model probes.
	} else if mode == "implicit" {
		mode = ""
	}
	cacheConfig := config.CacheConfig{Enabled: os.Getenv("PHASE29_MODE") != "off", Provider: "phase29-embedding", EmbeddingModel: os.Getenv("PHASE29_MODEL"), VectorStore: "qdrant", VectorDimension: inputs.dimension, TTL: "1h", Threshold: 0.99999, MaxCandidates: 10, Qdrant: config.QdrantConfig{Addr: "127.0.0.1:6334", APIKey: os.Getenv("QDRANT_API_KEY")}, UseCases: []config.CacheUseCaseConfig{{ReuseMode: mode, APIKeyIDs: []string{inputs.database.keyID}, UseCaseID: "phase29-static-faq", KnowledgeVersion: version, TargetModel: env["SANS_PRIMARY_DEFAULT_MODEL"], SystemPrompt: system}}}
	if addr := os.Getenv("PHASE29_QDRANT_ADDR"); addr != "" {
		cacheConfig.Qdrant.Addr = addr
	}
	if dimension := liveInteger(t, "PHASE29_VECTOR_DIMENSION"); dimension > 0 {
		cacheConfig.VectorDimension = dimension
	}
	if threshold := os.Getenv("PHASE29_THRESHOLD"); threshold != "" {
		value, err := strconv.ParseFloat(threshold, 32)
		if err != nil {
			t.Fatal(err)
		}
		cacheConfig.Threshold = float32(value)
	}
	cacheConfig.ReadTimeout = os.Getenv("PHASE29_READ_TIMEOUT")
	if ttl := os.Getenv("PHASE29_TTL"); ttl != "" {
		cacheConfig.TTL = ttl
	}
	cacheConfig.ReadConcurrency = liveInteger(t, "PHASE29_READ_CONCURRENCY")
	cacheConfig.WriteTimeout = os.Getenv("PHASE29_WRITE_TIMEOUT")
	cacheConfig.WriteWorkers = liveInteger(t, "PHASE29_WRITE_WORKERS")
	cacheConfig.QueueCapacity = liveInteger(t, "PHASE29_QUEUE_CAPACITY")
	cacheConfig.ShutdownGrace = os.Getenv("PHASE29_SHUTDOWN_GRACE")
	cacheConfig.EmbeddingInputPrefix = os.Getenv("PHASE29_INPUT_PREFIX")
	cacheConfig.EmbeddingMemoCapacity = liveInteger(t, "PHASE29_MEMO_CAPACITY")
	cacheConfig.EmbeddingMemoTTL = os.Getenv("PHASE29_MEMO_TTL")
	return cacheConfig
}

func liveInteger(t *testing.T, name string) int {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("invalid %s", name)
	}
	return number
}

func liveRequest(options liveRequestOptions) liveSample {
	temperature, maxTokens := 0.0, 256
	system, question := options.system, options.question
	if system == "" {
		system = liveFAQSystem
	}
	if question == "" {
		question = fmt.Sprintf("How long is the trial for plan %d?", options.index)
	}
	body, _ := json.Marshal(llm.ChatCompletionRequest{Model: options.model, Temperature: &temperature, MaxTokens: &maxTokens, Messages: []llm.Message{{Role: llm.RoleSystem, Content: system}, {Role: llm.RoleUser, Content: question}}})
	req, _ := http.NewRequest(http.MethodPost, options.url+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+options.token)
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	sample := liveSample{Type: "client", StartMS: float64(time.Since(options.origin).Microseconds()) / 1000, Concurrent: options.concurrent}
	response, err := options.client.Do(req)
	if err == nil {
		var body []byte
		body, err = io.ReadAll(response.Body)
		response.Body.Close()
		sample.Status, sample.Hit = response.StatusCode, response.Header.Get("X-Cache-Hit") == "true"
		var content struct {
			Choices []struct{ Message struct{ Content string } }
		}
		if json.Unmarshal(body, &content) == nil && len(content.Choices) > 0 {
			digest := sha256.Sum256([]byte(content.Choices[0].Message.Content))
			sample.AnswerHash = hex.EncodeToString(digest[:])
		}
	}
	if err != nil {
		sample.Error = err.Error()
	}
	sample.ElapsedMS, sample.OK = float64(time.Since(started).Microseconds())/1000, err == nil && sample.Status == http.StatusOK
	return sample
}

type liveRequestOptions struct {
	client            *http.Client
	url, token, model string
	system, question  string
	index             int
	hitEvery          int
	origin            time.Time
	concurrent        int64
	finish            func()
	direct            bool
	models            []string
	onSamples         func([]shipSample)
}

func TestPhase29LiveMeasure(t *testing.T) {
	if os.Getenv("PHASE29_MODEL") == "" {
		t.Skip("isolated opt-in measurement")
	}
	env := liveEnvironment(t)
	application, token := liveApplication(t, env)
	server := httptest.NewServer(application.Router)
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 10 * time.Second}
	warm := liveRequest(liveRequestOptions{client: client, url: server.URL, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], index: 999, origin: time.Now()})
	if !warm.OK {
		t.Fatalf("warmup HTTP %d", warm.Status)
	}
	recorder := &liveRecorder{StubMetrics: observability.NewStubMetrics(), started: time.Now()}
	previous := observability.DefaultMetrics
	observability.DefaultMetrics = recorder
	defer func() { observability.DefaultMetrics = previous }()
	liveMeasureLoad(t, recorder, liveRequestOptions{client: client, url: server.URL, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], origin: recorder.started, finish: application.Close})
}

func liveMeasureLoad(t *testing.T, recorder *liveRecorder, options liveRequestOptions) {
	t.Helper()
	intervalMS, err := strconv.Atoi(os.Getenv("PHASE29_INTERVAL_MS"))
	if err != nil || intervalMS < 1 {
		t.Fatal("invalid measurement interval")
	}
	count, err := strconv.Atoi(os.Getenv("PHASE29_COUNT"))
	if err != nil || count < 1 {
		t.Fatal("invalid sample count")
	}
	concurrency := liveInteger(t, "PHASE29_CLIENT_CONCURRENCY")
	if concurrency < 1 {
		t.Fatal("invalid client concurrency")
	}
	sem := make(chan struct{}, concurrency)
	var active atomic.Int64
	var group sync.WaitGroup
	for index := 1; index <= count; index++ {
		target := options.origin.Add(time.Duration(index-1) * time.Duration(intervalMS) * time.Millisecond)
		if delay := time.Until(target); delay > 0 {
			time.Sleep(delay)
		}
		sem <- struct{}{}
		group.Add(1)
		go func(index int) {
			defer group.Done()
			defer func() { <-sem; active.Add(-1) }()
			request := options
			request.index, request.concurrent = index, active.Add(1)
			if index%21 == 0 {
				request.index = index - 1
			}
			recorder.add(liveRequest(request))
		}(index)
	}
	group.Wait()
	foregroundElapsed := time.Since(options.origin)
	if options.finish != nil {
		options.finish()
	}
	liveSaveSamples(t, recorder, map[string]any{"type": "metadata", "model": os.Getenv("PHASE29_MODEL"), "mode": os.Getenv("PHASE29_MODE"), "count": count, "interval_ms": intervalMS, "elapsed_ms": float64(foregroundElapsed.Microseconds()) / 1000, "drain_ms": float64((time.Since(options.origin) - foregroundElapsed).Microseconds()) / 1000, "max_concurrency": cap(sem)})
}

func liveSaveSamples(t *testing.T, recorder *liveRecorder, metadata map[string]any) {
	t.Helper()
	if os.Getenv("PHASE29_OUTPUT") == "" {
		shipLogJSON(t, metadata)
		for _, sample := range recorder.samples {
			shipLogJSON(t, sample)
		}
		return
	}
	file, err := os.Create(os.Getenv("PHASE29_OUTPUT"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(metadata); err != nil {
		t.Fatal(err)
	}
	for _, sample := range recorder.samples {
		if err := encoder.Encode(sample); err != nil {
			t.Fatal(err)
		}
	}
}
