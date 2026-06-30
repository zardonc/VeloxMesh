package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"veloxmesh/internal/controlstate"
)

type sqliteLimitRuleRepository struct {
	db *sql.DB
}

func (r *sqliteLimitRuleRepository) ListByTarget(ctx context.Context, scope controlstate.LimitRuleScope, targetID string) ([]*controlstate.LimitRule, error) {
	query := `
		SELECT id, scope, target_id, dimension, window, limit_val, enabled, created_at, updated_at
		FROM limit_rules
		WHERE scope = ? AND target_id = ?
	`
	rows, err := r.db.QueryContext(ctx, query, string(scope), targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query limit rules: %w", err)
	}
	defer rows.Close()

	var rules []*controlstate.LimitRule
	for rows.Next() {
		var rule controlstate.LimitRule
		var createdAt, updatedAt string
		if err := rows.Scan(
			&rule.ID, &rule.Scope, &rule.TargetID, &rule.Dimension, &rule.Window,
			&rule.Limit, &rule.Enabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan limit rule: %w", err)
		}
		rule.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		rule.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		rules = append(rules, &rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("limit rules iteration error: %w", err)
	}
	return rules, nil
}

func (r *sqliteLimitRuleRepository) Save(ctx context.Context, rule *controlstate.LimitRule) error {
	// Validate constraints
	if rule.Scope != controlstate.ScopeAPIKey && rule.Scope != controlstate.ScopeUpstreamAccount {
		return fmt.Errorf("%w: %s", controlstate.ErrUnsupportedScope, rule.Scope)
	}
	if rule.Dimension == controlstate.DimensionProviderBalance {
		return fmt.Errorf("%w: %s is rejected", controlstate.ErrUnsupportedDimension, rule.Dimension)
	}
	if rule.Dimension != controlstate.DimensionRPM && 
	   rule.Dimension != controlstate.DimensionPeriodicBudget && 
	   rule.Dimension != controlstate.DimensionPeriodicRequests {
		return fmt.Errorf("%w: %s", controlstate.ErrUnsupportedDimension, rule.Dimension)
	}

	query := `
		INSERT INTO limit_rules (id, scope, target_id, dimension, window, limit_val, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scope=excluded.scope,
			target_id=excluded.target_id,
			dimension=excluded.dimension,
			window=excluded.window,
			limit_val=excluded.limit_val,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at
	`
	now := time.Now().UTC().Format(time.RFC3339)
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	
	_, err := r.db.ExecContext(ctx, query,
		rule.ID,
		string(rule.Scope),
		rule.TargetID,
		string(rule.Dimension),
		string(rule.Window),
		rule.Limit,
		rule.Enabled,
		rule.CreatedAt.Format(time.RFC3339),
		now,
	)
	if err != nil {
		return fmt.Errorf("failed to save limit rule: %w", err)
	}
	rule.UpdatedAt, _ = time.Parse(time.RFC3339, now)
	return nil
}

func (r *sqliteLimitRuleRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM limit_rules WHERE id = ?`
	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete limit rule: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return controlstate.ErrLimitRuleNotFound
	}
	return nil
}
