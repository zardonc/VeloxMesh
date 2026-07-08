package scheduler

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"veloxmesh/internal/config"
	"veloxmesh/internal/scheduler/schedulerv1"
)

func TestDisabledScorerDoesNotDialAndUsesFIFO(t *testing.T) {
	scorer, err := NewScorer(context.Background(), config.SchedulerConfig{Enabled: false, Endpoint: "127.0.0.1:1", Timeout: "15ms"})
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 42, Priority: PriorityNormal}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].Score != 42 || results[0].FallbackReason != "disabled" || results[0].SchedulerType != SchedulerTypeFIFO {
		t.Fatalf("expected disabled FIFO fallback, got %#v", results[0])
	}
}

func TestGRPCScorerCallsRealSchedulerOverTCP(t *testing.T) {
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			return &schedulerv1.BatchScoreResponse{Results: []*schedulerv1.ScoreResult{{
				TaskId: req.GetTasks()[0].GetTaskId(), Score: 12.5, Priority: "high", Confidence: 0.9, SchedulerVersion: "test-scheduler",
			}}}, nil
		},
	})
	defer stop()

	scorer := newTCPScorer(t, endpoint, 15*time.Millisecond)
	defer scorer.Close()

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 7, Priority: PriorityHigh}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].Score != 12.5 || results[0].SchedulerVersion != "test-scheduler" || results[0].SchedulerType != SchedulerTypeHeuristic || results[0].FallbackReason != "" {
		t.Fatalf("expected scheduler score from real TCP call, got %#v", results[0])
	}
}

func TestGRPCScorerTimeoutFallsBackToFIFO(t *testing.T) {
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(ctx context.Context, _ *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	defer stop()

	scorer := newTCPScorer(t, endpoint, 15*time.Millisecond)
	defer scorer.Close()

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 7, Priority: PriorityHigh}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].Score != 7 || results[0].FallbackReason != "timeout" || results[0].SchedulerType != SchedulerTypeFIFO {
		t.Fatalf("expected timeout FIFO fallback, got %#v", results[0])
	}
}

func TestScoreWithDefaultTypePreventsEmptyQualityMetadata(t *testing.T) {
	score := scoreWithDefaultType(ScoreResult{})
	if score.SchedulerType != SchedulerTypeFIFO {
		t.Fatalf("expected FIFO default type, got %#v", score)
	}
}

func TestGRPCScorerBreakerOpenSkipsSecondNetworkCall(t *testing.T) {
	var calls int32
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(context.Context, *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			atomic.AddInt32(&calls, 1)
			return nil, status.Error(codes.Unavailable, "scheduler down")
		},
	})
	defer stop()

	scorer := newTCPScorerWithConfig(t, endpoint, config.SchedulerConfig{
		Enabled:                 true,
		Endpoint:                endpoint,
		Timeout:                 "15ms",
		BreakerFailureThreshold: 1,
		BreakerRecoveryTimeout:  "1m",
	})
	defer scorer.Close()

	task := []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 9, Priority: PriorityLow}}
	if _, err := scorer.Score(context.Background(), task); err != nil {
		t.Fatalf("first Score: %v", err)
	}
	results, err := scorer.Score(context.Background(), task)
	if err != nil {
		t.Fatalf("second Score: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected breaker to skip second TCP call, got %d calls", calls)
	}
	if results[0].FallbackReason != "breaker_open" {
		t.Fatalf("expected breaker fallback, got %#v", results[0])
	}
}

func TestGRPCScorerBreakerUsesWindowInsteadOfSingleSuccessReset(t *testing.T) {
	breaker := newBreaker(3, time.Minute)
	breaker.Record(false)
	breaker.Record(true)
	breaker.Record(false)

	if breaker.State() != "open" {
		t.Fatalf("expected error-rate window to open breaker, got %s", breaker.State())
	}
}

func TestGRPCScorerSlowSuccessFallsBackAndOpensBreaker(t *testing.T) {
	var calls int32
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond)
			return schedulerResponse(req, "slow-v1"), nil
		},
	})
	defer stop()

	scorer := newTCPScorerWithConfig(t, endpoint, config.SchedulerConfig{
		Enabled:                 true,
		Endpoint:                endpoint,
		Timeout:                 "100ms",
		ScorerSlowThreshold:     "5ms",
		BreakerFailureThreshold: 1,
		BreakerRecoveryTimeout:  "1m",
	})
	defer scorer.Close()

	task := []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 11, Priority: PriorityNormal}}
	first, err := scorer.Score(context.Background(), task)
	if err != nil {
		t.Fatalf("first Score: %v", err)
	}
	second, err := scorer.Score(context.Background(), task)
	if err != nil {
		t.Fatalf("second Score: %v", err)
	}
	if first[0].FallbackReason != "scorer_slow" || second[0].FallbackReason != "breaker_open" {
		t.Fatalf("expected slow then breaker fallback, got %#v %#v", first[0], second[0])
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected breaker to skip second TCP call, got %d calls", calls)
	}
}

