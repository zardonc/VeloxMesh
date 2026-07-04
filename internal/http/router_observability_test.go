package http

import (
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"veloxmesh/internal/config"
	"veloxmesh/internal/coordination"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/observability"
)

func TestMetricsRouteIsScrapeable(t *testing.T) {
	cfg := &config.Config{}

	// Create isolated registry
	reg := prometheus.NewRegistry()
	m := observability.NewPrometheusMetrics(reg)

	// Temporarily override global gatherer so promhttp.Handler() uses our registry if we were to configure it.
	// Actually, the default promhttp.Handler() uses prometheus.DefaultGatherer.
	// Since we can't easily mock DefaultGatherer for just this test without risking side effects,
	// we will register our metrics to the default registry to ensure it's scrapeable.
	observability.DefaultMetrics = m
	prometheus.MustRegister(reg) // just for test purposes, or we can just rely on DefaultMetrics

	// Let's just generate a request and hit /metrics
	m.RecordRequestOutcome("req-1", "openai", "gpt-4", "test", 200, "", "none", 100)

	svc := gateway.NewService(nil, nil, nil, false, 1, nil, nil, nil, nil, nil)
	r := NewRouter(cfg, svc, nil, nil, nil, nil, hotstate.NewLocalHotState(), nil, coordination.NewNoopCoordinator(), nil)

	req, _ := http.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "veloxmesh_request_outcome_total") {
		t.Errorf("Expected body to contain veloxmesh_request_outcome_total, got %s", body)
	}
}
