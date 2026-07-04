package scheduler

import "time"

type TaskState string

const (
	TaskStateQueued    TaskState = "queued"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateCanceled  TaskState = "canceled"
	TaskStateFailed    TaskState = "failed"
)

type Task struct {
	ID          string
	Feature     TaskFeature
	Score       float64
	EnqueueTime time.Time
	Deadline    time.Time
	Attempts    int
	State       TaskState
	Metadata    map[string]string
}

type TaskResult struct {
	Response any
	Error    error
}
