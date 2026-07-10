package scheduler

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/providers/openai"
)

func TestSemanticNeighborEnricherUsesTenantScope(t *testing.T) {
	service := semanticNeighborTestService(3, semanticNeighborSamples())
	feature := semanticNeighborFeature()

	got, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborRequest(), feature)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.CoverageLevel != SemanticCoverageTenant || got.NeighborCount != 3 || got.CoverageRatio != 1 {
		t.Fatalf("unexpected coverage: %#v", got)
	}
	if got.LatencyP50Ms != 200 || got.LatencyP90Ms != 300 || got.OutputTokensP70 != 90 {
		t.Fatalf("unexpected percentiles: %#v", got)
	}
	if math.Abs(got.LatencyStddevMs-81.65) > 0.1 {
		t.Fatalf("unexpected stddev: %f", got.LatencyStddevMs)
	}
	if math.Abs(got.SuccessRate-0.333) > 0.01 || math.Abs(got.TimeoutRate-0.333) > 0.01 {
		t.Fatalf("unexpected rates: %#v", got)
	}
}

func TestSemanticNeighborEnricherFallsBackToModelScope(t *testing.T) {
	service := semanticNeighborTestService(2, semanticNeighborSamples())

	got, err := service.Enrich(tenantContext("tenant-b"), semanticNeighborRequest(), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.CoverageLevel != SemanticCoverageFallback || got.NeighborCount != 3 {
		t.Fatalf("expected model/request fallback, got %#v", got)
	}
}

func TestSemanticNeighborEnricherDefaultsBelowMinCount(t *testing.T) {
	metrics := &semanticNeighborMetricsSpy{StubMetrics: observability.NewStubMetrics()}
	service := semanticNeighborTestService(4, semanticNeighborSamples())
	service.Metrics = metrics

	got, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborRequest(), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.CoverageLevel != SemanticCoverageNone || got.NeighborCount != 0 {
		t.Fatalf("expected neutral defaults, got %#v", got)
	}
	if metrics.fallbackReason != "insufficient_samples" {
		t.Fatalf("expected insufficient sample metric, got %q", metrics.fallbackReason)
	}
}

func TestSemanticNeighborEnricherDefaultsWithNilDependencies(t *testing.T) {
	service := &SemanticNeighborService{Config: SemanticNeighborConfig{Enabled: true, MinCount: 2}}

	got, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborRequest(), TaskFeature{})
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if got.CoverageLevel != SemanticCoverageNone {
		t.Fatalf("expected neutral defaults, got %#v", got)
	}
}

func TestSemanticNeighborHydrationUsesExactIDsInVectorOrder(t *testing.T) {
	repo := &fakeTrainingRepo{samples: semanticNeighborSamples()}
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Repo = repo
	service.Vector = &fakeVector{results: []map[string]interface{}{
		{"sample_id": "s3", "tenant": "tenant-a"},
		{"sample_id": "missing", "tenant": "tenant-a"},
		{"sample_id": "s1", "tenant": "tenant-a"},
	}}

	got, err := service.hydrate(context.Background(), service.Vector.(*fakeVector).results)
	if err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if strings.Join(repo.listByIDs, ",") != "s3,missing,s1" {
		t.Fatalf("expected exact IDs, got %#v", repo.listByIDs)
	}
	if len(got) != 2 || got[0].Sample.ID != "s3" || got[1].Sample.ID != "s1" {
		t.Fatalf("expected found samples in vector order, got %#v", got)
	}
}

func TestSemanticNeighborEmbeddingUsesDefaultModel(t *testing.T) {
	embedder := &recordingEmbedder{}
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Embedder = func() providers.EmbedAdapter { return embedder }

	_, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborRequest(), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if embedder.model != semanticNeighborEmbeddingModel {
		t.Fatalf("expected embedding model %q, got %q", semanticNeighborEmbeddingModel, embedder.model)
	}
}

func TestSemanticNeighborEmbeddingUsesConfiguredModel(t *testing.T) {
	embedder := &recordingEmbedder{}
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Config.EmbeddingModel = "text-embedding-3-large"
	service.Embedder = func() providers.EmbedAdapter { return embedder }

	_, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborRequest(), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if embedder.model != "text-embedding-3-large" {
		t.Fatalf("expected configured embedding model, got %q", embedder.model)
	}
}

