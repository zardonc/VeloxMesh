//go:build phase29preflight

package integration

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
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/cache"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	controlsqlite "veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
	router "veloxmesh/internal/http"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/pipeline"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/openai"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/storage"
)

const (
	phase29Concurrency = 4
	phase29Interval    = time.Second
	phase29System      = "Static FAQ for this test: hours 09:00-17:00 UTC weekdays; email help@example.invalid; replies within one business day; billing monthly; invoices on day 1; refunds within 14 days; trials last 7 days; data retained 30 days; exports JSON; backups daily; region EU; SSO via SAML; MFA required; password reset by email; support English; maintenance Sunday 02:00 UTC; status updates hourly; cancellation anytime; plan changes next billing cycle; API rate limit 100/minute. Answer only from these facts in one sentence."
)

var phase29Questions = []string{
	"What are support hours?", "What is the support email?", "When will support reply?",
	"How often are customers billed?", "When are invoices issued?", "What is the refund window?",
	"How long is a trial?", "How long is data retained?", "What export format is available?",
	"How frequently are backups taken?", "Which data region is used?", "Which SSO protocol is supported?",
	"Is multi-factor authentication required?", "How do I reset a password?", "Which support language is available?",
	"When is scheduled maintenance?", "How often are status updates posted?", "When can I cancel?",
	"When do plan changes take effect?", "What is the API rate limit?", "What are support hours?",
}

type phase29Sample struct {
	Type      string  `json:"type"`
	Name      string  `json:"name,omitempty"`
	Index     int     `json:"index,omitempty"`
	AtMS      int64   `json:"at_ms"`
	ElapsedMS float64 `json:"elapsed_ms"`
	OK        bool    `json:"ok"`
	Status    int     `json:"status,omitempty"`
	CacheHit  string  `json:"cache_hit,omitempty"`
	Bytes     int     `json:"bytes,omitempty"`
	Error     string  `json:"error,omitempty"`
}

type phase29Recorder struct {
	mu      sync.Mutex
	started time.Time
	samples []phase29Sample
}

func (r *phase29Recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = time.Now()
	r.samples = nil
}

func (r *phase29Recorder) add(sample phase29Sample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sample.AtMS = time.Since(r.started).Milliseconds()
	r.samples = append(r.samples, sample)
}

func (r *phase29Recorder) operation(name string, started time.Time, err error) {
	sample := phase29Sample{Type: "operation", Name: name, ElapsedMS: float64(time.Since(started).Microseconds()) / 1000, OK: err == nil}
	if err != nil {
		sample.Error = err.Error()
	}
	r.add(sample)
}

type phase29EmbedMeter struct {
	providers.EmbedAdapter
	recorder *phase29Recorder
}

func (m *phase29EmbedMeter) Embed(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	started := time.Now()
	result, err := m.EmbedAdapter.Embed(ctx, req)
	m.recorder.operation("embedding", started, err)
	return result, err
}

type phase29PrimaryMeter struct {
	providers.ProviderAdapter
	recorder *phase29Recorder
}

func (m *phase29PrimaryMeter) Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error) {
	started := time.Now()
	result, err := m.ProviderAdapter.Complete(ctx, req)
	m.recorder.operation("primary", started, err)
	return result, err
}

type phase29VectorMeter struct {
	storage.VectorAdapter
	recorder *phase29Recorder
}

func (m *phase29VectorMeter) Search(ctx context.Context, collection string, query []float32, limit int) ([]map[string]interface{}, error) {
	started := time.Now()
	result, err := m.VectorAdapter.Search(ctx, collection, query, limit)
	m.recorder.operation("vector_search", started, err)
	return result, err
}

func (m *phase29VectorMeter) Insert(ctx context.Context, collection string, vectors [][]float32, metadata []map[string]interface{}) error {
	started := time.Now()
	err := m.VectorAdapter.Insert(ctx, collection, vectors, metadata)
	m.recorder.operation("vector_insert", started, err)
	return err
}

type phase29RepoMeter struct {
	controlstate.SemanticCacheRepository
	recorder *phase29Recorder
}

func (m *phase29RepoMeter) Store(ctx context.Context, entry *controlstate.SemanticCacheEntry) error {
	started := time.Now()
	err := m.SemanticCacheRepository.Store(ctx, entry)
	m.recorder.operation("repo_write", started, err)
	return err
}

func (m *phase29RepoMeter) ListCandidates(ctx context.Context, scope, model string) ([]*controlstate.SemanticCacheEntry, error) {
	started := time.Now()
	result, err := m.SemanticCacheRepository.ListCandidates(ctx, scope, model)
	m.recorder.operation("repo_read", started, err)
	return result, err
}

