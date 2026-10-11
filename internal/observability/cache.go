package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"time"
)

func (m *PrometheusMetrics) initCacheMetrics(reg prometheus.Registerer) {
	m.cacheOutcomes = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "veloxmesh_semantic_cache_outcome_total", Help: "Optional semantic cache outcomes, including bypasses and dropped writes."}, []string{"operation", "reason"})
	m.cacheOperations = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "veloxmesh_semantic_cache_operation_seconds", Help: "Cache operation duration including complete writes.", Buckets: prometheus.DefBuckets}, []string{"operation", "result"})
	if reg == nil {
		return
	}
	if err := reg.Register(m.cacheOutcomes); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m.cacheOutcomes = existing.ExistingCollector.(*prometheus.CounterVec)
		} else {
			panic(err)
		}
	}
	if err := reg.Register(m.cacheOperations); err != nil {
		if existing, ok := err.(prometheus.AlreadyRegisteredError); ok {
			m.cacheOperations = existing.ExistingCollector.(*prometheus.HistogramVec)
		} else {
			panic(err)
		}
	}
}

func (m *PrometheusMetrics) RecordCacheOutcome(operation, reason string) {
	m.cacheOutcomes.WithLabelValues(allowedLabel(operation, "lookup", "store", "enqueue"), allowedLabel(reason, "hit", "miss", "timeout", "embedding_error", "invalid_embedding", "vector_error", "repository_error", "invalid_entry", "concurrency_full", "queue_full", "closed", "shutdown_drop", "shutdown_cancelled", "accepted", "stored")).Inc()
}

func (m *PrometheusMetrics) RecordCacheOperation(operation string, elapsed time.Duration, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	m.cacheOperations.WithLabelValues(allowedLabel(operation, "lookup_embedding", "store_embedding", "vector_search", "vector_insert", "repo_read", "repo_write", "store_total"), result).Observe(elapsed.Seconds())
}
