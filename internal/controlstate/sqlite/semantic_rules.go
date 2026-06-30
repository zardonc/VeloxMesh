package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/pipeline"
)

type semanticRuleRepository struct {
	db *sql.DB
}

func (r *Repository) SemanticRules() controlstate.SemanticRuleStore {
	return &semanticRuleRepository{db: r.db}
}

func (r *semanticRuleRepository) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT rule, enabled, options_json FROM semantic_rule_global_defaults`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cfg := pipeline.DefaultSemanticPipelineConfig()

	for rows.Next() {
		var rule string
		var enabled bool
		var optionsJSON string
		if err := rows.Scan(&rule, &enabled, &optionsJSON); err != nil {
			return nil, err
		}

		var options map[string]interface{}
		if optionsJSON != "" && optionsJSON != "{}" && optionsJSON != "null" {
			if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
				return nil, err
			}
		}

		cfg.Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{
			Enabled: enabled,
			Options: options,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (r *semanticRuleRepository) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT rule, enabled, options_json FROM semantic_rule_user_configs WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cfg := &pipeline.SemanticPipelineConfig{
		Rules: make(map[pipeline.RuleName]pipeline.RuleConfig),
	}

	for rows.Next() {
		var rule string
		var enabled bool
		var optionsJSON string
		if err := rows.Scan(&rule, &enabled, &optionsJSON); err != nil {
			return nil, err
		}

		var options map[string]interface{}
		if optionsJSON != "" && optionsJSON != "{}" && optionsJSON != "null" {
			if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
				return nil, err
			}
		}

		cfg.Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{
			Enabled: enabled,
			Options: options,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (r *semanticRuleRepository) ListUserConfigs(ctx context.Context) (map[string]*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id, rule, enabled, options_json FROM semantic_rule_user_configs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	configs := make(map[string]*pipeline.SemanticPipelineConfig)

	for rows.Next() {
		var userID string
		var rule string
		var enabled bool
		var optionsJSON string
		if err := rows.Scan(&userID, &rule, &enabled, &optionsJSON); err != nil {
			return nil, err
		}

		if _, exists := configs[userID]; !exists {
			configs[userID] = &pipeline.SemanticPipelineConfig{
				Rules: make(map[pipeline.RuleName]pipeline.RuleConfig),
			}
		}

		var options map[string]interface{}
		if optionsJSON != "" && optionsJSON != "{}" && optionsJSON != "null" {
			if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
				return nil, err
			}
		}

		configs[userID].Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{
			Enabled: enabled,
			Options: options,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return configs, nil
}

func (r *semanticRuleRepository) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Clear existing defaults to handle removed overrides if we wanted, but there are fixed 7 rules.
	// Actually, just UPSERT everything.
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO semantic_rule_global_defaults (rule, enabled, options_json, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(rule) DO UPDATE SET
			enabled = excluded.enabled,
			options_json = excluded.options_json,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for name, rule := range cfg.Rules {
		optionsJSON, err := json.Marshal(rule.Options)
		if err != nil {
			return err
		}
		if string(optionsJSON) == "null" {
			optionsJSON = []byte("{}")
		}

		if _, err := stmt.ExecContext(ctx, string(name), rule.Enabled, string(optionsJSON), now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *semanticRuleRepository) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	// A user config might be partial, but the plan says "validate every save with the config contract".
	// Wait, validate will fail if Caveman and Ponytail are both enabled. If they only specify one, they might not specify the other.
	// We can validate just the rules they provided by using a temporary config.
	testCfg := pipeline.DefaultSemanticPipelineConfig()
	for k, v := range cfg.Rules {
		testCfg.Rules[k] = v
	}
	if err := testCfg.Validate(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Delete all existing user configs first? Or upsert?
	// If the user config only contains partial updates, maybe UPSERT.
	// But if they removed an override, how do they delete it?
	// To keep it simple, we could delete all existing and insert the map.
	if _, err := tx.ExecContext(ctx, `DELETE FROM semantic_rule_user_configs WHERE user_id = ?`, userID); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO semantic_rule_user_configs (user_id, rule, enabled, options_json, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for name, rule := range cfg.Rules {
		optionsJSON, err := json.Marshal(rule.Options)
		if err != nil {
			return err
		}
		if string(optionsJSON) == "null" {
			optionsJSON = []byte("{}")
		}

		if _, err := stmt.ExecContext(ctx, userID, string(name), rule.Enabled, string(optionsJSON), now); err != nil {
			return err
		}
	}

	return tx.Commit()
}
