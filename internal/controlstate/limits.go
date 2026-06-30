package controlstate

import (
	"context"
	"errors"
	"time"
)

var (
	ErrLimitRuleNotFound = errors.New("limit rule not found")
	ErrUnsupportedScope  = errors.New("unsupported limit rule scope")
	ErrUnsupportedDimension = errors.New("unsupported limit rule dimension")
)

type LimitRuleScope string

const (
	ScopeAPIKey          LimitRuleScope = "api_key"
	ScopeUpstreamAccount LimitRuleScope = "upstream_account"
	// Deferred scopes
	ScopeTeam   LimitRuleScope = "team"
	ScopeModel  LimitRuleScope = "model"
	ScopeGlobal LimitRuleScope = "global"
)

type LimitRuleDimension string

const (
	DimensionRPM             LimitRuleDimension = "rpm"
	DimensionPeriodicBudget  LimitRuleDimension = "periodic_budget"
	DimensionPeriodicRequests LimitRuleDimension = "periodic_requests"
	// Rejected
	DimensionProviderBalance LimitRuleDimension = "provider_balance"
)

type LimitRuleWindow string

const (
	Window1M LimitRuleWindow = "1m"
	Window5H LimitRuleWindow = "5h"
	Window1D LimitRuleWindow = "1d"
	Window7D LimitRuleWindow = "7d"
)

type LimitRule struct {
	ID        string             `json:"id"`
	Scope     LimitRuleScope     `json:"scope"`
	TargetID  string             `json:"target_id"` // e.g. the API key ID or provider ID
	Dimension LimitRuleDimension `json:"dimension"`
	Window    LimitRuleWindow    `json:"window"`
	Limit     int64              `json:"limit"`
	Enabled   bool               `json:"enabled"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

type LimitRuleRepository interface {
	ListByTarget(ctx context.Context, scope LimitRuleScope, targetID string) ([]*LimitRule, error)
	Save(ctx context.Context, rule *LimitRule) error
	Delete(ctx context.Context, id string) error
}
