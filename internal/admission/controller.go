package admission

import (
	"context"
	"fmt"
	"strings"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
)

type ReleaseFunc func()

type AdmissionDecision struct {
	PriorityClass string
	QueueWaitMs   int64
}

type Controller interface {
	Admit(ctx context.Context, req *llm.LLMRequest, route routing.RoutingDecision) (ReleaseFunc, AdmissionDecision, error)
}

type PassThroughController struct{}

func NewPassThroughController() *PassThroughController {
	return &PassThroughController{}
}

func (c *PassThroughController) Admit(ctx context.Context, req *llm.LLMRequest, route routing.RoutingDecision) (ReleaseFunc, AdmissionDecision, error) {
	priority := strings.ToLower(req.PriorityClass)
	if priority == "" {
		priority = "interactive"
	}

	if priority != "interactive" && priority != "batch" && priority != "background" {
		return nil, AdmissionDecision{}, fmt.Errorf("invalid priority class: %s", priority)
	}

	// Phase 1: Pass-through admission, wait is 0, release does nothing.
	return func() {}, AdmissionDecision{
		PriorityClass: priority,
		QueueWaitMs:   0,
	}, nil
}
