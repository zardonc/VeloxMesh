package gateway

import (
	"context"
	"time"

	"veloxmesh/internal/observability"
)

type providerOutcome struct {
	provider, model string
	latency         time.Duration
	err             error
}

func (s *Service) recordProviderOutcome(ctx context.Context, result providerOutcome) {
	finish := observability.Stage(ctx, "health_provider_sync")
	s.healthStore.EndRequest(result.provider, result.latency, result.err)
	finish()
	finish = observability.Stage(ctx, "circuit_result")
	s.cb.RecordResult(result.provider, result.err == nil)
	finish()
	finish = observability.Stage(ctx, "health_model_sync")
	s.healthStore.RecordModelOutcome(result.provider, result.model, result.err == nil)
	finish()
}
