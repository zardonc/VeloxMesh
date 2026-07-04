package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"veloxmesh/internal/scheduler/heuristic"
)

func TestSchedulerHTTPHealthAndMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := heuristic.NewMetrics(reg)
	metrics.Observe(1, "structured", 2)
	server := httptest.NewServer(newHTTPMux(reg))
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", resp.StatusCode)
	}

	metricsResp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer metricsResp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := metricsResp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "scheduler_tasks_scored_total") && !strings.Contains(body, "scheduler_batch_score_duration_ms") {
		t.Fatalf("missing scheduler metrics: %s", body)
	}
}
