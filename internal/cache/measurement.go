package cache

import (
	"time"
	"veloxmesh/internal/observability"
)

// The optional observer also lets isolated preflights retain exact operation samples.
func measureOperation(name string, started time.Time, err error) {
	if observer, ok := observability.DefaultMetrics.(interface {
		RecordCacheOperation(string, time.Duration, error)
	}); ok {
		observer.RecordCacheOperation(name, time.Since(started), err)
	}
}