func TestSemanticNeighborEmbeddingTruncatesInputByDefault(t *testing.T) {
	var input string
	server := semanticNeighborEmbeddingServer(t, &input)
	defer server.Close()
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Embedder = func() providers.EmbedAdapter {
		return openai.NewAdapter("openai-test", server.URL, "test-key", semanticNeighborEmbeddingModel)
	}
	service.Metrics = &semanticNeighborMetricsSpy{StubMetrics: observability.NewStubMetrics()}

	_, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborLongRequest(defaultSemanticNeighborInputMaxChars+32), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if len(input) != defaultSemanticNeighborInputMaxChars {
		t.Fatalf("expected capped input length %d, got %d", defaultSemanticNeighborInputMaxChars, len(input))
	}
	if service.Metrics.(*semanticNeighborMetricsSpy).errorReason != "input_truncated" {
		t.Fatalf("expected input_truncated metric, got %q", service.Metrics.(*semanticNeighborMetricsSpy).errorReason)
	}
}

func TestSemanticNeighborEmbeddingUsesCustomInputCap(t *testing.T) {
	const inputCap = 32
	var input string
	server := semanticNeighborEmbeddingServer(t, &input)
	defer server.Close()
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Config.InputMaxChars = inputCap
	service.Embedder = func() providers.EmbedAdapter {
		return openai.NewAdapter("openai-test", server.URL, "test-key", semanticNeighborEmbeddingModel)
	}

	_, err := service.Enrich(tenantContext("tenant-a"), semanticNeighborLongRequest(inputCap+7), semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if len(input) != inputCap {
		t.Fatalf("expected custom capped input length %d, got %d", inputCap, len(input))
	}
}

func TestSemanticNeighborEmbeddingCapsMultibyteInputByCharacter(t *testing.T) {
	const inputCap = 8
	var input string
	server := semanticNeighborEmbeddingServer(t, &input)
	defer server.Close()
	service := semanticNeighborTestService(1, semanticNeighborSamples())
	service.Config.InputMaxChars = inputCap
	service.Embedder = func() providers.EmbedAdapter {
		return openai.NewAdapter("openai-test", server.URL, "test-key", semanticNeighborEmbeddingModel)
	}

	req := &llm.LLMRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("界", inputCap+3)}}}
	_, err := service.Enrich(tenantContext("tenant-a"), req, semanticNeighborFeature())
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if len([]rune(input)) != inputCap {
		t.Fatalf("expected %d characters, got %d in %q", inputCap, len([]rune(input)), input)
	}
}

func TestSemanticNeighborRequestTextEmptyInput(t *testing.T) {
	got, truncated := requestText(nil, defaultSemanticNeighborInputMaxChars)
	if got != "" || truncated {
		t.Fatalf("expected empty non-truncated nil request, got %q truncated=%v", got, truncated)
	}
	got, truncated = requestText(&llm.LLMRequest{}, defaultSemanticNeighborInputMaxChars)
	if got != "" || truncated {
		t.Fatalf("expected empty non-truncated request, got %q truncated=%v", got, truncated)
	}
}

func TestSemanticNeighborIndexerWritesSafeMetadata(t *testing.T) {
	vector := &fakeVector{}
	service := semanticNeighborTestService(2, semanticNeighborSamples())
	service.Vector = vector
	task := Task{ID: "task-1", Feature: semanticNeighborFeature()}
	labels := TrainingLabels{Outcome: TrainingOutcomeTimeout, CompletedAt: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)}

	err := service.IndexCompletedSample(tenantContext("tenant-a"), semanticNeighborRequest(), task, labels, "sample-1")
	if err != nil {
		t.Fatalf("IndexCompletedSample: %v", err)
	}
	if len(vector.inserted) != 1 {
		t.Fatalf("expected one vector insert, got %d", len(vector.inserted))
	}
	meta := vector.inserted[0]
	for _, key := range []string{"sample_id", "tenant", "model_class", "request_kind", "outcome", "completed_at"} {
		if meta[key] == "" {
			t.Fatalf("missing safe metadata key %q in %#v", key, meta)
		}
	}
	for _, forbidden := range []string{"prompt", "embedding", "api_key", "authorization", "semantic_cache_payload"} {
		if _, ok := meta[forbidden]; ok {
			t.Fatalf("unsafe metadata key %q in %#v", forbidden, meta)
		}
	}
}

