package observability

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusMetrics struct {
	requestCount      *prometheus.CounterVec
	requestLatency    *prometheus.HistogramVec
	providerLatency   *prometheus.HistogramVec
	routingStrategy   *prometheus.CounterVec
	healthStatus      *prometheus.GaugeVec
	requestOutcome    *prometheus.CounterVec
	requestOutcomeLat *prometheus.HistogramVec
	queueDepth        *prometheus.GaugeVec
	taskWait          *prometheus.HistogramVec
	schedulerCall     *prometheus.HistogramVec
	schedulerErrors   *prometheus.CounterVec
	breakerState      *prometheus.GaugeVec
	priorityDowngrade *prometheus.CounterVec
	classificationSrc *prometheus.CounterVec
	predictionMAPE    *prometheus.HistogramVec
	comparisonWait    *prometheus.HistogramVec
	comparisonCall    *prometheus.HistogramVec
	comparisonErrors  *prometheus.CounterVec
	anomalyStatus     *prometheus.CounterVec
	rolloutAlerts     *prometheus.CounterVec
	semanticAttempts  *prometheus.CounterVec
	semanticTimeouts  prometheus.Counter
	semanticErrors    *prometheus.CounterVec
	semanticFallbacks *prometheus.CounterVec
	semanticCoverage  *prometheus.CounterVec
}

