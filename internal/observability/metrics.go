package observability

import (
	"sync/atomic"
)

type Metrics interface {
	IncRequestCount(provider string, model string, status int)
	RecordLatency(provider string, model string, latencyMs int64)
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

// Global metrics instance for Phase 1
var DefaultMetrics Metrics = NewStubMetrics()
