package providers

import (
	"context"
	"veloxmesh/internal/llm"
)

type HealthStatus struct {
	Available bool
	Message   string
}

type ProviderAdapter interface {
	ID() string
	Models() []string
	Complete(ctx context.Context, req *llm.LLMRequest) (*llm.LLMResponse, error)
	HealthCheck(ctx context.Context) HealthStatus

	// Capabilities returns the provider-neutral capabilities of this adapter.
	Capabilities() CapabilitySet
}

type StreamAdapter interface {
	ProviderAdapter
	Stream(ctx context.Context, req *llm.LLMRequest) (<-chan llm.StreamEvent, error)
}
