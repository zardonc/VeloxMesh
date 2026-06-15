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
}
