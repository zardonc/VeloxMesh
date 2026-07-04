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
	if results[0].Score != 42 || results[0].FallbackReason != "disabled" {
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
	if results[0].Score != 12.5 || results[0].SchedulerVersion != "test-scheduler" || results[0].FallbackReason != "" {
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
	if results[0].Score != 7 || results[0].FallbackReason != "timeout" {
		t.Fatalf("expected timeout FIFO fallback, got %#v", results[0])
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

type schedulerServer struct {
	schedulerv1.UnimplementedTaskSchedulerServer
	score func(context.Context, *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error)
}

func (s schedulerServer) BatchScoreTasks(ctx context.Context, req *schedulerv1.BatchScoreRequest) (*schedulerv1.BatchScoreResponse, error) {
	return s.score(ctx, req)
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
