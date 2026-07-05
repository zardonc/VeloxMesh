package scheduler

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/providers"
	"veloxmesh/internal/storage"
)

const (
	semanticNeighborCollection     = "scheduler_training_samples"
	semanticNeighborEmbeddingModel = "text-embedding-3-small"
	semanticNeighborLookback       = 30 * 24 * time.Hour
	semanticNeighborHydrateLimit   = 1000
	semanticNeighborSearchFactor   = 4
)

var errSemanticNeighborEmbeddingEmpty = errors.New("semantic neighbor embedding empty")

type SemanticNeighborEnricher interface {
	Enrich(ctx context.Context, req *llm.LLMRequest, feature TaskFeature) (TaskFeature, error)
}

type SemanticNeighborIndexer interface {
	IndexCompletedSample(ctx context.Context, req *llm.LLMRequest, task Task, labels TrainingLabels, sampleID string) error
}

type SemanticNeighborConfig struct {
	Enabled  bool
	MinCount int
}

type SemanticNeighborService struct {
	Config   SemanticNeighborConfig
	Embedder func() providers.EmbedAdapter
	Vector   storage.VectorAdapter
	Repo     controlstate.SchedulerTrainingSampleRepository
	Metrics  observability.Metrics
}

func (s *SemanticNeighborService) Enrich(ctx context.Context, req *llm.LLMRequest, feature TaskFeature) (TaskFeature, error) {
	if s == nil || !s.Config.Enabled {
		s.recordAttempt("disabled")
		s.recordFallback("disabled")
		return semanticDefaults(feature), nil
	}
	if !s.ready() {
		s.recordAttempt("fallback")
		s.recordFallback("missing_dependency")
		return semanticDefaults(feature), nil
	}
	vector, err := s.embed(ctx, req)
	if err != nil {
		s.recordEnrichError("embedding", err)
		return semanticDefaults(feature), nil
	}
	results, err := s.Vector.Search(ctx, semanticNeighborCollection, vector, s.searchLimit())
	if err != nil {
		s.recordEnrichError("vector_search", err)
		return semanticDefaults(feature), nil
	}
	samples, err := s.hydrate(ctx, results)
	if err != nil {
		s.recordEnrichError("sample_hydrate", err)
		return semanticDefaults(feature), nil
	}
	return s.aggregate(feature, identityID(ctx), samples), nil
}

func (s *SemanticNeighborService) IndexCompletedSample(ctx context.Context, req *llm.LLMRequest, task Task, labels TrainingLabels, sampleID string) error {
	if !s.ready() || sampleID == "" {
		return nil
	}
	vector, err := s.embed(ctx, req)
	if err != nil {
		s.recordEnrichError("embedding", err)
		return err
	}
	metadata := []map[string]interface{}{{
		"sample_id":    sampleID,
		"tenant":       identityID(ctx),
		"model_class":  task.Feature.ModelClass,
		"request_kind": string(task.Feature.RequestKind),
		"outcome":      labels.Outcome,
		"completed_at": labels.CompletedAt.UTC().Format(time.RFC3339),
	}}
	if err := s.Vector.Insert(ctx, semanticNeighborCollection, [][]float32{vector}, metadata); err != nil {
		s.recordError("index")
		return err
	}
	return nil
}

func (s *SemanticNeighborService) ready() bool {
	return s != nil && s.Config.Enabled && s.Vector != nil && s.Repo != nil && s.embedder() != nil
}

func (s *SemanticNeighborService) embedder() providers.EmbedAdapter {
	if s == nil || s.Embedder == nil {
		return nil
	}
	return s.Embedder()
}

func (s *SemanticNeighborService) embed(ctx context.Context, req *llm.LLMRequest) ([]float32, error) {
	resp, err := s.embedder().Embed(ctx, &llm.EmbeddingRequest{
		Model: semanticNeighborEmbeddingModel,
		Input: []string{requestText(req)},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, errSemanticNeighborEmbeddingEmpty
	}
	return resp.Data[0].Embedding, nil
}

func (s *SemanticNeighborService) hydrate(ctx context.Context, results []map[string]interface{}) ([]neighborSample, error) {
	ids := resultIDs(results)
	if len(ids) == 0 {
		return nil, nil
	}
	end := time.Now().UTC()
	rows, err := s.Repo.ListByWindow(ctx, end.Add(-semanticNeighborLookback), end, semanticNeighborHydrateLimit)
	if err != nil {
		return nil, err
	}
	byID := samplesByID(rows)
	out := make([]neighborSample, 0, len(ids))
	for _, result := range results {
		id := stringValue(result["sample_id"])
		if sample := byID[id]; sample != nil {
			out = append(out, neighborSample{Sample: sample, Tenant: stringValue(result["tenant"])})
		}
	}
	return out, nil
}

func (s *SemanticNeighborService) aggregate(feature TaskFeature, tenant string, samples []neighborSample) TaskFeature {
	tenantScoped := filterSamples(samples, tenant, feature.ModelClass, string(feature.RequestKind), true)
	if len(tenantScoped) >= s.minCount() {
		s.recordAttempt("ok")
		s.recordCoverage(SemanticCoverageTenant)
		return applyNeighborStats(feature, tenantScoped, SemanticCoverageTenant, s.minCount())
	}
	fallbackScoped := filterSamples(samples, tenant, feature.ModelClass, string(feature.RequestKind), false)
	if len(fallbackScoped) >= s.minCount() {
		s.recordAttempt("ok")
		s.recordCoverage(SemanticCoverageFallback)
		return applyNeighborStats(feature, fallbackScoped, SemanticCoverageFallback, s.minCount())
	}
	s.recordAttempt("fallback")
	s.recordFallback("insufficient_samples")
	return semanticDefaults(feature)
}

func (s *SemanticNeighborService) minCount() int {
	if s == nil || s.Config.MinCount <= 0 {
		return 20
	}
	return s.Config.MinCount
}

func (s *SemanticNeighborService) searchLimit() int {
	return max(s.minCount()*semanticNeighborSearchFactor, s.minCount())
}

func (s *SemanticNeighborService) recordError(reason string) {
	if s != nil && s.Metrics != nil {
		s.Metrics.IncSemanticNeighborError(reason)
	}
}

func (s *SemanticNeighborService) recordEnrichError(reason string, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		s.recordAttempt("timeout")
		s.recordFallback("timeout")
		if s != nil && s.Metrics != nil {
			s.Metrics.IncSemanticNeighborTimeout()
		}
		return
	}
	s.recordAttempt("error")
	s.recordFallback("error")
	s.recordError(reason)
}

