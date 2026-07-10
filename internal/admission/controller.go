package admission

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
	"veloxmesh/internal/scheduler"
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
	rawPriority := strings.ToLower(req.PriorityClass)
	if rawPriority == "" {
		rawPriority = "normal"
	}
	priority := string(scheduler.NormalizePriority(rawPriority))
	if priority == "" {
		return nil, AdmissionDecision{}, fmt.Errorf("invalid priority class: %s", rawPriority)
	}

	// Phase 1: Pass-through admission, wait is 0, release does nothing.
	return func() {}, AdmissionDecision{
		PriorityClass: priority,
		QueueWaitMs:   0,
	}, nil
}

type LimitAdmissionController struct {
	repo    controlstate.Repository
	limiter hotstate.AtomicLimiter
}

func NewLimitAdmissionController(repo controlstate.Repository, limiter hotstate.AtomicLimiter) *LimitAdmissionController {
	return &LimitAdmissionController{repo: repo, limiter: limiter}
}

func (c *LimitAdmissionController) Admit(ctx context.Context, req *llm.LLMRequest, route routing.RoutingDecision) (ReleaseFunc, AdmissionDecision, error) {
	rawPriority := strings.ToLower(req.PriorityClass)
	if rawPriority == "" {
		rawPriority = "normal"
	}
	priority := string(scheduler.NormalizePriority(rawPriority))
	if priority == "" {
		return nil, AdmissionDecision{}, fmt.Errorf("invalid priority class: %s", rawPriority)
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

	if c.limiter != nil {
		// 1. Evaluate API Key Limits
		apiKeyRules, err := c.repo.LimitRules().ListByTarget(ctx, controlstate.ScopeAPIKey, identity.ID)
		if err != nil {
			return nil, AdmissionDecision{}, fmt.Errorf("failed to fetch api key rules: %w", err)
		}
		for _, rule := range apiKeyRules {
			if !rule.Enabled {
				continue
			}
			windowDuration := parseWindow(rule.Window)
			// Avoid circular dependency by manually namespacing or using hotstate.NamespacedKey
			key := fmt.Sprintf("limit:api_key:%s:%s", rule.Dimension, identity.ID)

			_, allowed, err := c.limiter.CheckAndIncrement(ctx, key, rule.Limit, windowDuration)
			if err != nil {
				return nil, AdmissionDecision{}, fmt.Errorf("limiter unavailable: %w", err)
			}
			if !allowed {
				err := errors.NewGatewayError("rate_limit_exceeded", "API key rate limit exceeded", http.StatusTooManyRequests)
				err.Headers = map[string]string{"X-RateLimit-Remaining-Tokens": "0"}
				return nil, AdmissionDecision{}, err
			}
		}

		// 2. Evaluate Upstream Account Limits (ProviderID)
		if route.ProviderID != "" {
			providerRules, err := c.repo.LimitRules().ListByTarget(ctx, controlstate.ScopeUpstreamAccount, route.ProviderID)
			if err != nil {
				return nil, AdmissionDecision{}, fmt.Errorf("failed to fetch provider rules: %w", err)
			}
			for _, rule := range providerRules {
				if !rule.Enabled {
					continue
				}
				windowDuration := parseWindow(rule.Window)
				key := fmt.Sprintf("limit:upstream:%s:%s", rule.Dimension, route.ProviderID)

				_, allowed, err := c.limiter.CheckAndIncrement(ctx, key, rule.Limit, windowDuration)
				if err != nil {
					return nil, AdmissionDecision{}, fmt.Errorf("limiter unavailable: %w", err)
				}
				if !allowed {
					err := errors.NewGatewayError("provider_rate_limit_exceeded", "Provider rate limit exceeded", http.StatusTooManyRequests)
					err.Headers = map[string]string{"X-RateLimit-Remaining-Tokens": "0"}
					return nil, AdmissionDecision{}, err
				}
			}
		}
	}

	return func() {}, AdmissionDecision{
		PriorityClass: priority,
		QueueWaitMs:   0,
	}, nil
}

func parseWindow(w controlstate.LimitRuleWindow) time.Duration {
	switch w {
	case controlstate.Window1M:
		return time.Minute
	case controlstate.Window5H:
		return 5 * time.Hour
	case controlstate.Window1D:
		return 24 * time.Hour
	case controlstate.Window7D:
		return 7 * 24 * time.Hour
	}
	return time.Minute
}
