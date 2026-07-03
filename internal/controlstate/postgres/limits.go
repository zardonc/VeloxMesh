package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
)

type limitRuleRepo struct {
	pool *pgxpool.Pool
}

func (r *limitRuleRepo) ListByTarget(ctx context.Context, scope controlstate.LimitRuleScope, targetID string) ([]*controlstate.LimitRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, scope, target_id, dimension, "window", limit_val, enabled, created_at, updated_at
		FROM limit_rules
		WHERE scope = $1 AND target_id = $2`, string(scope), targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query limit rules: %w", err)
	}
	defer rows.Close()

	var rules []*controlstate.LimitRule
	for rows.Next() {
		rule := &controlstate.LimitRule{}
		err := rows.Scan(
			&rule.ID, &rule.Scope, &rule.TargetID, &rule.Dimension, &rule.Window,
			&rule.Limit, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan limit rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("limit rules iteration error: %w", err)
	}
	return rules, nil
}

func (r *limitRuleRepo) Save(ctx context.Context, rule *controlstate.LimitRule) error {
	if err := validateLimitRule(rule); err != nil {
		return err
	}
	now := time.Now().UTC()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	_, err := r.pool.Exec(ctx, `
		INSERT INTO limit_rules (id, scope, target_id, dimension, "window", limit_val, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			target_id=excluded.target_id,
			dimension=excluded.dimension,
			"window"=excluded."window",
			limit_val=excluded.limit_val,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at`,
		rule.ID, string(rule.Scope), rule.TargetID, string(rule.Dimension), string(rule.Window),
		rule.Limit, rule.Enabled, rule.CreatedAt, rule.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save limit rule: %w", err)
	}
	return nil
}

func (r *limitRuleRepo) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM limit_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete limit rule: %w", err)
	}
	if res.RowsAffected() == 0 {
		return controlstate.ErrLimitRuleNotFound
	}
	return nil
}

func validateLimitRule(rule *controlstate.LimitRule) error {
	if rule.Scope != controlstate.ScopeAPIKey && rule.Scope != controlstate.ScopeUpstreamAccount {
		return fmt.Errorf("%w: %s", controlstate.ErrUnsupportedScope, rule.Scope)
	}
	switch rule.Dimension {
	case controlstate.DimensionRPM, controlstate.DimensionPeriodicBudget, controlstate.DimensionPeriodicRequests:
		return nil
	case controlstate.DimensionProviderBalance:
		return fmt.Errorf("%w: %s is rejected", controlstate.ErrUnsupportedDimension, rule.Dimension)
	default:
		return fmt.Errorf("%w: %s", controlstate.ErrUnsupportedDimension, rule.Dimension)
	}
}