func TestGRPCScorerConcurrencyLimitFallsBackWithoutWaiting(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	var calls int32
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			atomic.AddInt32(&calls, 1)
			closeOnce(entered)
			<-release
			return schedulerResponse(req, "v1"), nil
		},
	})
	defer stop()

	scorer := newTCPScorerWithConfig(t, endpoint, config.SchedulerConfig{
		Enabled:                 true,
		Endpoint:                endpoint,
		Timeout:                 "200ms",
		ScorerMaxConcurrency:    1,
		ScorerSlowThreshold:     "200ms",
		BreakerFailureThreshold: 3,
		BreakerRecoveryTimeout:  "1m",
	})
	defer scorer.Close()

	firstDone := make(chan struct{})
	go func() {
		_, _ = scorer.Score(context.Background(), []TaskFeature{{TaskID: "held", Priority: PriorityNormal}})
		close(firstDone)
	}()
	<-entered

	start := time.Now()
	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "busy", Priority: PriorityNormal}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("expected busy fallback without waiting, took %s", elapsed)
	}
	if results[0].FallbackReason != "scorer_busy" {
		t.Fatalf("expected scorer_busy fallback, got %#v", results[0])
	}
	close(release)
	<-firstDone
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected only held call to reach server, got %d", calls)
	}
}

func TestGRPCScorerMissingTaskIDsFallBackPerTask(t *testing.T) {
	endpoint, stop := startSchedulerServer(t, schedulerServer{
		score: func(context.Context, *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			return &schedulerv1.BatchScoreResponse{Results: []*schedulerv1.ScoreResult{{TaskId: "t1", Score: 1.5, Priority: "high"}}}, nil
		},
	})
	defer stop()

	scorer := newTCPScorer(t, endpoint, 15*time.Millisecond)
	defer scorer.Close()

	results, err := scorer.Score(context.Background(), []TaskFeature{
		{TaskID: "t1", EnqueueTimeMs: 10, Priority: PriorityHigh},
		{TaskID: "t2", EnqueueTimeMs: 20, Priority: PriorityNormal},
	})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].Score != 1.5 || results[0].FallbackReason != "" {
		t.Fatalf("expected scheduler score for t1, got %#v", results[0])
	}
	if results[1].Score != 20 || results[1].FallbackReason != "missing_score" {
		t.Fatalf("expected per-task FIFO fallback for t2, got %#v", results[1])
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func TestWeightedScorerRolloutZeroNeverCallsONNX(t *testing.T) {
	heuristic := &recordingScorer{result: ScoreResult{SchedulerVersion: "heuristic-v1"}}
	onnx := &recordingScorer{result: ScoreResult{SchedulerVersion: "onnx-v1"}}
	scorer := WeightedScorer{Heuristic: heuristic, ONNX: onnx, ONNXRolloutPercent: 0}

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", Priority: PriorityNormal}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if onnx.calls != 0 || heuristic.calls != 1 {
		t.Fatalf("unexpected calls: heuristic=%d onnx=%d", heuristic.calls, onnx.calls)
	}
	if results[0].SchedulerType != SchedulerTypeHeuristic {
		t.Fatalf("expected heuristic score, got %#v", results[0])
	}
}

func TestNewScorerWithControllerKeepsONNXAvailableAtZeroRollout(t *testing.T) {
	heuristicEndpoint, stopHeuristic := startSchedulerServer(t, schedulerServer{
		score: func(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			return schedulerResponse(req, "heuristic-v1"), nil
		},
	})
	defer stopHeuristic()
	onnxEndpoint, stopONNX := startSchedulerServer(t, schedulerServer{
		score: func(_ context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
			return schedulerResponse(req, "onnx-v1"), nil
		},
	})
	defer stopONNX()

	cfg := config.SchedulerConfig{Enabled: true, HeuristicEndpoint: heuristicEndpoint, ONNXEndpoint: onnxEndpoint, ONNXRolloutPercent: 0, Timeout: "15ms", BreakerFailureThreshold: 3, BreakerRecoveryTimeout: "1m"}
	controller := NewSchedulerRolloutController(cfg)
	scorer, err := NewScorerWithController(context.Background(), cfg, controller)
	if err != nil {
		t.Fatalf("NewScorerWithController: %v", err)
	}
	defer closeWeightedScorer(t, scorer)

	if _, err := controller.SetONNXRolloutPercent(100); err != nil {
		t.Fatalf("set rollout percent: %v", err)
	}
	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", Priority: PriorityNormal}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].SchedulerType != SchedulerTypeONNX || results[0].SchedulerVersion != "onnx-v1" {
		t.Fatalf("expected runtime rollout to use ONNX, got %#v", results[0])
	}
}

