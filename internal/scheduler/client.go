package scheduler

import (
	"context"
	"errors"
	"hash/fnv"
	"strconv"
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
	enabled       bool
	timeout       time.Duration
	client        schedulerv1.TaskSchedulerClient
	conn          *grpc.ClientConn
	breaker       *breaker
	schedulerType SchedulerType
}

func NewScorer(ctx context.Context, cfg config.SchedulerConfig) (Scorer, error) {
	return NewScorerWithController(ctx, cfg, NewSchedulerRolloutController(cfg))
}

func NewScorerWithController(ctx context.Context, cfg config.SchedulerConfig, controller *SchedulerRolloutController) (Scorer, error) {
	heuristicEndpoint := cfg.HeuristicEndpoint
	if heuristicEndpoint == "" {
		heuristicEndpoint = cfg.Endpoint
	}
	if !cfg.Enabled || heuristicEndpoint == "" {
		return FIFOScorer{Reason: "disabled"}, nil
	}
	heuristicCfg := cfg
	heuristicCfg.Endpoint = heuristicEndpoint
	heuristic, err := NewGRPCScorer(ctx, heuristicCfg)
	if err != nil || cfg.ONNXEndpoint == "" {
		return heuristic, err
	}
	onnxCfg := cfg
	onnxCfg.Endpoint = cfg.ONNXEndpoint
	onnx, err := newGRPCScorer(ctx, onnxCfg, SchedulerTypeONNX)
	if err != nil {
		_ = heuristic.Close()
		return nil, err
	}
	return WeightedScorer{Heuristic: heuristic, ONNX: onnx, Controller: controller}, nil
}

func NewGRPCScorer(ctx context.Context, cfg config.SchedulerConfig) (*GRPCScorer, error) {
	return newGRPCScorer(ctx, cfg, SchedulerTypeHeuristic)
}

func newGRPCScorer(ctx context.Context, cfg config.SchedulerConfig, schedulerType SchedulerType) (*GRPCScorer, error) {
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
		enabled:       cfg.Enabled,
		timeout:       timeout,
		client:        schedulerv1.NewTaskSchedulerClient(conn),
		conn:          conn,
		breaker:       &breaker{threshold: threshold, recovery: recovery},
		schedulerType: schedulerType,
	}, nil
}

func (s *GRPCScorer) Close() error {
	if s.conn == nil {
		return nil
	}
	return s.conn.Close()
}

func (s *GRPCScorer) BreakerState() string {
	if s == nil || s.breaker == nil {
		return "unknown"
	}
	return s.breaker.State()
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
	for i, result := range results {
		results[i].SchedulerType = s.schedulerType
		if result.FallbackReason != "" {
			success = false
		}
	}
	s.breaker.Record(success)
	return results, nil
}

type WeightedScorer struct {
	Heuristic          Scorer
	ONNX               Scorer
	ONNXRolloutPercent int
	Controller         *SchedulerRolloutController
}

func (s WeightedScorer) Score(ctx context.Context, tasks []TaskFeature) ([]ScoreResult, error) {
	results := make([]ScoreResult, len(tasks))
	heuristicTasks, onnxTasks := splitByRollout(tasks, s.rolloutPercent())

	scoreIndexed(ctx, s.Heuristic, heuristicTasks, SchedulerTypeHeuristic, results, "heuristic_failed")
	scoreIndexed(ctx, s.ONNX, onnxTasks, SchedulerTypeONNX, results, "onnx_failed")

	for _, item := range onnxTasks {
		if !unusableScore(results[item.Index]) {
			continue
		}
		scoreIndexed(ctx, s.Heuristic, []indexedTask{item}, SchedulerTypeHeuristic, results, "onnx_then_heuristic_failed")
		if unusableScore(results[item.Index]) {
			results[item.Index] = fifoScore(item.Task, "onnx_then_heuristic_failed")
		}
	}
	return results, nil
}

func (s WeightedScorer) rolloutPercent() int {
	if s.Controller != nil {
		return s.Controller.RolloutPercent()
	}
	return s.ONNXRolloutPercent
}

func (s WeightedScorer) BreakerState() string {
	return "heuristic=" + scorerBreakerState(s.Heuristic) + ",onnx=" + scorerBreakerState(s.ONNX)
}

type indexedTask struct {
	Index int
	Task  TaskFeature
}

func splitByRollout(tasks []TaskFeature, percent int) ([]indexedTask, []indexedTask) {
	heuristicTasks, onnxTasks := make([]indexedTask, 0, len(tasks)), make([]indexedTask, 0, len(tasks))
	for i, task := range tasks {
		item := indexedTask{Index: i, Task: task}
		if assignedToONNX(task, i, percent) {
			onnxTasks = append(onnxTasks, item)
			continue
		}
		heuristicTasks = append(heuristicTasks, item)
	}
	return heuristicTasks, onnxTasks
}

func scoreIndexed(ctx context.Context, scorer Scorer, items []indexedTask, schedulerType SchedulerType, dst []ScoreResult, fallbackReason string) {
	if len(items) == 0 {
		return
	}
	tasks := make([]TaskFeature, len(items))
	for i, item := range items {
		tasks[i] = item.Task
	}
	results, err := scorer.Score(ctx, tasks)
	if err != nil || len(results) != len(items) {
		results = fallback(tasks, fallbackReason)
	}
	for i, result := range results {
		result.SchedulerType = schedulerType
		dst[items[i].Index] = result
	}
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
		SchedulerType:  SchedulerTypeFIFO,
		FallbackReason: reason,
	}
}

func assignedToONNX(task TaskFeature, index, percent int) bool {
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	return rolloutBucket(task, index) < uint32(percent*100)
}

func rolloutBucket(task TaskFeature, index int) uint32 {
	key := task.TaskID
	if key == "" {
		key = strconv.Itoa(index) + ":" + strconv.FormatInt(task.EnqueueTimeMs, 10)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() % 10000
}

func unusableScore(result ScoreResult) bool {
	switch result.FallbackReason {
	case "breaker_open", "disabled", "heuristic_failed", "missing_score", "onnx_failed", "scheduler_error", "timeout":
		return true
	default:
		return result.SchedulerVersion == "" && result.SchedulerType != SchedulerTypeFIFO
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

func (b *breaker) State() string {
	if b == nil {
		return "unknown"
	}
	if b.openedAt.IsZero() {
		return "closed"
	}
	if time.Since(b.openedAt) >= b.recovery {
		return "half_open"
	}
	return "open"
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

func scorerBreakerState(scorer Scorer) string {
	reporter, ok := scorer.(interface{ BreakerState() string })
	if !ok {
		return "unavailable"
	}
	return reporter.BreakerState()
}
