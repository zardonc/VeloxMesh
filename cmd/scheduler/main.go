package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"veloxmesh/internal/scheduler/heuristic"
	"veloxmesh/internal/scheduler/predictive"
	"veloxmesh/internal/scheduler/predictor"
	"veloxmesh/internal/scheduler/schedulerv1"
)

const (
	defaultPredictorTimeout        = 15 * time.Millisecond
	predictorStatusDegraded        = "degraded"
	predictorStatusUnavailable     = "unavailable"
	predictorReasonManifestInvalid = "manifest_invalid"
	predictorReasonSignal          = "predictor_signal"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	grpcAddr := getenv("SCHEDULER_GRPC_ADDR", ":50051")
	httpAddr := getenv("SCHEDULER_HTTP_ADDR", ":9091")
	reg := prometheus.NewRegistry()
	metrics := heuristic.NewMetrics(reg)
	service, status, err := newSchedulerServiceWithStatus(getenv("SCHEDULER_MODE", "heuristic"), getenv("SCHEDULER_ONNX_ARTIFACT_DIR", ""), metrics)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	schedulerv1.RegisterTaskSchedulerServer(grpcServer, service)
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()

	httpServer := &http.Server{Addr: httpAddr, Handler: newHTTPMux(reg, status)}
	go func() { _ = httpServer.ListenAndServe() }()
	defer httpServer.Shutdown(ctx)

	<-ctx.Done()
	return ctx.Err()
}

func newSchedulerService(mode, artifactDir string, metrics *heuristic.Metrics) (schedulerv1.TaskSchedulerServer, error) {
	service, _, err := newSchedulerServiceWithStatus(mode, artifactDir, metrics)
	return service, err
}

type schedulerStatus struct {
	AnomalyStatus string `json:"anomaly_status,omitempty"`
	AnomalyReason string `json:"anomaly_reason,omitempty"`
}

func newSchedulerServiceWithStatus(mode, artifactDir string, metrics *heuristic.Metrics) (schedulerv1.TaskSchedulerServer, schedulerStatus, error) {
	if mode == "" || mode == "heuristic" {
		return heuristic.NewBatchScoreService(nil, metrics), schedulerStatus{}, nil
	}
	if mode != "onnx" && mode != "predictive" {
		return nil, schedulerStatus{}, fmt.Errorf("unsupported scheduler mode: %s", mode)
	}
	scorer, status := newPredictiveScorer(context.Background(), artifactDir, metrics)
	fmt.Fprintf(os.Stderr, "predictive anomaly_status=%s anomaly_reason=%s\n", status.AnomalyStatus, status.AnomalyReason)
	return predictive.NewBatchScoreService(scorer), status, nil
}

func newPredictiveScorer(ctx context.Context, artifactDir string, _ *heuristic.Metrics) (*predictive.Scorer, schedulerStatus) {
	manifest, err := predictor.LoadManifest(filepath.Join(artifactDir, "manifest.json"))
	if err != nil {
		p := predictor.NoopPredictor{Reason: err}
		return predictive.NewScorer(p, predictive.Config{}), schedulerStatus{AnomalyStatus: predictorStatusDegraded, AnomalyReason: predictorReasonManifestInvalid}
	}
	p, _ := predictor.NewPythonONNXPredictor(ctx, predictor.PythonClientConfig{
		Endpoint: getenv("SCHEDULER_PREDICTOR_ENDPOINT", ""), Timeout: defaultPredictorTimeout,
	})
	return predictive.NewScorer(p, predictive.Config{Version: manifest.ModelVersion}), schedulerStatus{AnomalyStatus: predictorStatusUnavailable, AnomalyReason: predictorReasonSignal}
}

func newHTTPMux(reg *prometheus.Registry, statuses ...schedulerStatus) http.Handler {
	status := schedulerStatus{}
	if len(statuses) > 0 {
		status = statuses[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return mux
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