func TestWeightedScorerRolloutHundredCallsONNXForAllTasks(t *testing.T) {
	heuristic := &recordingScorer{result: ScoreResult{SchedulerVersion: "heuristic-v1"}}
	onnx := &recordingScorer{result: ScoreResult{SchedulerVersion: "onnx-v1"}}
	scorer := WeightedScorer{Heuristic: heuristic, ONNX: onnx, ONNXRolloutPercent: 100}

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1"}, {TaskID: "t2"}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if onnx.calls != 1 || heuristic.calls != 0 {
		t.Fatalf("unexpected calls: heuristic=%d onnx=%d", heuristic.calls, onnx.calls)
	}
	if results[0].SchedulerType != SchedulerTypeONNX || results[1].SchedulerType != SchedulerTypeONNX {
		t.Fatalf("expected ONNX scores, got %#v", results)
	}
}

func TestWeightedScorerONNXFailureFallsBackToHeuristicThenFIFO(t *testing.T) {
	heuristic := &recordingScorer{result: ScoreResult{SchedulerVersion: "heuristic-v1"}}
	onnx := &recordingScorer{result: ScoreResult{FallbackReason: "scheduler_error"}}
	scorer := WeightedScorer{Heuristic: heuristic, ONNX: onnx, ONNXRolloutPercent: 100}

	results, err := scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 42}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if heuristic.calls != 1 || results[0].SchedulerType != SchedulerTypeHeuristic {
		t.Fatalf("expected heuristic fallback, calls=%d result=%#v", heuristic.calls, results[0])
	}

	heuristic.result = ScoreResult{FallbackReason: "scheduler_error"}
	results, err = scorer.Score(context.Background(), []TaskFeature{{TaskID: "t1", EnqueueTimeMs: 42}})
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if results[0].SchedulerType != SchedulerTypeFIFO || results[0].FallbackReason != "onnx_then_heuristic_failed" {
		t.Fatalf("expected FIFO fallback, got %#v", results[0])
	}
}

func TestRolloutAssignmentUsesTaskID(t *testing.T) {
	task := TaskFeature{TaskID: "stable", EnqueueTimeMs: 1}
	if rolloutBucket(task, 0) != rolloutBucket(TaskFeature{TaskID: "stable", EnqueueTimeMs: 999}, 7) {
		t.Fatalf("expected task ID to determine rollout bucket")
	}
	if rolloutBucket(TaskFeature{EnqueueTimeMs: 1}, 0) == rolloutBucket(TaskFeature{EnqueueTimeMs: 1}, 1) {
		t.Fatalf("expected empty task IDs to use index and enqueue time fallback")
	}
}

type recordingScorer struct {
	calls  int
	result ScoreResult
}

func (s *recordingScorer) Score(_ context.Context, tasks []TaskFeature) ([]ScoreResult, error) {
	s.calls++
	results := make([]ScoreResult, len(tasks))
	for i, task := range tasks {
		result := s.result
		result.TaskID = task.TaskID
		results[i] = result
	}
	return results, nil
}

type schedulerServer struct {
	schedulerv1.UnimplementedTaskSchedulerServer
	score func(context.Context, *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error)
}

func (s schedulerServer) BatchScoreTasks(ctx context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
	return s.score(ctx, req)
}

func schedulerResponse(req *schedulerv1.BatchScoreRequest, version string) *schedulerv1.BatchScoreResponse {
	results := make([]*schedulerv1.ScoreResult, 0, len(req.GetTasks()))
	for _, task := range req.GetTasks() {
		results = append(results, &schedulerv1.ScoreResult{TaskId: task.GetTaskId(), Score: 12.5, Priority: task.GetPriority(), Confidence: 0.9, SchedulerVersion: version})
	}
	return &schedulerv1.BatchScoreResponse{Results: results}
}

func closeWeightedScorer(t *testing.T, scorer Scorer) {
	t.Helper()
	weighted, ok := scorer.(WeightedScorer)
	if !ok {
		return
	}
	for _, candidate := range []Scorer{weighted.Heuristic, weighted.ONNX} {
		grpcScorer, ok := candidate.(*GRPCScorer)
		if ok {
			_ = grpcScorer.Close()
		}
	}
}

func startSchedulerServer(t *testing.T, srv schedulerServer) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	schedulerv1.RegisterTaskSchedulerServer(server, srv)
	go func() {
		_ = server.Serve(listener)
	}()
	return listener.Addr().String(), server.Stop
}

func newTCPScorer(t *testing.T, endpoint string, timeout time.Duration) *GRPCScorer {
	t.Helper()
	return newTCPScorerWithConfig(t, endpoint, config.SchedulerConfig{
		Enabled:                 true,
		Endpoint:                endpoint,
		Timeout:                 timeout.String(),
		BreakerFailureThreshold: 3,
		BreakerRecoveryTimeout:  "1m",
	})
}

func newTCPScorerWithConfig(t *testing.T, endpoint string, cfg config.SchedulerConfig) *GRPCScorer {
	t.Helper()
	cfg.Enabled = true
	cfg.Endpoint = endpoint
	scorer, err := NewGRPCScorer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewGRPCScorer: %v", err)
	}
	return scorer
}
