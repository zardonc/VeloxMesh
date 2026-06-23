package admission

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/http/middleware"
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

type CreditAdmissionController struct {
	repo controlstate.Repository
}

func NewCreditAdmissionController(repo controlstate.Repository) *CreditAdmissionController {
	return &CreditAdmissionController{repo: repo}
}

func (c *CreditAdmissionController) Admit(ctx context.Context, req *llm.LLMRequest, route routing.RoutingDecision) (ReleaseFunc, AdmissionDecision, error) {
	priority := strings.ToLower(req.PriorityClass)
	if priority == "" {
		priority = "interactive"
	}

	if priority != "interactive" && priority != "batch" && priority != "background" {
		return nil, AdmissionDecision{}, fmt.Errorf("invalid priority class: %s", priority)
	}

	identity := middleware.GetAuthIdentity(ctx)
	if identity == nil {
		return nil, AdmissionDecision{}, errors.NewGatewayError("unauthorized", "Missing authentication identity", http.StatusUnauthorized)
	}

	if identity.ID == "dev-key" {
		return func() {}, AdmissionDecision{
			PriorityClass: priority,
			QueueWaitMs:   0,
		}, nil
	}

	if identity.CreditBalance <= 0 {
		err := errors.NewGatewayError("insufficient_credits", "Insufficient credits for request", http.StatusTooManyRequests)
		err.Headers = map[string]string{
			"X-RateLimit-Remaining-Tokens": "0",
		}
		return nil, AdmissionDecision{}, err
	}

	return func() {}, AdmissionDecision{
		PriorityClass: priority,
		QueueWaitMs:   0,
	}, nil
}
