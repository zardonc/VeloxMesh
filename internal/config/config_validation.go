package config

import (
	"fmt"
	"math"
	"net/url"
	"time"
)

func validateProvider(p *ProviderConfig) error {
	if p.ID == "" {
		return fmt.Errorf("empty provider id")
	}
	if p.Type != "openai-compatible" && p.Type != "anthropic" && p.Type != "gemini" {
		return fmt.Errorf("unsupported provider type for %s", p.ID)
	}
	if err := validateProviderBaseURL(p.ID, p.BaseURL); err != nil {
		return err
	}
	if err := validateProviderModels(p); err != nil {
		return err
	}
	if p.Timeout != "" {
		if err := validateDurationField(p.Timeout, fmt.Sprintf("provider %s timeout", p.ID)); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderBaseURL(id, baseURL string) error {
	if baseURL == "" {
		return fmt.Errorf("missing base URL for %s", id)
	}
	u, err := url.ParseRequestURI(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL for %s", id)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base URL must use http or https for %s", id)
	}
	if u.Host == "" {
		return fmt.Errorf("base URL host cannot be empty for %s", id)
	}
	return nil
}

func validateProviderModels(p *ProviderConfig) error {
	if len(p.Models) == 0 {
		return fmt.Errorf("missing models for %s", p.ID)
	}
	if p.DefaultModel == "" {
		return nil
	}
	for _, m := range p.Models {
		if m == p.DefaultModel {
			return nil
		}
	}
	return fmt.Errorf("default model %q not found in models for %s", p.DefaultModel, p.ID)
}

func validateFallback(c *Config) error {
	if c.MaxAttempts < 1 {
		return fmt.Errorf("fallback max_attempts must be >= 1")
	}
	if !c.FallbackEnabled && c.MaxAttempts > 1 {
		return fmt.Errorf("explicit multi-attempt fallback setting when fallback is disabled")
	}
	if c.FallbackEnabled && c.MaxAttempts > len(c.Providers) {
		return fmt.Errorf("fallback max_attempts greater than configured provider count")
	}
	return nil
}

func validateSemanticCacheConfig(c *Config) error {
	cache := c.Cache
	if cache.VectorDimension <= 0 {
		return fmt.Errorf("semantic_cache_vector_dimension must be >= 1")
	}
	if cache.PGVector.HNSWM <= 0 || cache.PGVector.HNSWEFConstruct <= 0 || cache.PGVector.SearchEF <= 0 {
		return fmt.Errorf("pgvector numeric settings must be >= 1")
	}
	if cache.PGVector.IndexType != "hnsw" && cache.PGVector.IndexType != "ivfflat" {
		return fmt.Errorf("pgvector_index_type must be 'hnsw' or 'ivfflat'")
	}
	if !cache.Enabled {
		return nil
	}
	switch cache.VectorStore {
	case "", "lancedb", "qdrant", "pgvector":
	default:
		return fmt.Errorf("unsupported semantic_cache_vector_store: %s", cache.VectorStore)
	}
	if cache.VectorStore == "qdrant" && cache.Qdrant.Addr == "" {
		return fmt.Errorf("qdrant_addr is required when semantic_cache_vector_store is qdrant")
	}
	return nil
}

func validateSchedulerConfig(s SchedulerConfig) error {
	if err := validateSchedulerDurations(s); err != nil {
		return err
	}
	if err := validateSchedulerLimits(s); err != nil {
		return err
	}
	if err := validateSchedulerEnums(s); err != nil {
		return err
	}
	return validateSLAPromotionConfig(s)
}

func validateSchedulerDurations(s SchedulerConfig) error {
	if err := validateDurationField(s.Timeout, "scheduler.timeout"); err != nil {
		return err
	}
	if err := validateDurationField(s.BreakerRecoveryTimeout, "scheduler.breaker_recovery_timeout"); err != nil {
		return err
	}
	if err := validateDurationField(s.QueuePopTimeout, "scheduler.queue_pop_timeout"); err != nil {
		return err
	}
	if err := validateDurationField(s.SemanticNeighborsTaskTimeout, "scheduler.semantic_neighbors_task_timeout"); err != nil {
		return err
	}
	if err := validateDurationField(s.SemanticNeighborsBatchTimeout, "scheduler.semantic_neighbors_batch_timeout"); err != nil {
		return err
	}
	taskTimeout, _ := time.ParseDuration(s.SemanticNeighborsTaskTimeout)
	batchTimeout, _ := time.ParseDuration(s.SemanticNeighborsBatchTimeout)
	if batchTimeout < taskTimeout {
		return fmt.Errorf("scheduler.semantic_neighbors_batch_timeout must be >= scheduler.semantic_neighbors_task_timeout")
	}
	return nil
}

func validateSchedulerLimits(s SchedulerConfig) error {
	if s.BreakerFailureThreshold < 1 {
		return fmt.Errorf("scheduler.breaker_failure_threshold must be >= 1")
	}
	if s.SemanticNeighborsMinCount < 1 {
		return fmt.Errorf("scheduler.semantic_neighbors_min_count must be >= 1")
	}
	if s.ONNXRolloutPercent < 0 || s.ONNXRolloutPercent > 100 {
		return fmt.Errorf("scheduler.onnx_rollout_percent must be between 0 and 100")
	}
	if s.ONNXRolloutPercent > 0 && s.ONNXEndpoint == "" {
		return fmt.Errorf("scheduler.onnx_endpoint is required when scheduler.onnx_rollout_percent is greater than 0")
	}
	if math.IsNaN(s.QualityMAPEAlertPercent) || math.IsInf(s.QualityMAPEAlertPercent, 0) || s.QualityMAPEAlertPercent < 0 {
		return fmt.Errorf("scheduler.quality_mape_alert_percent must be a non-negative finite value")
	}
	if math.IsNaN(s.ErrorSpikeAlertRate) || math.IsInf(s.ErrorSpikeAlertRate, 0) || s.ErrorSpikeAlertRate < 0 {
		return fmt.Errorf("scheduler.error_spike_alert_rate must be a non-negative finite value")
	}
	if s.QueueSoftLimit < 0 || s.QueueHardLimit < 0 {
		return fmt.Errorf("scheduler queue limits must be >= 0")
	}
	if s.QueueHardLimit > 0 && s.QueueSoftLimit > s.QueueHardLimit {
		return fmt.Errorf("scheduler.queue_soft_limit cannot exceed scheduler.queue_hard_limit")
	}
	if s.ExecutorConcurrency < 1 {
		return fmt.Errorf("scheduler.executor_concurrency must be >= 1")
	}
	return nil
}

func validateSchedulerEnums(s SchedulerConfig) error {
	if !isSchedulerPriority(s.DefaultPriority) {
		return fmt.Errorf("invalid scheduler.default_priority")
	}
	if !isSchedulerPriority(s.MaxPriority) {
		return fmt.Errorf("invalid scheduler.max_priority")
	}
	switch s.QueueBackend {
	case "auto", "redis", "memory":
	default:
		return fmt.Errorf("scheduler.queue_backend must be 'auto', 'redis', or 'memory'")
	}
	switch s.Mode {
	case "heuristic", "onnx":
	default:
		return fmt.Errorf("scheduler.mode must be 'heuristic' or 'onnx'")
	}
	if s.Mode == "onnx" && s.ONNXArtifactDir == "" {
		return fmt.Errorf("scheduler.onnx_artifact_dir is required when scheduler.mode is onnx")
	}
	return nil
}

func validateSLAPromotionConfig(s SchedulerConfig) error {
	if !s.SLAPromotionEnabled {
		return nil
	}
	if s.SLAPromotionCandidateWindow < 1 {
		return fmt.Errorf("scheduler.sla_promotion_candidate_window must be >= 1")
	}
	for i, rule := range s.SLAPromotionRules {
		if err := validateSLAPromotionRule(i, rule); err != nil {
			return err
		}
	}
	return nil
}

func validateSLAPromotionRule(index int, rule SLAPromotionRule) error {
	prefix := fmt.Sprintf("scheduler.sla_promotion_rules[%d]", index)
	if rule.PolicyID == "" {
		return fmt.Errorf("%s.policy_id is required", prefix)
	}
	if rule.TenantID == "" && rule.TenantClass == "" {
		return fmt.Errorf("%s requires tenant_id or tenant_class", prefix)
	}
	if rule.ModelClass == "" {
		return fmt.Errorf("%s.model_class is required", prefix)
	}
	if !isSchedulerRequestKind(rule.RequestKind) {
		return fmt.Errorf("%s.request_kind is invalid", prefix)
	}
	wait, err := time.ParseDuration(rule.WaitThreshold)
	if err != nil {
		return fmt.Errorf("invalid duration for %s.wait_threshold", prefix)
	}
	if wait <= 0 {
		return fmt.Errorf("%s.wait_threshold must be > 0", prefix)
	}
	return nil
}

func isSchedulerPriority(value string) bool {
	return value == "high" || value == "normal" || value == "low"
}

func isSchedulerRequestKind(value string) bool {
	switch value {
	case "simple_qa", "code_gen", "code_review", "summarization", "translation":
		return true
	case "structured_output", "multi_step", "tool_call", "rag", "creative":
		return true
	default:
		return false
	}
}

func validateHealthCheckConfig(hc *HealthCheckConfig) error {
	if err := validateDurationField(hc.Interval, "health_check.interval"); err != nil {
		return err
	}
	if err := validateDurationField(hc.Timeout, "health_check.timeout"); err != nil {
		return err
	}
	if err := validateDurationField(hc.InitialDelay, "health_check.initial_delay"); err != nil {
		return err
	}
	if err := validateDurationField(hc.StaleAfter, "health_check.stale_after"); err != nil {
		return err
	}
	if hc.FailureThreshold < 1 {
		return fmt.Errorf("health_check.failure_threshold must be >= 1")
	}
	if hc.SuccessThreshold < 1 {
		return fmt.Errorf("health_check.success_threshold must be >= 1")
	}
	if hc.MaxConcurrency < 1 {
		return fmt.Errorf("health_check.max_concurrency must be >= 1")
	}
	return nil
}

func validateProviderHealthCheck(p *ProviderConfig) error {
	if p.HealthCheck.Interval != "" {
		if err := validateDurationField(p.HealthCheck.Interval, fmt.Sprintf("provider %s health_check.interval", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.Timeout != "" {
		if err := validateDurationField(p.HealthCheck.Timeout, fmt.Sprintf("provider %s health_check.timeout", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.InitialDelay != "" {
		if err := validateDurationField(p.HealthCheck.InitialDelay, fmt.Sprintf("provider %s health_check.initial_delay", p.ID)); err != nil {
			return err
		}
	}
	if p.HealthCheck.FailureThreshold != 0 && p.HealthCheck.FailureThreshold < 1 {
		return fmt.Errorf("provider %s health_check.failure_threshold must be >= 1", p.ID)
	}
	if p.HealthCheck.SuccessThreshold != 0 && p.HealthCheck.SuccessThreshold < 1 {
		return fmt.Errorf("provider %s health_check.success_threshold must be >= 1", p.ID)
	}
	return nil
}

func validateDurationField(d, name string) error {
	dur, err := time.ParseDuration(d)
	if err != nil {
		return fmt.Errorf("invalid duration for %s", name)
	}
	if dur < 0 {
		return fmt.Errorf("duration for %s cannot be negative", name)
	}
	return nil
}
