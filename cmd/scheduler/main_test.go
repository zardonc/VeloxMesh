package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/heuristic"
	"veloxmesh/internal/scheduler/predictive"
	"veloxmesh/internal/scheduler/predictor"
	"veloxmesh/internal/scheduler/predictorv1"
	"veloxmesh/internal/scheduler/schedulerv1"
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

func TestSchedulerServiceLoadsHeuristicConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "heuristic.json")
	if err := os.WriteFile(path, []byte(`{"base_latency":{"simple_qa":1600}}`), 0o600); err != nil {
		t.Fatalf("write heuristic config: %v", err)
	}
	t.Setenv("SCHEDULER_HEURISTIC_CONFIG_FILE", path)
	service, err := newSchedulerService("", "", nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	resp, err := service.BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{{
		TaskId: "t1", ModelClass: "standard", EstimatedInputTokens: 256,
		Priority: string(scheduler.PriorityNormal), RequestKind: string(scheduler.RequestKindSimpleQA),
	}}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	if resp.GetResults()[0].GetPredictedLatencyMs() != 1600 {
		t.Fatalf("expected override latency 1600, got %d", resp.GetResults()[0].GetPredictedLatencyMs())
	}
}

func TestSchedulerServiceONNXInvalidArtifactDegrades(t *testing.T) {
	service, status, err := newSchedulerServiceWithStatus("onnx", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	if _, ok := service.(*predictive.BatchScoreService); !ok {
		t.Fatalf("expected predictive service, got %T", service)
	}
	if status.AnomalyStatus != predictorStatusDegraded || status.AnomalyReason != predictorReasonManifestInvalid {
		t.Fatalf("unexpected degraded status: %#v", status)
	}
}

func TestSchedulerServiceONNXModeUsesPredictiveService(t *testing.T) {
	dir := writeSchedulerMainTestArtifact(t)
	service, status, err := newSchedulerServiceWithStatus("onnx", dir, nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	if _, ok := service.(*predictive.BatchScoreService); !ok {
		t.Fatalf("expected predictive service, got %T", service)
	}
	if status.AnomalyStatus != predictorStatusUnavailable || status.AnomalyReason != predictorReasonSignal {
		t.Fatalf("unexpected anomaly status: %#v", status)
	}
}

func TestSchedulerServiceUsesPythonONNXWorkerSmoke(t *testing.T) {
	dir := writeSchedulerMainRuntimeArtifact(t)
	endpoint := freeLocalEndpoint(t)
	var workerLog bytes.Buffer
	cmd := exec.Command(schedulerTrainingPython(), "-m", "scheduler_training.onnx_worker", "--artifact-dir", dir, "--addr", endpoint)
	cmd.Dir = filepath.Join("..", "..", "tools", "scheduler_training")
	cmd.Stdout = &workerLog
	cmd.Stderr = &workerLog
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	waitForPredictorHealth(t, endpoint, &workerLog)
	t.Setenv("SCHEDULER_PREDICTOR_ENDPOINT", endpoint)

	service, status, err := newSchedulerServiceWithStatus("onnx", dir, nil)
	if err != nil {
		t.Fatalf("newSchedulerService: %v", err)
	}
	if status.AnomalyStatus != predictorStatusReady || status.AnomalyReason != "" {
		t.Fatalf("expected ready predictor status, got %#v", status)
	}
	resp, err := service.BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{{
		TaskId: "smoke", ModelClass: "standard", EstimatedInputTokens: 10,
		EstimatedOutputTokens: 1, Priority: string(scheduler.PriorityNormal), RequestKind: string(scheduler.RequestKindSimpleQA),
	}}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetReason() != "" {
		t.Fatalf("expected non-fallback predictive score, got reason=%q version=%q latency=%d", resp.GetResults()[0].GetReason(), resp.GetResults()[0].GetSchedulerVersion(), resp.GetResults()[0].GetPredictedLatencyMs())
	}
}

func TestRunReturnsHTTPServeError(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen occupied http addr: %v", err)
	}
	defer occupied.Close()

	t.Setenv("SCHEDULER_GRPC_ADDR", freeLocalEndpoint(t))
	t.Setenv("SCHEDULER_HTTP_ADDR", occupied.Addr().String())
	t.Setenv("SCHEDULER_MODE", "heuristic")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err = run(ctx)
	if err == nil {
		t.Fatalf("expected http serve error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("serve error was swallowed until context deadline: %v", err)
	}
	if !strings.Contains(err.Error(), "http serve") {
		t.Fatalf("expected http serve error, got %v", err)
	}
}

func writeSchedulerMainTestArtifact(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifest := predictor.Manifest{
		ProtocolVersion: predictor.ProtocolVersion, ModelVersion: "scheduler-predictor-v1",
		TaskType: "quantile_regression", Quantiles: []int{50, 70, 90},
		FeatureSchema: predictor.SupportedFeatureSchema(), TrainingDataHash: strings.Repeat("a", 64),
		CompatibleSchedulerVersion: ">=0.9.0",
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

func writeSchedulerMainRuntimeArtifact(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	script := `
import json
import sys
from pathlib import Path
from scheduler_training.publish import publish_artifact
from scheduler_training.train import train_file

root = Path(sys.argv[1])
samples = root / "samples.jsonl"
build = root / "build"
build.mkdir(parents=True, exist_ok=True)
rows = [
    {"task_id": "t1", "output_tokens": 10, "outcome": "success"},
    {"task_id": "t2", "output_tokens": 20, "outcome": "success"},
    {"task_id": "t3", "output_tokens": 30, "outcome": "success"},
]
samples.write_text("\n".join(json.dumps(row) for row in rows), encoding="utf-8")
model = build / "model.json"
metrics = build / "metrics.json"
train_file(samples, model)
metrics.write_text(json.dumps({"sample_count": 3}), encoding="utf-8")
artifact = publish_artifact(model, metrics, root / "artifacts", "scheduler-predictor-v1", {})
print(artifact)
`
	cmd := exec.Command("uv", "run", "python", "-c", script, root)
	cmd.Dir = schedulerTrainingDir()
	cmd.Env = schedulerTrainingEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish runtime artifact: %v\n%s", err, string(out))
	}
	return lastOutputLine(string(out))
}

func lastOutputLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func schedulerTrainingPython() string {
	venv := schedulerTrainingVenv()
	if runtime.GOOS == "windows" {
		return filepath.Join(venv, "Scripts", "python.exe")
	}
	return filepath.Join(venv, "bin", "python")
}

func schedulerTrainingDir() string {
	return filepath.Join("..", "..", "tools", "scheduler_training")
}

func schedulerTrainingVenv() string {
	return absTestPath(filepath.Join("..", "..", ".tmp", "scheduler-training-venv"))
}

func schedulerTrainingEnv() []string {
	return withEnv(os.Environ(), []string{
		"UV_PROJECT_ENVIRONMENT=" + schedulerTrainingVenv(),
		"UV_CACHE_DIR=" + absTestPath(filepath.Join("..", "..", ".tmp", "uv-cache")),
	})
}

func absTestPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func withEnv(env []string, overrides []string) []string {
	filtered := make([]string, 0, len(env)+len(overrides))
	for _, item := range env {
		if envKeyOverridden(item, overrides) {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, overrides...)
}

func envKeyOverridden(item string, overrides []string) bool {
	key, _, ok := strings.Cut(item, "=")
	if !ok {
		return false
	}
	for _, override := range overrides {
		overrideKey, _, _ := strings.Cut(override, "=")
		if strings.EqualFold(key, overrideKey) {
			return true
		}
	}
	return false
}

func freeLocalEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func waitForPredictorHealth(t *testing.T, endpoint string, workerLog *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			client := predictorv1.NewOutputTokenPredictorClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			health, callErr := client.Health(ctx, &predictorv1.HealthRequest{})
			cancel()
			_ = conn.Close()
			if callErr == nil && health.GetReady() {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("predictor worker did not become healthy at %s\n%s", endpoint, workerLog.String())
}
