package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/pipeline"
)

type semanticRuleRepo struct {
	pool *pgxpool.Pool
}

const (
	emptyRuleOptionsJSON = "{}"
	nullRuleOptionsJSON  = "null"
)

func (r *semanticRuleRepo) GetGlobalDefaults(ctx context.Context) (*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.pool.Query(ctx, `SELECT rule, enabled, options_json FROM semantic_rule_global_defaults`)
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
		options, err := decodeRuleOptions(optionsJSON)
		if err != nil {
			return nil, err
		}
		cfg.Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{Enabled: enabled, Options: options}
	}
	return cfg, rows.Err()
}

func (r *semanticRuleRepo) GetUserConfig(ctx context.Context, userID string) (*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.pool.Query(ctx, `SELECT rule, enabled, options_json FROM semantic_rule_user_configs WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cfg := &pipeline.SemanticPipelineConfig{Rules: make(map[pipeline.RuleName]pipeline.RuleConfig)}
	for rows.Next() {
		var rule string
		var enabled bool
		var optionsJSON string
		if err := rows.Scan(&rule, &enabled, &optionsJSON); err != nil {
			return nil, err
		}
		options, err := decodeRuleOptions(optionsJSON)
		if err != nil {
			return nil, err
		}
		cfg.Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{Enabled: enabled, Options: options}
	}
	return cfg, rows.Err()
}

func (r *semanticRuleRepo) ListUserConfigs(ctx context.Context) (map[string]*pipeline.SemanticPipelineConfig, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id, rule, enabled, options_json FROM semantic_rule_user_configs`)
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
			configs[userID] = &pipeline.SemanticPipelineConfig{Rules: make(map[pipeline.RuleName]pipeline.RuleConfig)}
		}
		options, err := decodeRuleOptions(optionsJSON)
		if err != nil {
			return nil, err
		}
		configs[userID].Rules[pipeline.RuleName(rule)] = pipeline.RuleConfig{Enabled: enabled, Options: options}
	}
	return configs, rows.Err()
}

func (r *semanticRuleRepo) SaveGlobalDefaults(ctx context.Context, cfg *pipeline.SemanticPipelineConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	for name, rule := range cfg.Rules {
		optionsJSON, err := encodeRuleOptions(rule.Options)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO semantic_rule_global_defaults (rule, enabled, options_json, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT(rule) DO UPDATE SET
				enabled = excluded.enabled,
				options_json = excluded.options_json,
				updated_at = excluded.updated_at`,
			string(name), rule.Enabled, optionsJSON, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *semanticRuleRepo) SaveUserConfig(ctx context.Context, userID string, cfg *pipeline.SemanticPipelineConfig) error {
	if err := validateUserRuleConfig(cfg); err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM semantic_rule_user_configs WHERE user_id = $1`, userID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for name, rule := range cfg.Rules {
		optionsJSON, err := encodeRuleOptions(rule.Options)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO semantic_rule_user_configs (user_id, rule, enabled, options_json, updated_at)
			VALUES ($1, $2, $3, $4, $5)`,
			userID, string(name), rule.Enabled, optionsJSON, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func validateUserRuleConfig(cfg *pipeline.SemanticPipelineConfig) error {
	base := pipeline.DefaultSemanticPipelineConfig()
	rules := make(map[pipeline.RuleName]pipeline.RuleConfig, len(base.Rules)+len(cfg.Rules))
	for name, rule := range base.Rules {
		rules[name] = rule
	}
	for name, rule := range cfg.Rules {
		rules[name] = rule
	}
	return (&pipeline.SemanticPipelineConfig{Rules: rules}).Validate()
}

func encodeRuleOptions(options map[string]interface{}) (string, error) {
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return "", err
	}
	if string(optionsJSON) == nullRuleOptionsJSON {
		return emptyRuleOptionsJSON, nil
	}
	return string(optionsJSON), nil
}

func decodeRuleOptions(optionsJSON string) (map[string]interface{}, error) {
	if optionsJSON == "" || optionsJSON == emptyRuleOptionsJSON || optionsJSON == nullRuleOptionsJSON {
		return nil, nil
	}
	var options map[string]interface{}
	if err := json.Unmarshal([]byte(optionsJSON), &options); err != nil {
		return nil, err
	}
	return options, nil
}

var _ controlstate.SemanticRuleStore = (*semanticRuleRepo)(nil)
