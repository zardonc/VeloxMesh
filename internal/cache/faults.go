package cache

import (
	"errors"
	"log/slog"
	"math"
	"veloxmesh/internal/observability"
)

func (s *SemanticCacheService) fault(operation, reason string, cause error) error {
	recordCacheOutcome(operation, reason)
	if cause == nil {
		return errors.New(reason)
	}
	slog.Error("semantic cache operation failed", "operation", operation, "reason", reason, "error", cause)
	return errors.Join(errors.New(reason), cause)
}

func recordCacheOutcome(operation, reason string) {
	if metrics, ok := observability.DefaultMetrics.(interface{ RecordCacheOutcome(string, string) }); ok {
		metrics.RecordCacheOutcome(operation, reason)
	}
}

func validVector(values []float32, dimension int) bool {
	if dimension <= 0 || len(values) != dimension {
		return false
	}
	var norm float64
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		norm += float64(value) * float64(value)
	}
	return norm > 0
}
