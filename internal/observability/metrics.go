package observability

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics interface {
	IncRequestCount(provider string, model string, status int)
	RecordLatency(provider string, model string, latencyMs int64)
	RecordProviderLatency(provider string, latencyMs float64)
	RecordRoutingStrategy(strategy string)
	RecordHealthStatus(provider string, status string)
	RecordRequestOutcome(reqID string, provider string, model string, strategy string, status int, errorCategory string, cacheResult string, latencyMs float64)
}

type StubMetrics struct {
	totalRequests atomic.Int64
}

func NewStubMetrics() *StubMetrics {
	return &StubMetrics{}
}

func (m *StubMetrics) IncRequestCount(provider string, model string, status int) {
	m.totalRequests.Add(1)
	// Placeholder for future Prometheus counters
}

func (m *StubMetrics) RecordLatency(provider string, model string, latencyMs int64) {
	// Placeholder for future Prometheus histograms
}

func (m *StubMetrics) RecordProviderLatency(provider string, latencyMs float64) {}

func (m *StubMetrics) RecordRoutingStrategy(strategy string) {}

func (m *StubMetrics) RecordHealthStatus(provider string, status string) {}

func (m *StubMetrics) RecordRequestOutcome(reqID string, provider string, model string, strategy string, status int, errorCategory string, cacheResult string, latencyMs float64) {
	// Log the outcome securely without dumping raw response bodies or prompts.
}

// Global metrics instance for Phase 1/2
var DefaultMetrics Metrics = NewStubMetrics()

func InitPrometheusMetrics() {
	DefaultMetrics = NewPrometheusMetrics(prometheus.DefaultRegisterer)
}
