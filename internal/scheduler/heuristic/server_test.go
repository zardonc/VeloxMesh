package heuristic

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"veloxmesh/internal/scheduler/schedulerv1"
)

func TestBatchScoreServiceReturnsOneResultPerTask(t *testing.T) {
	addr, stop := startRealScheduler(t)
	defer stop()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer conn.Close()

	resp, err := schedulerv1.NewTaskSchedulerClient(conn).BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{
		{TaskId: "t1", Priority: "high", RequestKind: "code_gen", EnqueueTimeMs: 1000, EstimatedInputTokens: 100, EstimatedOutputTokens: 100},
		{TaskId: "t2", Priority: "low", RequestKind: "simple_qa", EnqueueTimeMs: 1000, EstimatedInputTokens: 100, EstimatedOutputTokens: 100},
	}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	if len(resp.GetResults()) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.GetResults()))
	}
}

func TestBatchScoreServiceBoundsUnknownEnums(t *testing.T) {
	service := NewBatchScoreService(nil, nil)
	resp, err := service.BatchScoreTasks(context.Background(), &schedulerv1.BatchScoreRequest{Tasks: []*schedulerv1.TaskFeature{{TaskId: "t1", Priority: "nope", RequestKind: "wat"}}})
	if err != nil {
		t.Fatalf("BatchScoreTasks: %v", err)
	}
	if resp.GetResults()[0].GetPriority() != "normal" {
		t.Fatalf("expected bounded normal priority, got %s", resp.GetResults()[0].GetPriority())
	}
}

func startRealScheduler(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	schedulerv1.RegisterTaskSchedulerServer(server, NewBatchScoreService(nil, nil))
	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String(), server.Stop
}
