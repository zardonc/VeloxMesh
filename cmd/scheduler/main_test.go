package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"veloxmesh/internal/scheduler/heuristic"
	scheduleronnx "veloxmesh/internal/scheduler/onnx"
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

func TestSchedulerHTTPStatusExposesAnomalyEnums(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := httptest.NewServer(newHTTPMux(reg, schedulerStatus{
		AnomalyStatus: "unavailable",
		AnomalyReason: "missing_metadata",
	}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["anomaly_status"] != "unavailable" || body["anomaly_reason"] != "missing_metadata" {
		t.Fatalf("unexpected status body: %#v", body)
	}
	if _, ok := body["threshold"]; ok {
		t.Fatalf("status leaked threshold details: %#v", body)
	}
}

func TestSchedulerServiceDefaultsToHeuristic(t *testing.T) {
	service, err := newSchedulerService("", "", nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	if _, ok := service.(*heuristic.BatchScoreService); !ok {
		t.Fatalf("expected heuristic service, got %T", service)
	}
}

func TestSchedulerServiceONNXInvalidArtifactFails(t *testing.T) {
	_, err := newSchedulerService("onnx", t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "start ONNX scheduler") {
		t.Fatalf("expected ONNX startup error, got %v", err)
	}
}

func TestSchedulerServiceONNXValidArtifactStarts(t *testing.T) {
	dir := writeSchedulerMainTestArtifact(t)
	service, status, err := newSchedulerServiceWithStatus("onnx", dir, nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	if _, ok := service.(*scheduleronnx.BatchScoreService); !ok {
		t.Fatalf("expected ONNX service, got %T", service)
	}
	if status.AnomalyStatus != "unavailable" || status.AnomalyReason != "missing_metadata" {
		t.Fatalf("unexpected anomaly status: %#v", status)
	}
}

func writeSchedulerMainTestArtifact(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	model := []byte("onnx-test-model")
	if err := os.WriteFile(filepath.Join(dir, "model.onnx"), model, 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	sum := sha256.Sum256(model)
	manifest := map[string]any{
		"scheduler_version":      "scheduler-p70-v1",
		"model_version":          "scheduler-p70-v1",
		"target":                 "p70_output_tokens",
		"feature_schema_version": "scheduler-training-v1",
		"training_window":        map[string]string{"start": "a", "end": "b"},
		"metrics":                map[string]float64{"mae": 1},
		"onnx_parity":            map[string]any{"passed": true, "max_abs_error": 0},
		"model_sha256":           hex.EncodeToString(sum[:]),
		"model_parameters":       map[string]float64{"p70_output_tokens": 42},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return dir
}
