package observability

import (
	"context"
	"time"
)

type timingIDKey struct{}

type StageMeasurement struct {
	ID, Name string
	Started  time.Time
	Elapsed  time.Duration
}

// WithTimingID correlates optional diagnostics, including detached cache writes.
func WithTimingID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, timingIDKey{}, id)
}

func TimingID(ctx context.Context) string {
	id, _ := ctx.Value(timingIDKey{}).(string)
	return id
}

// Stage adds no exporter dependency and records no prompts or credentials.
// Observers are optional; normal metrics implementations do not enable this hook.
func Stage(ctx context.Context, name string) func() {
	observer, ok := DefaultMetrics.(interface {
		RecordStage(StageMeasurement)
	})
	if !ok {
		return func() {}
	}
	started := time.Now()
	id := TimingID(ctx)
	return func() {
		observer.RecordStage(StageMeasurement{ID: id, Name: name, Started: started, Elapsed: time.Since(started)})
	}
}
