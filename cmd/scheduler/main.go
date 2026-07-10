package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	predictorStatusReady           = "ready"
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
	serveErrs := make(chan error, 2)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErrs <- fmt.Errorf("grpc serve: %w", err)
		}
	}()
	defer grpcServer.Stop()

	httpServer := &http.Server{Addr: httpAddr, Handler: newHTTPMux(reg, status)}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrs <- fmt.Errorf("http serve: %w", err)
		}
	}()
	defer httpServer.Shutdown(ctx)

	select {
	case err := <-serveErrs:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
		cfg, err := heuristic.LoadConfigFile(getenv("SCHEDULER_HEURISTIC_CONFIG_FILE", ""), heuristic.DefaultConfig())
		if err != nil {
			return nil, schedulerStatus{}, err
		}
		return heuristic.NewBatchScoreService(heuristic.NewScoreCalculator(cfg), metrics), schedulerStatus{}, nil
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
		Endpoint:       getenv("SCHEDULER_PREDICTOR_ENDPOINT", ""),
		Timeout:        defaultPredictorTimeout,
		MaxConcurrency: getenvInt("SCHEDULER_SCORER_MAX_CONCURRENCY", 4),
		SlowThreshold:  getenvDuration("SCHEDULER_SCORER_SLOW_THRESHOLD", defaultPredictorTimeout),
	})
	if isNoopPredictor(p) {
		return predictive.NewScorer(p, predictive.Config{Version: manifest.ModelVersion}), schedulerStatus{AnomalyStatus: predictorStatusUnavailable, AnomalyReason: predictorReasonSignal}
	}
	return predictive.NewScorer(p, predictive.Config{Version: manifest.ModelVersion}), schedulerStatus{AnomalyStatus: predictorStatusReady}
}

func isNoopPredictor(p predictor.OutputTokenPredictor) bool {
	switch p.(type) {
	case predictor.NoopPredictor, *predictor.NoopPredictor:
		return true
	default:
		return false
	}
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

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
