package predictor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"veloxmesh/internal/scheduler"
	"veloxmesh/internal/scheduler/predictorv1"
)

var ErrBreakerOpen = errors.New("predictor breaker open")

type PythonClientConfig struct {
	Endpoint                string
	Timeout                 time.Duration
	BreakerFailureThreshold int
	BreakerRecoveryTimeout  time.Duration
}

type PythonONNXPredictorClient struct {
	timeout time.Duration
	conn    *grpc.ClientConn
	client  predictorv1.OutputTokenPredictorClient
	breaker *clientBreaker
}

func NewPythonONNXPredictor(ctx context.Context, cfg PythonClientConfig) (OutputTokenPredictor, error) {
	client, err := NewPythonONNXPredictorClient(ctx, cfg)
	if err != nil {
		return NoopPredictor{Reason: err}, nil
	}
	return client, nil
}

func NewPythonONNXPredictorClient(ctx context.Context, cfg PythonClientConfig) (*PythonONNXPredictorClient, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("predictor endpoint is required")
	}
	conn, err := grpc.NewClient(cfg.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := predictorv1.NewOutputTokenPredictorClient(conn)
	if err := checkHealth(ctx, client, timeoutOrDefault(cfg.Timeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &PythonONNXPredictorClient{timeout: timeoutOrDefault(cfg.Timeout), conn: conn, client: client, breaker: newClientBreaker(cfg)}, nil
}

func (c *PythonONNXPredictorClient) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *PythonONNXPredictorClient) Predict(ctx context.Context, tasks []scheduler.TaskFeature) ([]Prediction, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	if !c.breaker.Allow() {
		return NoopPredictor{Reason: ErrBreakerOpen}.Predict(ctx, tasks)
	}
	resp, err := c.batchPredict(ctx, tasks)
	if err != nil {
		c.breaker.Record(false)
		return NoopPredictor{Reason: err}.Predict(ctx, tasks)
	}
	predictions, err := predictionsFromProto(resp, len(tasks))
	c.breaker.Record(err == nil)
	if err != nil {
		return NoopPredictor{Reason: err}.Predict(ctx, tasks)
	}
	return predictions, nil
}

func (c *PythonONNXPredictorClient) batchPredict(ctx context.Context, tasks []scheduler.TaskFeature) (*predictorv1.BatchPredictResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req := &predictorv1.BatchPredictRequest{Tasks: make([]*predictorv1.TaskFeature, 0, len(tasks))}
	for _, task := range tasks {
		req.Tasks = append(req.Tasks, taskToProto(task))
	}
	return c.client.BatchPredict(callCtx, req)
}

func checkHealth(ctx context.Context, client predictorv1.OutputTokenPredictorClient, timeout time.Duration) error {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	health, err := client.Health(callCtx, &predictorv1.HealthRequest{})
	if err != nil {
		return fmt.Errorf("predictor health: %w", err)
	}
	if !health.GetReady() {
		return fmt.Errorf("predictor health: %s", health.GetReason())
	}
	return nil
}