func (m *phase29RepoMeter) GetCandidate(ctx context.Context, id, scope, model string) (*controlstate.SemanticCacheEntry, error) {
	started := time.Now()
	result, err := m.SemanticCacheRepository.GetCandidate(ctx, id, scope, model)
	m.recorder.operation("repo_read", started, err)
	return result, err
}

type phase29Rig struct {
	server   *httptest.Server
	repo     controlstate.Repository
	token    string
	model    string
	interval time.Duration
	recorder *phase29Recorder
}

type phase29RunInfo struct {
	mode     string
	run      string
	elapsed  time.Duration
	interval time.Duration
}

func newPhase29Rig(t *testing.T, enabled bool, env map[string]string) *phase29Rig {
	t.Helper()
	ctx := context.Background()
	dsn := fmt.Sprintf("file:phase29-preflight-%d?mode=memory&cache=shared", time.Now().UnixNano())
	repo, err := controlsqlite.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("phase29-measure-token-%d", time.Now().UnixNano())
	digest := sha256.Sum256([]byte(token))
	keyID := fmt.Sprintf("phase29-measure-key-%d", time.Now().UnixNano())
	if err := repo.APIKeys().Create(ctx, &controlstate.APIKeyRecord{
		ID: keyID, Hash: hex.EncodeToString(digest[:]), Name: "phase29 isolated measurement", Role: "user", Enabled: true, CreditBalance: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	vector, err := storage.NewQdrantVectorAdapter(os.Getenv("PHASE29_QDRANT_ADDR"), os.Getenv("PHASE29_QDRANT_API_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := &phase29Recorder{started: time.Now()}
	primary := &phase29PrimaryMeter{ProviderAdapter: openai.NewAdapter("phase29-primary", env["SANS_BASE_URL"], env["SANS_PRIMARY_API_KEY"], env["SANS_PRIMARY_DEFAULT_MODEL"]), recorder: recorder}
	embed := &phase29EmbedMeter{EmbedAdapter: newPhase29GeminiAdapter(env), recorder: recorder}
	probe, err := embed.Embed(ctx, &llm.EmbeddingRequest{Model: env["GEM_EMBEDDING"], Input: []string{"phase29 vector dimension probe"}})
	if err != nil || probe == nil || len(probe.Data) != 1 || len(probe.Data[0].Embedding) == 0 {
		t.Fatalf("embedding dimension probe: %v", err)
	}
	semantic := cache.NewSemanticCacheService(cache.SemanticCacheConfig{
		Enabled: enabled, Threshold: 0.99999, MaxCandidates: 10, TTL: time.Hour,
		EmbeddingModel: env["GEM_EMBEDDING"], VectorDimension: len(probe.Data[0].Embedding),
		UseCases: []cache.SemanticCacheUseCase{{
			APIKeyIDs: []string{keyID}, UseCaseID: "phase29-static-faq", KnowledgeVersion: "faq-v1",
			TargetModel: env["SANS_PRIMARY_DEFAULT_MODEL"], SystemPrompt: phase29System,
		}},
	}, &phase29RepoMeter{SemanticCacheRepository: repo.SemanticCache(), recorder: recorder},
		&phase29VectorMeter{VectorAdapter: vector, recorder: recorder}, embed)
	store := health.NewInMemoryStore()
	store.EnsureProvider("phase29-primary", 3, 1)
	cfg := &config.Config{}
	registry := providers.NewRegistry(cfg, []providers.ProviderAdapter{primary}, nil)
	route := routing.NewHealthAwareRouter(registry, store, "round-robin", nil)
	service := gateway.NewService(route, admission.NewPassThroughController(), store, true, 2, repo, semantic, pipeline.DefaultRegistry(), nil, nil)
	server := httptest.NewServer(router.NewRouter(cfg, service, nil, nil, nil, nil, nil, repo, nil, nil))
	t.Cleanup(server.Close)
	return &phase29Rig{server: server, repo: repo, token: token, model: env["SANS_PRIMARY_DEFAULT_MODEL"], interval: phase29Interval, recorder: recorder}
}

func (r *phase29Rig) request(ctx context.Context, client *http.Client, index int, question string) phase29Sample {
	temperature, maxTokens := 0.0, 256
	body, err := json.Marshal(llm.ChatCompletionRequest{Model: r.model, Temperature: &temperature, MaxTokens: &maxTokens,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: phase29System}, {Role: llm.RoleUser, Content: question}}})
	if err != nil {
		return phase29Sample{Type: "client", Index: index, Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.server.URL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return phase29Sample{Type: "client", Index: index, Error: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return phase29Sample{Type: "client", Index: index, ElapsedMS: float64(time.Since(started).Microseconds()) / 1000, Error: err.Error()}
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	sample := phase29Sample{Type: "client", Index: index, ElapsedMS: float64(time.Since(started).Microseconds()) / 1000,
		Status: resp.StatusCode, CacheHit: resp.Header.Get("X-Cache-Hit"), Bytes: len(content), OK: err == nil && resp.StatusCode == http.StatusOK}
	if err != nil {
		sample.Error = err.Error()
	}
	if resp.StatusCode != http.StatusOK {
		sample.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return sample
}

func TestPhase29Measure(t *testing.T) {
	mode, run, output := os.Getenv("PHASE29_MODE"), os.Getenv("PHASE29_RUN"), os.Getenv("PHASE29_OUTPUT")
	if mode == "" || run == "" || output == "" {
		t.Skip("set PHASE29_MODE, PHASE29_RUN and PHASE29_OUTPUT")
	}
	if mode != "off" && mode != "low_hit" {
		t.Fatal("PHASE29_MODE must be off or low_hit")
	}
	if os.Getenv("PHASE29_QDRANT_API_KEY") == "" {
		t.Fatal("PHASE29_QDRANT_API_KEY is required for authenticated Qdrant checks")
	}
	if os.Getenv("PHASE29_QDRANT_ADDR") == "" {
		t.Fatal("PHASE29_QDRANT_ADDR is required")
	}
	env, err := godotenv.Read("../../.env.local")
	if err != nil {
		t.Fatal(err)
	}
	rig := newPhase29Rig(t, mode == "low_hit", env)
	if value := os.Getenv("PHASE29_INTERVAL_MS"); value != "" {
		milliseconds, err := strconv.Atoi(value)
		if err != nil || milliseconds < 1 {
			t.Fatal("PHASE29_INTERVAL_MS must be a positive integer")
		}
		rig.interval = time.Duration(milliseconds) * time.Millisecond
	}
	client := &http.Client{Timeout: 25 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 52*time.Second)
	defer cancel()
	warmup := rig.request(ctx, client, -1, "Warmup sentinel outside the fixed FAQ set")
	if !warmup.OK {
		t.Fatalf("warmup failed: status=%d error=%s", warmup.Status, warmup.Error)
	}
	for _, sample := range rig.recorder.samples {
		if (sample.Name == "repo_write" || sample.Name == "vector_insert") && !sample.OK {
			t.Fatalf("warmup %s failed: %s", sample.Name, sample.Error)
		}
	}
	rig.recorder.reset()
	started := time.Now()
	runPhase29Requests(ctx, rig, client)
	elapsed := time.Since(started)
	if err := writePhase29Samples(output, phase29RunInfo{mode: mode, run: run, elapsed: elapsed, interval: rig.interval}, rig.recorder); err != nil {
		t.Fatal(err)
	}
	t.Logf("mode=%s run=%s samples=%d elapsed_ms=%d raw=%s", mode, run, len(phase29Questions), elapsed.Milliseconds(), output)
	clientCount := 0
	for _, sample := range rig.recorder.samples {
		if sample.Type == "operation" && !sample.OK {
			t.Errorf("operation %s failed: %s", sample.Name, sample.Error)
		}
		if sample.Type != "client" {
			continue
		}
		clientCount++
		if !sample.OK {
			t.Errorf("client %d: status=%d error=%s", sample.Index, sample.Status, sample.Error)
		}
	}
	if clientCount != len(phase29Questions) {
		t.Errorf("client samples=%d want=%d", clientCount, len(phase29Questions))
	}
}

func runPhase29Requests(ctx context.Context, rig *phase29Rig, client *http.Client) {
	limit := make(chan struct{}, phase29Concurrency)
	var group sync.WaitGroup
	defer group.Wait()
schedule:
	for index, question := range phase29Questions {
		if index > 0 {
			timer := time.NewTimer(rig.interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				break schedule
			case <-timer.C:
			}
		}
		select {
		case <-ctx.Done():
			break schedule
		case limit <- struct{}{}:
		}
		group.Add(1)
		go func(index int, question string) {
			defer group.Done()
			defer func() { <-limit }()
			rig.recorder.add(rig.request(ctx, client, index, question))
		}(index, question)
	}
}

func writePhase29Samples(path string, info phase29RunInfo, recorder *phase29Recorder) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(map[string]any{"type": "metadata", "mode": info.mode, "run": info.run, "requests": len(phase29Questions),
		"target_rps": float64(time.Second) / float64(info.interval), "max_concurrency": phase29Concurrency, "elapsed_ms": info.elapsed.Milliseconds(),
		"client_timing": "through_body_eof", "test_only_collection_name_mapping": "colon_to_underscore"}); err != nil {
		return err
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for _, sample := range recorder.samples {
		if err := encoder.Encode(sample); err != nil {
			return err
		}
	}
	return nil
}