func TestSynchronousRunnerIndexesAfterTrainingSampleRecord(t *testing.T) {
	events := []string{}
	repo := &orderingTrainingRepo{events: &events}
	indexer := &orderingIndexer{events: &events}
	registry := NewResultRegistry()
	queue := NewMemoryQueue()
	intake := &TaskIntake{
		Queue: queue, Scorer: FIFOScorer{Reason: "disabled"}, Registry: registry,
		Priority: NewPriorityResolver(nil), Policy: PriorityPolicy{Default: PriorityNormal, Max: PriorityHigh},
		Backend: "memory",
	}
	runner := NewSynchronousRunner(intake, &Executor{Queue: queue, Registry: registry}, registry)
	runner.Recorder = &TrainingRecorder{Repo: repo}
	runner.Indexer = indexer

	_, err := runner.RunChat(tenantContext("tenant-a"), &llm.LLMRequest{RequestID: "task-1"}, func(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
		return &llm.LLMResponse{Usage: &llm.Usage{CompletionTokens: 8}}, nil
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if len(events) != 2 || events[0] != "record" || events[1] != "index" {
		t.Fatalf("unexpected order: %v", events)
	}
	if indexer.sampleID == "" || indexer.outcome != TrainingOutcomeSuccess {
		t.Fatalf("indexer missing sample evidence: %#v", indexer)
	}
}

func semanticNeighborTestService(minCount int, samples []*controlstate.SchedulerTrainingSample) *SemanticNeighborService {
	results := make([]map[string]interface{}, 0, len(samples))
	for _, sample := range samples {
		results = append(results, map[string]interface{}{
			"sample_id":    sample.ID,
			"tenant":       sample.RouteHint,
			"model_class":  sample.ModelClass,
			"request_kind": sample.RequestKind,
		})
	}
	return &SemanticNeighborService{
		Config:   SemanticNeighborConfig{Enabled: true, MinCount: minCount},
		Embedder: func() providers.EmbedAdapter { return fakeEmbedder{} },
		Vector:   &fakeVector{results: results},
		Repo:     &fakeTrainingRepo{samples: samples},
	}
}

func semanticNeighborSamples() []*controlstate.SchedulerTrainingSample {
	now := time.Now().UTC()
	return []*controlstate.SchedulerTrainingSample{
		semanticNeighborSample("s1", "tenant-a", 100, 10, TrainingOutcomeSuccess, now),
		semanticNeighborSample("s2", "tenant-a", 200, 50, TrainingOutcomeFailure, now),
		semanticNeighborSample("s3", "tenant-a", 300, 90, TrainingOutcomeTimeout, now),
	}
}

func semanticNeighborSample(id string, tenant string, latency int64, output int64, outcome string, completed time.Time) *controlstate.SchedulerTrainingSample {
	return &controlstate.SchedulerTrainingSample{
		ID: id, RouteHint: tenant, ModelClass: "standard", RequestKind: string(RequestKindSimpleQA),
		ActualLatencyMs: latency, OutputTokens: output, Outcome: outcome, CompletedAt: completed,
	}
}

func semanticNeighborFeature() TaskFeature {
	return TaskFeature{ModelClass: "standard", RequestKind: RequestKindSimpleQA, CoverageLevel: SemanticCoverageNone}
}

func semanticNeighborRequest() *llm.LLMRequest {
	return &llm.LLMRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}}}
}

func semanticNeighborLongRequest(length int) *llm.LLMRequest {
	return &llm.LLMRequest{Messages: []llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("x", length)}}}
}

func semanticNeighborEmbeddingServer(t *testing.T, input *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode embedding request: %v", err)
		}
		if len(body.Input) != 1 {
			t.Fatalf("expected one embedding input, got %d", len(body.Input))
		}
		*input = body.Input[0]
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{{
				"object":    "embedding",
				"index":     0,
				"embedding": []float32{1, 0},
			}},
			"model": semanticNeighborEmbeddingModel,
		})
		if err != nil {
			t.Fatalf("encode embedding response: %v", err)
		}
	}))
}

