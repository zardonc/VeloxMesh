package scheduler

import (
	"context"
	"errors"
)

var (
	ErrQueueFull    = errors.New("queue full")
	ErrQueueEmpty   = errors.New("queue empty")
	ErrTaskNotFound = errors.New("task not found")
)

type QueueItem struct {
	TaskID string
	Score  float64
}

type QueueBackend interface {
	Push(ctx context.Context, item QueueItem) error
	PeekMin(ctx context.Context, limit int) ([]QueueItem, error)
	PopMin(ctx context.Context) (QueueItem, error)
	Remove(ctx context.Context, taskID string) error
	Len(ctx context.Context) (int64, error)
}
