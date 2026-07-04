package scheduler

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"veloxmesh/internal/config"
	"veloxmesh/internal/scheduler/schedulerv1"
)

type FIFOScorer struct {
	Reason string
}

func (s FIFOScorer) Score(_ context.Context, tasks []TaskFeature) ([]ScoreResult, error) {
	results := make([]ScoreResult, len(tasks))
	for i, task := range tasks {
		results[i] = fifoScore(task, s.Reason)
	}
	return results, nil
}

type GRPCScorer struct {
	enabled bool
	timeout time.Duration
	client  schedulerv1.TaskSchedulerClient
	conn    *grpc.ClientConn
	breaker *breaker
}

func NewScorer(ctx context.Context, cfg config.SchedulerConfig) (Scorer, error) {
	if !cfg.Enabled || cfg.Endpoint == "" {
		return FIFOScorer{Reason: "disabled"}, nil
	}
	return NewGRPCScorer(ctx, cfg)
}

func NewGRPCScorer(ctx context.Context, cfg config.SchedulerConfig) (*GRPCScorer, error) {
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Millisecond
	}
	conn, err := grpc.DialContext(ctx, cfg.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	threshold := cfg.BreakerFailureThreshold
	if threshold < 1 {
		threshold = 3
	}
	recovery, _ := time.ParseDuration(cfg.BreakerRecoveryTimeout)
	if recovery <= 0 {
		recovery = time.Minute
	}

	return &GRPCScorer{
		enabled: cfg.Enabled,
		timeout: timeout,
		client:  schedulerv1.NewTaskSchedulerClient(conn),
		conn:    conn,
		breaker: &breaker{threshold: threshold, recovery: recovery},
	}, nil
}

func (s *GRPCScorer) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *GRPCScorer) Score(ctx context.Context, tasks []TaskFeature) ([]ScoreResult, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	if !s.enabled {
		return fallback(tasks, "disabled"), nil
	}
	if !s.breaker.Allow() {
		return fallback(tasks, "breaker_open"), nil
	}

	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req := &schedulerv1.BatchScoreRequest{Tasks: make([]*schedulerv1.TaskFeature, 0, len(tasks))}
	for _, task := range tasks {
		req.Tasks = append(req.Tasks, task.proto())
	}

	resp, err := s.client.BatchScoreTasks(callCtx, req)
	if err != nil {
		s.breaker.Record(false)
		return fallback(tasks, fallbackReason(err)), nil
	}

	results := mergeResults(tasks, resp)
	success := true
	for _, result := range results {
		if result.FallbackReason != "" {
			success = false
			break
		}
	}
	s.breaker.Record(success)
	return results, nil
}

func mergeResults(tasks []TaskFeature, resp *schedulerv1.BatchScoreResponse) []ScoreResult {
	byID := map[string]*schedulerv1.ScoreResult{}
	if resp != nil {
		for _, result := range resp.GetResults() {
			if result.GetTaskId() != "" {
				byID[result.GetTaskId()] = result
			}
		}
	}

	results := make([]ScoreResult, len(tasks))
	for i, task := range tasks {
		if result, ok := byID[task.TaskID]; ok {
			results[i] = scoreFromProto(result)
			continue
		}
		results[i] = fifoScore(task, "missing_score")
	}
	return results
}

func fallback(tasks []TaskFeature, reason string) []ScoreResult {
	results, _ := FIFOScorer{Reason: reason}.Score(context.Background(), tasks)
	return results
}

func fifoScore(task TaskFeature, reason string) ScoreResult {
	return ScoreResult{
		TaskID:         task.TaskID,
		Score:          float64(task.EnqueueTimeMs),
		Priority:       task.Priority,
		Confidence:     1,
		FallbackReason: reason,
	}
}

func fallbackReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if status.Code(err) == codes.DeadlineExceeded {
		return "timeout"
	}
	return "scheduler_error"
}

type breaker struct {
	failures  int
	openedAt  time.Time
	threshold int
	recovery  time.Duration
}

func (b *breaker) Allow() bool {
	if b.openedAt.IsZero() {
		return true
	}
	return time.Since(b.openedAt) >= b.recovery
}

func (b *breaker) Record(success bool) {
	if success {
		b.failures = 0
		b.openedAt = time.Time{}
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.openedAt = time.Now()
	}
}
