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
						}
					case *prometheus.HistogramVec:
						if c == m.requestLatency {
							m.requestLatency = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.providerLatency {
							m.providerLatency = are.ExistingCollector.(*prometheus.HistogramVec)
						} else if c == m.requestOutcomeLat {
							m.requestOutcomeLat = are.ExistingCollector.(*prometheus.HistogramVec)
						}
					case *prometheus.GaugeVec:
						if c == m.healthStatus {
							m.healthStatus = are.ExistingCollector.(*prometheus.GaugeVec)
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