func NewPrometheusMetrics(reg prometheus.Registerer) *PrometheusMetrics {
	m := &PrometheusMetrics{
		requestCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "veloxmesh_request_count_total",
				Help: "Total number of requests.",
			},
			[]string{"provider", "model", "status"},
		),
		requestLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "veloxmesh_request_latency_ms",
				Help:    "Request latency in milliseconds.",
				Buckets: prometheus.DefBuckets, // Consider custom buckets if needed
			},
			[]string{"provider", "model"},
		),
		providerLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "veloxmesh_provider_latency_ms",
				Help:    "Provider-specific latency in milliseconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"provider"},
		),
		routingStrategy: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "veloxmesh_routing_strategy_total",
				Help: "Total number of times a routing strategy was chosen.",
			},
			[]string{"strategy"},
		),
		healthStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "veloxmesh_health_status",
				Help: "Current health status of a provider (1=healthy, 0=degraded/unhealthy).",
			},
			[]string{"provider", "status"},
		),
		requestOutcome: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "veloxmesh_request_outcome_total",
				Help: "Total number of completed request outcomes.",
			},
			[]string{"provider", "model", "strategy", "status", "cache_result", "error_category"},
		),
		requestOutcomeLat: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "veloxmesh_request_outcome_latency_ms",
				Help:    "Latency of completed request outcomes in milliseconds.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"provider", "model", "strategy", "status", "cache_result", "error_category"},
		),
		queueDepth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_queue_depth",
			Help: "Current gateway scheduler queue depth.",
		}, []string{"backend", "priority"}),
		taskWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_task_wait_duration_ms",
			Help:    "Gateway task queue wait duration in milliseconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"priority"}),
		schedulerCall: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_scheduler_call_duration_ms",
			Help:    "Gateway scheduler call latency in milliseconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"result"}),
		schedulerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_errors_total",
			Help: "Gateway scheduler errors.",
		}, []string{"reason"}),
		breakerState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_circuit_breaker_state",
			Help: "Scheduler circuit breaker state.",
		}, []string{"state"}),
		priorityDowngrade: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_priority_downgrade_total",
			Help: "Priority downgrade count.",
		}, []string{"reason", "from", "to"}),
		classificationSrc: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_classification_source_total",
			Help: "Gateway scheduler classification source count.",
		}, []string{"source"}),
		predictionMAPE: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_scheduler_prediction_mape_percent",
			Help:    "Gateway scheduler prediction MAPE by backend.",
			Buckets: prometheus.DefBuckets,
		}, []string{"scheduler_type", "scheduler_version", "task_type", "coverage_level", "anomaly_status"}),
		comparisonWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_scheduler_comparison_wait_duration_ms",
			Help:    "Gateway scheduler rollout task wait duration by backend.",
			Buckets: prometheus.DefBuckets,
		}, []string{"scheduler_type", "scheduler_version", "task_type", "coverage_level", "anomaly_status"}),
		comparisonCall: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_scheduler_comparison_call_duration_ms",
			Help:    "Gateway scheduler rollout call duration by backend.",
			Buckets: prometheus.DefBuckets,
		}, []string{"scheduler_type", "scheduler_version", "task_type", "coverage_level", "anomaly_status"}),
		comparisonErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_comparison_errors_total",
			Help: "Gateway scheduler rollout errors by backend.",
		}, []string{"scheduler_type", "scheduler_version", "task_type"}),
		anomalyStatus: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_anomaly_status_total",
			Help: "Gateway scheduler anomaly/OOD status counts.",
		}, []string{"scheduler_version", "task_type", "coverage_level", "anomaly_status"}),
		rolloutAlerts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_rollout_alerts_total",
			Help: "Gateway scheduler rollout alert count.",
		}, []string{"reason"}),
		semanticAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_semantic_neighbor_attempts_total",
			Help: "Gateway scheduler semantic-neighbor enrichment attempts.",
		}, []string{"result"}),
		semanticTimeouts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "gateway_scheduler_semantic_neighbor_timeouts_total",
			Help: "Gateway scheduler semantic-neighbor enrichment timeouts.",
		}),
		semanticErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_semantic_neighbor_errors_total",
			Help: "Gateway scheduler semantic-neighbor enrichment errors.",
		}, []string{"reason"}),
		semanticFallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_semantic_neighbor_fallbacks_total",
			Help: "Gateway scheduler semantic-neighbor fallback reasons.",
		}, []string{"reason"}),
		semanticCoverage: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_scheduler_semantic_neighbor_coverage_total",
			Help: "Gateway scheduler semantic-neighbor coverage levels.",
		}, []string{"coverage_level"}),
	}

	if reg != nil {
		for _, collector := range []prometheus.Collector{
			m.requestCount,
			m.requestLatency,
			m.providerLatency,
			m.routingStrategy,
			m.healthStatus,
			m.requestOutcome,
			m.requestOutcomeLat,
			m.queueDepth,
			m.taskWait,
			m.schedulerCall,
			m.schedulerErrors,
			m.breakerState,
			m.priorityDowngrade,
			m.classificationSrc,
			m.predictionMAPE,
			m.comparisonWait,
			m.comparisonCall,
			m.comparisonErrors,
			m.anomalyStatus,
			m.rolloutAlerts,
			m.semanticAttempts,
			m.semanticTimeouts,
			m.semanticErrors,
			m.semanticFallbacks,
			m.semanticCoverage,
		} {
			if err := reg.Register(collector); err != nil {
				if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
					// Use the already registered collector instead to avoid duplicate series panics in tests
					switch c := collector.(type) {
					case *prometheus.CounterVec:
						if c == m.requestCount {
							m.requestCount = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.routingStrategy {
							m.routingStrategy = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.requestOutcome {
							m.requestOutcome = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.schedulerErrors {
							m.schedulerErrors = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.priorityDowngrade {
							m.priorityDowngrade = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.classificationSrc {
							m.classificationSrc = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.comparisonErrors {
							m.comparisonErrors = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.anomalyStatus {
							m.anomalyStatus = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.rolloutAlerts {
							m.rolloutAlerts = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.semanticAttempts {
							m.semanticAttempts = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.semanticErrors {
							m.semanticErrors = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.semanticFallbacks {
							m.semanticFallbacks = are.ExistingCollector.(*prometheus.CounterVec)
						} else if c == m.semanticCoverage {
							m.semanticCoverage = are.ExistingCollector.(*prometheus.CounterVec)
						}
					case *prometheus.HistogramVec:
						if c == m.requestLatency {
							m.requestLatency = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.providerLatency {
							m.providerLatency = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.requestOutcomeLat {
							m.requestOutcomeLat = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.taskWait {
							m.taskWait = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.schedulerCall {
							m.schedulerCall = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.predictionMAPE {
							m.predictionMAPE = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.comparisonWait {
							m.comparisonWait = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.comparisonCall {
							m.comparisonCall = are.ExistingCollector.(*prometheus.HistogramVec)
						}
					case *prometheus.GaugeVec:
						if c == m.healthStatus {
							m.healthStatus = are.ExistingCollector.(*prometheus.GaugeVec)
						} else if c == m.queueDepth {
							m.queueDepth = are.ExistingCollector.(*prometheus.GaugeVec)
						} else if c == m.breakerState {
							m.breakerState = are.ExistingCollector.(*prometheus.GaugeVec)
						}
					case prometheus.Counter:
						if c == m.semanticTimeouts {
							m.semanticTimeouts = are.ExistingCollector.(prometheus.Counter)
						}
					}
				} else {
					panic(err)
				}
			}
		}
	}

	return m
}

func (m *PrometheusMetrics) IncRequestCount(provider string, model string, status int) {
	m.requestCount.WithLabelValues(provider, model, strconv.Itoa(status)).Inc()
}

func (m *PrometheusMetrics) RecordLatency(provider string, model string, latencyMs int64) {
	m.requestLatency.WithLabelValues(provider, model).Observe(float64(latencyMs))
}

func (m *PrometheusMetrics) RecordProviderLatency(provider string, latencyMs float64) {
	m.providerLatency.WithLabelValues(provider).Observe(latencyMs)
}

func (m *PrometheusMetrics) RecordRoutingStrategy(strategy string) {
	m.routingStrategy.WithLabelValues(strategy).Inc()
}

func (m *PrometheusMetrics) RecordHealthStatus(provider string, status string) {
	// Gauge value 1.0 represents current state, others could be set to 0, but resetting old labels is hard unless we track them.
	// For simplicity, we just set the gauge for the current status. To avoid stale labels, we can clear and set.
	m.healthStatus.DeletePartialMatch(prometheus.Labels{"provider": provider})
	m.healthStatus.WithLabelValues(provider, status).Set(1.0)
}

func (m *PrometheusMetrics) RecordRequestOutcome(reqID string, provider string, model string, strategy string, status int, errorCategory string, cacheResult string, latencyMs float64) {
	// D-12: explicitly do not log reqID or any high cardinality identifiers
	statusStr := strconv.Itoa(status)
	m.requestOutcome.WithLabelValues(provider, model, strategy, statusStr, cacheResult, errorCategory).Inc()
	m.requestOutcomeLat.WithLabelValues(provider, model, strategy, statusStr, cacheResult, errorCategory).Observe(latencyMs)
}

func (m *PrometheusMetrics) RecordQueueDepth(backend string, priority string, depth int64) {
	m.queueDepth.WithLabelValues(allowedLabel(backend, "memory", "redis"), allowedLabel(priority, "normal", "high", "low")).Set(float64(depth))
}

func (m *PrometheusMetrics) RecordTaskWait(priority string, waitMs float64) {
	m.taskWait.WithLabelValues(allowedLabel(priority, "normal", "high", "low")).Observe(waitMs)
}

func (m *PrometheusMetrics) RecordSchedulerCall(result string, latencyMs float64) {
	m.schedulerCall.WithLabelValues(allowedLabel(result, "fallback", "ok", "timeout", "error")).Observe(latencyMs)
}

func (m *PrometheusMetrics) IncSchedulerError(reason string) {
	m.schedulerErrors.WithLabelValues(allowedLabel(reason, "error", "timeout", "queue", "breaker_open")).Inc()
}

func (m *PrometheusMetrics) RecordSchedulerBreakerState(state string) {
	m.breakerState.DeletePartialMatch(prometheus.Labels{})
	m.breakerState.WithLabelValues(allowedLabel(state, "closed", "half_open", "open")).Set(1)
}

func (m *PrometheusMetrics) IncPriorityDowngrade(reason string, from string, to string) {
	m.priorityDowngrade.WithLabelValues(
		allowedLabel(reason, "policy", "quota", "untrusted"),
		allowedLabel(from, "normal", "high", "low"),
		allowedLabel(to, "normal", "high", "low"),
	).Inc()
}

func (m *PrometheusMetrics) IncSchedulerClassificationSource(source string) {
	m.classificationSrc.WithLabelValues(allowedLabel(source, "fallback", "structured", "rule")).Inc()
}

func (m *PrometheusMetrics) RecordSchedulerPredictionMAPE(schedulerType string, schedulerVersion string, taskType string, coverageLevel string, anomalyStatus string, mape float64) {
	m.predictionMAPE.WithLabelValues(safeSchedulerType(schedulerType), safeSchedulerVersion(schedulerVersion), safeTaskType(taskType), safeCoverageLevel(coverageLevel), safeAnomalyStatus(anomalyStatus)).Observe(mape)
}

func (m *PrometheusMetrics) RecordSchedulerComparisonWait(schedulerType string, schedulerVersion string, taskType string, coverageLevel string, anomalyStatus string, waitMs float64) {
	m.comparisonWait.WithLabelValues(safeSchedulerType(schedulerType), safeSchedulerVersion(schedulerVersion), safeTaskType(taskType), safeCoverageLevel(coverageLevel), safeAnomalyStatus(anomalyStatus)).Observe(waitMs)
}

func (m *PrometheusMetrics) RecordSchedulerComparisonCall(schedulerType string, schedulerVersion string, taskType string, coverageLevel string, anomalyStatus string, latencyMs float64) {
	m.comparisonCall.WithLabelValues(safeSchedulerType(schedulerType), safeSchedulerVersion(schedulerVersion), safeTaskType(taskType), safeCoverageLevel(coverageLevel), safeAnomalyStatus(anomalyStatus)).Observe(latencyMs)
}

func (m *PrometheusMetrics) IncSchedulerComparisonError(schedulerType string, schedulerVersion string, taskType string) {
	m.comparisonErrors.WithLabelValues(safeSchedulerType(schedulerType), safeSchedulerVersion(schedulerVersion), safeTaskType(taskType)).Inc()
}

func (m *PrometheusMetrics) IncSchedulerAnomalyStatus(schedulerVersion string, taskType string, coverageLevel string, anomalyStatus string) {
	m.anomalyStatus.WithLabelValues(
		safeSchedulerVersion(schedulerVersion),
		safeTaskType(taskType),
		safeCoverageLevel(coverageLevel),
		safeAnomalyStatus(anomalyStatus),
	).Inc()
}

func (m *PrometheusMetrics) IncSchedulerRolloutAlert(reason string) {
	m.rolloutAlerts.WithLabelValues(allowedLabel(reason, "mape_degradation", "mape_degradation", "scheduler_error_spike")).Inc()
}

func (m *PrometheusMetrics) IncSemanticNeighborAttempt(result string) {
	m.semanticAttempts.WithLabelValues(allowedLabel(result, "fallback", "ok", "fallback", "disabled", "timeout", "error")).Inc()
}

func (m *PrometheusMetrics) IncSemanticNeighborTimeout() {
	m.semanticTimeouts.Inc()
}

func (m *PrometheusMetrics) IncSemanticNeighborError(reason string) {
	m.semanticErrors.WithLabelValues(allowedLabel(reason, "error", "embedding", "vector_search", "sample_hydrate", "index")).Inc()
}

func (m *PrometheusMetrics) IncSemanticNeighborFallback(reason string) {
	m.semanticFallbacks.WithLabelValues(allowedLabel(reason, "insufficient_samples", "disabled", "missing_dependency", "insufficient_samples", "timeout", "error")).Inc()
}

func (m *PrometheusMetrics) IncSemanticNeighborCoverage(level string) {
	m.semanticCoverage.WithLabelValues(allowedLabel(level, "none", "none", "tenant", "fallback")).Inc()
}

func safeSchedulerType(value string) string {
	return allowedLabel(value, "unknown", "fifo", "heuristic", "onnx")
}

func safeSchedulerVersion(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func safeTaskType(value string) string {
	return allowedLabel(value, "simple_qa", "simple_qa", "code_gen", "code_review", "summarization", "translation", "structured_output", "multi_step", "tool_call", "rag", "creative")
}

func safeCoverageLevel(value string) string {
	return allowedLabel(value, "none", "none", "fallback", "tenant", "all")
}

func safeAnomalyStatus(value string) string {
	return allowedLabel(value, "normal", "normal", "ood", "unavailable", "degraded")
}

func allowedLabel(value string, fallback string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}
