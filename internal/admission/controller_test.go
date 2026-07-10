package admission_test

import (
	"context"
	"testing"
	"veloxmesh/internal/admission"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/routing"
)

type mockLimitRuleRepo struct {
	rules []*controlstate.LimitRule
}

func (m *mockLimitRuleRepo) ListByTarget(ctx context.Context, scope controlstate.LimitRuleScope, targetID string) ([]*controlstate.LimitRule, error) {
	var filtered []*controlstate.LimitRule
	for _, r := range m.rules {
		if r.Scope == scope && r.TargetID == targetID {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}
func (m *mockLimitRuleRepo) Save(ctx context.Context, rule *controlstate.LimitRule) error { return nil }
func (m *mockLimitRuleRepo) Delete(ctx context.Context, id string) error                  { return nil }

type mockRepo struct {
	controlstate.Repository
	limitRepo *mockLimitRuleRepo
}

func (m *mockRepo) LimitRules() controlstate.LimitRuleRepository {
	return m.limitRepo
}

func TestLimitAdmissionController_Admit(t *testing.T) {
	limiter := hotstate.NewLocalHotState() // use local hot state for atomic counter
	repo := &mockRepo{
		limitRepo: &mockLimitRuleRepo{
			rules: []*controlstate.LimitRule{
				{
					ID:        "r1",
					Scope:     controlstate.ScopeAPIKey,
					TargetID:  "key-1",
					Dimension: controlstate.DimensionRPM,
					Window:    controlstate.Window1M,
					Limit:     2,
					Enabled:   true,
				},
			},
		},
	}

	ctrl := admission.NewLimitAdmissionController(repo, limiter)

	req := &llm.LLMRequest{
		PriorityClass: "interactive",
	}

	identity := &middleware.AuthIdentity{
		ID:            "key-1",
		Role:          "user",
		CreditBalance: 1000,
	}
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, identity)
	route := routing.RoutingDecision{
		ProviderID: "openai",
	}

	// Request 1: allowed
	_, _, err := ctrl.Admit(ctx, req, route)
	if err != nil {
		t.Fatalf("expected allowed, got err: %v", err)
	}

	// Request 2: allowed
	_, _, err = ctrl.Admit(ctx, req, route)
	if err != nil {
		t.Fatalf("expected allowed, got err: %v", err)
	}

	// Request 3: rejected (limit 2)
	_, _, err = ctrl.Admit(ctx, req, route)
	if err == nil {
		t.Fatalf("expected rate limit error, got nil")
	}
}

func TestLimitAdmissionController_CreditBalance(t *testing.T) {
	limiter := hotstate.NewLocalHotState()
	repo := &mockRepo{
		limitRepo: &mockLimitRuleRepo{},
	}
	ctrl := admission.NewLimitAdmissionController(repo, limiter)

	req := &llm.LLMRequest{
		PriorityClass: "interactive",
	}

	identity := &middleware.AuthIdentity{
		ID:            "key-1",
		Role:          "user",
		CreditBalance: 0,
	}
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, identity)
	route := routing.RoutingDecision{}

	_, _, err := ctrl.Admit(ctx, req, route)
	if err == nil {
		t.Fatalf("expected insufficient credits error, got nil")
	}
}

func TestLimitAdmissionController_HighPriorityDoesNotBypassCredit(t *testing.T) {
	ctrl := admission.NewLimitAdmissionController(&mockRepo{limitRepo: &mockLimitRuleRepo{}}, hotstate.NewLocalHotState())
	req := &llm.LLMRequest{PriorityClass: "high"}
	identity := &middleware.AuthIdentity{ID: "key-1", Role: "user", CreditBalance: 0}
	ctx := context.WithValue(context.Background(), middleware.AuthIdentityKey, identity)

	_, _, err := ctrl.Admit(ctx, req, routing.RoutingDecision{})
	if err == nil {
		t.Fatalf("expected insufficient credits for high priority")
	}
}