func (s *SemanticNeighborService) recordAttempt(result string) {
	if s != nil && s.Metrics != nil {
		s.Metrics.IncSemanticNeighborAttempt(result)
	}
}

func (s *SemanticNeighborService) recordFallback(reason string) {
	if s != nil && s.Metrics != nil {
		s.Metrics.IncSemanticNeighborFallback(reason)
	}
}

func (s *SemanticNeighborService) recordCoverage(level string) {
	if s != nil && s.Metrics != nil {
		s.Metrics.IncSemanticNeighborCoverage(level)
	}
}

type neighborSample struct {
	Sample *controlstate.SchedulerTrainingSample
	Tenant string
}

func resultIDs(results []map[string]interface{}) map[string]struct{} {
	ids := make(map[string]struct{}, len(results))
	for _, result := range results {
		if id := stringValue(result["sample_id"]); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func samplesByID(samples []*controlstate.SchedulerTrainingSample) map[string]*controlstate.SchedulerTrainingSample {
	out := make(map[string]*controlstate.SchedulerTrainingSample, len(samples))
	for _, sample := range samples {
		out[sample.ID] = sample
	}
	return out
}

func filterSamples(samples []neighborSample, tenant string, modelClass string, requestKind string, requireTenant bool) []*controlstate.SchedulerTrainingSample {
	out := make([]*controlstate.SchedulerTrainingSample, 0, len(samples))
	for _, item := range samples {
		if item.Sample.ModelClass != modelClass || item.Sample.RequestKind != requestKind {
			continue
		}
		if requireTenant && item.Tenant != tenant {
			continue
		}
		out = append(out, item.Sample)
	}
	return out
}

func applyNeighborStats(feature TaskFeature, samples []*controlstate.SchedulerTrainingSample, level string, minCount int) TaskFeature {
	latencies, outputs := sampleDistributions(samples)
	enriched := feature
	enriched.NeighborCount = int64(len(samples))
	enriched.LatencyP50Ms = percentileInt64(latencies, 0.50)
	enriched.LatencyP90Ms = percentileInt64(latencies, 0.90)
	enriched.LatencyStddevMs = stddev(latencies)
	enriched.OutputTokensP70 = percentileInt64(outputs, 0.70)
	enriched.SuccessRate = outcomeRate(samples, TrainingOutcomeSuccess)
	enriched.TimeoutRate = outcomeRate(samples, TrainingOutcomeTimeout)
	enriched.CoverageLevel = level
	enriched.CoverageRatio = math.Min(float64(len(samples))/float64(minCount), 1)
	return enriched
}

func semanticDefaults(feature TaskFeature) TaskFeature {
	if feature.CoverageLevel != "" {
		return feature
	}
	feature.CoverageLevel = SemanticCoverageNone
	return feature
}

func sampleDistributions(samples []*controlstate.SchedulerTrainingSample) ([]int64, []int64) {
	latencies := make([]int64, 0, len(samples))
	outputs := make([]int64, 0, len(samples))
	for _, sample := range samples {
		latencies = append(latencies, sample.ActualLatencyMs)
		outputs = append(outputs, sample.OutputTokens)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	sort.Slice(outputs, func(i, j int) bool { return outputs[i] < outputs[j] })
	return latencies, outputs
}

func percentileInt64(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(p*float64(len(values)))) - 1
	return values[max(index, 0)]
}

func stddev(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, value := range values {
		sum += float64(value)
	}
	mean := sum / float64(len(values))
	var variance float64
	for _, value := range values {
		variance += math.Pow(float64(value)-mean, 2)
	}
	return math.Sqrt(variance / float64(len(values)))
}

func outcomeRate(samples []*controlstate.SchedulerTrainingSample, outcome string) float64 {
	var count int
	for _, sample := range samples {
		if sample.Outcome == outcome {
			count++
		}
	}
	return float64(count) / float64(len(samples))
}

func requestText(req *llm.LLMRequest) string {
	if req == nil {
		return ""
	}
	parts := make([]string, 0, len(req.Messages))
	for _, msg := range req.Messages {
		parts = append(parts, msg.Content)
		for _, part := range msg.MultiContent {
			if part.Type == llm.ContentTypeText {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}