func tenantContext(id string) context.Context {
	return context.WithValue(context.Background(), middleware.AuthIdentityKey, &middleware.AuthIdentity{ID: id})
}

type fakeEmbedder struct{}

func (fakeEmbedder) ID() string       { return "embedder" }
func (fakeEmbedder) Models() []string { return []string{"embedding"} }
func (fakeEmbedder) Complete(context.Context, *llm.LLMRequest) (*llm.LLMResponse, error) {
	return nil, nil
}
func (fakeEmbedder) HealthCheck(context.Context) providers.HealthStatus {
	return providers.HealthStatus{Available: true}
}
func (fakeEmbedder) Capabilities() providers.CapabilitySet { return providers.CapabilitySet{} }
func (fakeEmbedder) Embed(context.Context, *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}, nil
}

type recordingEmbedder struct {
	fakeEmbedder
	model string
}

func (e *recordingEmbedder) Embed(_ context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	e.model = req.Model
	return &llm.EmbeddingResponse{Data: []llm.Embedding{{Embedding: []float32{1, 0}}}}, nil
}

type fakeVector struct {
	results  []map[string]interface{}
	inserted []map[string]interface{}
}

func (v *fakeVector) Ping(context.Context) error { return nil }
func (v *fakeVector) Search(context.Context, string, []float32, int) ([]map[string]interface{}, error) {
	return v.results, nil
}
func (v *fakeVector) Insert(_ context.Context, _ string, _ [][]float32, metadata []map[string]interface{}) error {
	v.inserted = append(v.inserted, metadata...)
	return nil
}
func (v *fakeVector) Delete(context.Context, string, map[string]interface{}) error { return nil }

type fakeTrainingRepo struct {
	samples   []*controlstate.SchedulerTrainingSample
	listByIDs []string
}

func (r *fakeTrainingRepo) Insert(context.Context, *controlstate.SchedulerTrainingSample) error {
	return nil
}
func (r *fakeTrainingRepo) ListByWindow(context.Context, time.Time, time.Time, int) ([]*controlstate.SchedulerTrainingSample, error) {
	return r.samples, nil
}

func (r *fakeTrainingRepo) ListByIDs(_ context.Context, ids []string) ([]*controlstate.SchedulerTrainingSample, error) {
	r.listByIDs = append([]string(nil), ids...)
	byID := samplesByID(r.samples)
	out := make([]*controlstate.SchedulerTrainingSample, 0, len(ids))
	for _, id := range ids {
		if sample := byID[id]; sample != nil {
			out = append(out, sample)
		}
	}
	return out, nil
}

type orderingTrainingRepo struct {
	events *[]string
}

func (r *orderingTrainingRepo) Insert(_ context.Context, sample *controlstate.SchedulerTrainingSample) error {
	*r.events = append(*r.events, "record")
	return nil
}

func (r *orderingTrainingRepo) ListByWindow(context.Context, time.Time, time.Time, int) ([]*controlstate.SchedulerTrainingSample, error) {
	return nil, nil
}

func (r *orderingTrainingRepo) ListByIDs(context.Context, []string) ([]*controlstate.SchedulerTrainingSample, error) {
	return nil, nil
}

type orderingIndexer struct {
	events   *[]string
	sampleID string
	outcome  string
}

func (i *orderingIndexer) IndexCompletedSample(_ context.Context, _ *llm.LLMRequest, _ Task, labels TrainingLabels, sampleID string) error {
	*i.events = append(*i.events, "index")
	i.sampleID = sampleID
	i.outcome = labels.Outcome
	return nil
}

type semanticNeighborMetricsSpy struct {
	*observability.StubMetrics
	fallbackReason string
	errorReason    string
}

func (m *semanticNeighborMetricsSpy) IncSemanticNeighborFallback(reason string) {
	m.fallbackReason = reason
}

func (m *semanticNeighborMetricsSpy) IncSemanticNeighborError(reason string) {
	m.errorReason = reason
}
