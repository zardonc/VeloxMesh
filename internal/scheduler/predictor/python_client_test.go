package predictor

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/predictorv1"
)

func TestPythonPredictorDegradesWhenHealthUnavailable(t *testing.T) {
	predictor, err := NewPythonONNXPredictor(context.Background(), PythonClientConfig{Endpoint: "127.0.0.1:1", Timeout: time.Millisecond})
	if err != nil {
		t.Fatalf("NewPythonONNXPredictor: %v", err)
	}
	predictions, err := predictor.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(predictions) != 1 || predictions[0].Err == nil {
		t.Fatalf("expected degraded per-task error, got %#v", predictions)
	}
}

func TestPythonPredictorPreservesPartialFailures(t *testing.T) {
	server := startPredictorServer(t, func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error) {
		return &predictorv1.BatchPredictResponse{Predictions: []*predictorv1.Prediction{
			{ModelVersion: "v1", Error: "invalid_task"},
			{ModelVersion: "v1", Quantiles: map[int32]float64{70: 20}},
		}}, nil
	})
	client := newTestClient(t, server.Endpoint, time.Second, time.Millisecond)
	predictions, err := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "bad"}, {TaskID: "ok"}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if predictions[0].Err == nil || predictions[1].Quantiles[70] != 20 {
		t.Fatalf("unexpected predictions: %#v", predictions)
	}
}

func TestPythonPredictorBreakerSkipsAndRecovers(t *testing.T) {
	var calls atomic.Int32
	server := startPredictorServer(t, func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error) {
		if calls.Add(1) == 1 {
			time.Sleep(40 * time.Millisecond)
		}
		return &predictorv1.BatchPredictResponse{Predictions: []*predictorv1.Prediction{{ModelVersion: "v1", Quantiles: map[int32]float64{70: 20}}}}, nil
	})
	client := newTestClient(t, server.Endpoint, 5*time.Millisecond, 20*time.Millisecond)

	first, _ := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	second, _ := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if first[0].Err == nil || !errors.Is(second[0].Err, ErrBreakerOpen) {
		t.Fatalf("expected timeout then breaker_open, got %#v %#v", first, second)
	}
	if calls.Load() != 1 {
		t.Fatalf("breaker should skip second call, got %d calls", calls.Load())
	}
	time.Sleep(50 * time.Millisecond)
	third, _ := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if third[0].Err != nil || calls.Load() != 2 {
		t.Fatalf("expected recovered predictor call, got %#v calls=%d", third, calls.Load())
	}
}

func TestClientBreakerUsesWindowInsteadOfSingleSuccessReset(t *testing.T) {
	breaker := newClientBreaker(PythonClientConfig{
		BreakerFailureThreshold: 3,
		BreakerRecoveryTimeout:  time.Minute,
	})
	breaker.Record(false)
	breaker.Record(true)
	breaker.Record(false)

	if breaker.Allow() {
		t.Fatalf("expected error-rate window to open breaker")
	}
}

func TestPythonPredictorSlowSuccessFallsBackAndOpensBreaker(t *testing.T) {
	var calls atomic.Int32
	server := startPredictorServer(t, func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		return &predictorv1.BatchPredictResponse{Predictions: []*predictorv1.Prediction{{ModelVersion: "v1", Quantiles: map[int32]float64{70: 20}}}}, nil
	})
	client := newTestClientWithConfig(t, PythonClientConfig{
		Endpoint:                server.Endpoint,
		Timeout:                 100 * time.Millisecond,
		SlowThreshold:           5 * time.Millisecond,
		MaxConcurrency:          1,
		BreakerFailureThreshold: 1,
		BreakerRecoveryTimeout:  time.Minute,
	})

	first, _ := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	second, _ := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "t1"}})
	if !errors.Is(first[0].Err, ErrPredictorSlow) || !errors.Is(second[0].Err, ErrBreakerOpen) {
		t.Fatalf("expected slow then breaker_open, got %#v %#v", first, second)
	}
	if calls.Load() != 1 {
		t.Fatalf("breaker should skip second call, got %d calls", calls.Load())
	}
}

func TestPythonPredictorConcurrencyLimitFallsBackWithoutWaiting(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	var calls atomic.Int32
	server := startPredictorServer(t, func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error) {
		calls.Add(1)
		closeOnce(entered)
		<-release
		return &predictorv1.BatchPredictResponse{Predictions: []*predictorv1.Prediction{{ModelVersion: "v1", Quantiles: map[int32]float64{70: 20}}}}, nil
	})
	client := newTestClientWithConfig(t, PythonClientConfig{
		Endpoint:                server.Endpoint,
		Timeout:                 200 * time.Millisecond,
		SlowThreshold:           200 * time.Millisecond,
		MaxConcurrency:          1,
		BreakerFailureThreshold: 3,
		BreakerRecoveryTimeout:  time.Minute,
	})

	firstDone := make(chan struct{})
	go func() {
		_, _ = client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "held"}})
		close(firstDone)
	}()
	<-entered

	start := time.Now()
	predictions, err := client.Predict(context.Background(), []scheduler.TaskFeature{{TaskID: "busy"}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected busy fallback without waiting, took %s", elapsed)
	}
	if !errors.Is(predictions[0].Err, ErrPredictorBusy) {
		t.Fatalf("expected predictor busy fallback, got %#v", predictions)
	}
	close(release)
	<-firstDone
	if calls.Load() != 1 {
		t.Fatalf("expected only held call to reach server, got %d", calls.Load())
	}
}

type testServer struct {
	Endpoint string
	Stop     func()
}

type testPredictorServer struct {
	predictorv1.UnimplementedOutputTokenPredictorServer
	predict func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error)
}

func (s testPredictorServer) Health(context.Context, *predictorv1.HealthRequest) (*predictorv1.HealthResponse, error) {
	return &predictorv1.HealthResponse{Ready: true, ModelVersion: "v1"}, nil
}

func (s testPredictorServer) BatchPredict(ctx context.Context, req *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error) {
	return s.predict(ctx, req)
}

func startPredictorServer(t *testing.T, predict func(context.Context, *predictorv1.BatchPredictRequest) (*predictorv1.BatchPredictResponse, error)) testServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	predictorv1.RegisterOutputTokenPredictorServer(server, testPredictorServer{predict: predict})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return testServer{Endpoint: listener.Addr().String(), Stop: server.Stop}
}

func newTestClient(t *testing.T, endpoint string, timeout, recovery time.Duration) *PythonONNXPredictorClient {
	t.Helper()
	return newTestClientWithConfig(t, PythonClientConfig{
		Endpoint: endpoint, Timeout: timeout, BreakerFailureThreshold: 1, BreakerRecoveryTimeout: recovery,
	})
}

func newTestClientWithConfig(t *testing.T, cfg PythonClientConfig) *PythonONNXPredictorClient {
	t.Helper()
	client, err := NewPythonONNXPredictorClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPythonONNXPredictorClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}
