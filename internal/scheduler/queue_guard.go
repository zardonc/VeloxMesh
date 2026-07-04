package scheduler

import "context"

type QueueGuard struct {
	SoftLimit int64
	HardLimit int64
}

type QueueGuardResult struct {
	Allowed   bool
	Throttled bool
	Err       error
}

func (g QueueGuard) Check(ctx context.Context, backend QueueBackend, _ PriorityClass) QueueGuardResult {
	length, err := backend.Len(ctx)
	if err != nil {
		return QueueGuardResult{Err: err}
	}
	if g.HardLimit > 0 && length >= g.HardLimit {
		return QueueGuardResult{Err: ErrQueueFull}
	}
	if g.SoftLimit > 0 && length >= g.SoftLimit {
		return QueueGuardResult{Allowed: true, Throttled: true}
	}
	return QueueGuardResult{Allowed: true}
}
