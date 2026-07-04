package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"veloxmesh/internal/scheduler/heuristic"
	scheduleronnx "veloxmesh/internal/scheduler/onnx"
	"veloxmesh/internal/scheduler/schedulerv1"
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
	service, err := newSchedulerService(getenv("SCHEDULER_MODE", "heuristic"), getenv("SCHEDULER_ONNX_ARTIFACT_DIR", ""), metrics)
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

	httpServer := &http.Server{Addr: httpAddr, Handler: newHTTPMux(reg)}
	go func() { _ = httpServer.ListenAndServe() }()
	defer httpServer.Shutdown(ctx)

	<-ctx.Done()
	return ctx.Err()
}

func newSchedulerService(mode, artifactDir string, metrics *heuristic.Metrics) (schedulerv1.TaskSchedulerServer, error) {
	if mode == "" || mode == "heuristic" {
		return heuristic.NewBatchScoreService(nil, metrics), nil
	}
	if mode != "onnx" {
		return nil, fmt.Errorf("unsupported scheduler mode: %s", mode)
	}
	scorer, err := scheduleronnx.NewScorer(artifactDir)
	if err != nil {
		return nil, fmt.Errorf("start ONNX scheduler: %w", err)
	}
	return scheduleronnx.NewBatchScoreService(scorer), nil
}

func newHTTPMux(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
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
