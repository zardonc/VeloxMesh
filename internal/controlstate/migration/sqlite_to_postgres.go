package migration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

type Options struct {
	SQLiteDSN   string
	PostgresDSN string
}

type MigrationReport struct {
	CompletedTables []string `json:"completed_tables"`
	FailedTable     string   `json:"failed_table,omitempty"`
	FailedRecord    string   `json:"failed_record,omitempty"`
	RootError       string   `json:"root_error,omitempty"`
	Repair          string   `json:"repair,omitempty"`
}

type tablePlan struct {
	name       string
	columns    []string
	conflict   []string
	boolCols   map[string]bool
	jsonbCols  map[string]bool
	recordCols []string
}

type pgExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

func Migrate(ctx context.Context, opts Options) (MigrationReport, error) {
	source, err := sql.Open("sqlite", opts.SQLiteDSN)
	if err != nil {
		return MigrationReport{}, err
	}
	defer source.Close()

	target, err := pgxpool.New(ctx, opts.PostgresDSN)
	if err != nil {
		return MigrationReport{}, err
	}
	defer target.Close()

	return Run(ctx, source, target)
}

func Run(ctx context.Context, source *sql.DB, target pgExecutor) (MigrationReport, error) {
	report := MigrationReport{}
	for _, plan := range migrationPlans() {
		failedValues, err := migrateTable(ctx, source, target, plan)
		if err != nil {
			report.FailedTable = plan.name
			report.FailedRecord = recordKey(plan, failedValues)
			report.RootError = err.Error()
			report.Repair = "Fix the failed source record or target constraint, then rerun the same command. Completed tables are not rolled back."
			return report, err
		}
		report.CompletedTables = append(report.CompletedTables, plan.name)
	}
	return report, nil
}

func migrateTable(ctx context.Context, source *sql.DB, target pgExecutor, plan tablePlan) (map[string]interface{}, error) {
	rows, err := source.QueryContext(ctx, "SELECT "+strings.Join(plan.columns, ", ")+" FROM "+plan.name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		values, err := scanRow(rows, plan)
		if err != nil {
			return values, err
		}
		args := make([]interface{}, len(plan.columns))
		for i, column := range plan.columns {
			args[i] = values[column]
		}
		if _, err := target.Exec(ctx, upsertSQL(plan), args...); err != nil {
			return values, err
		}
	}
	return nil, rows.Err()
}

func scanRow(rows *sql.Rows, plan tablePlan) (map[string]interface{}, error) {
	values := make([]interface{}, len(plan.columns))
	ptrs := make([]interface{}, len(plan.columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	mapped := map[string]interface{}{}
	for i, column := range plan.columns {
		mapped[column] = normalizeValue(values[i], plan.boolCols[column])
	}
	return mapped, nil
}

func normalizeValue(value interface{}, isBool bool) interface{} {
	if !isBool {
		return value
	}
	switch v := value.(type) {
	case int64:
		return v != 0
	case bool:
		return v
	default:
		return value
	}
}

func upsertSQL(plan tablePlan) string {
	placeholders := make([]string, len(plan.columns))
	updates := []string{}
	for i, column := range plan.columns {
		placeholder := fmt.Sprintf("$%d", i+1)
		if plan.jsonbCols[column] {
			placeholder += "::jsonb"
		}
		placeholders[i] = placeholder
		if !contains(plan.conflict, column) {
			updates = append(updates, column+"=excluded."+column)
		}
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(%s) DO UPDATE SET %s",
		plan.name,
		strings.Join(plan.columns, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(plan.conflict, ", "),
		strings.Join(updates, ", "),
	)
}

func recordKey(plan tablePlan, values map[string]interface{}) string {
	if values == nil {
		return ""
	}
	parts := make([]string, 0, len(plan.recordCols))
	for _, column := range plan.recordCols {
		parts = append(parts, fmt.Sprintf("%s=%v", column, values[column]))
	}
	return strings.Join(parts, ",")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func migrationPlans() []tablePlan {
	jsonb := func(cols ...string) map[string]bool {
		m := map[string]bool{}
		for _, col := range cols {
			m[col] = true
		}
		return m
	}
	bools := func(cols ...string) map[string]bool {
		m := map[string]bool{}
		for _, col := range cols {
			m[col] = true
		}
		return m
	}
	return []tablePlan{
		{name: "provider_configs", columns: []string{"id", "name", "type", "base_url", "enabled", "models_json", "default_model", "timeout", "weight", "health_config", "revision", "created_at", "updated_at"}, conflict: []string{"id"}, boolCols: bools("enabled"), jsonbCols: jsonb("models_json", "health_config"), recordCols: []string{"id"}},
		{name: "provider_secrets", columns: []string{"provider_id", "ciphertext", "nonce", "key_id", "updated_at"}, conflict: []string{"provider_id"}, recordCols: []string{"provider_id"}},
		{name: "routing_configs", columns: []string{"id", "strategy", "default_provider", "fallback_enabled", "max_attempts", "revision", "created_at", "updated_at"}, conflict: []string{"id"}, boolCols: bools("fallback_enabled"), recordCols: []string{"id"}},
		{name: "api_keys", columns: []string{"id", "prefix", "hash", "name", "role", "enabled", "credit_balance", "created_at", "updated_at"}, conflict: []string{"id"}, boolCols: bools("enabled"), recordCols: []string{"id"}},
		{name: "provider_model_rates", columns: []string{"provider_id", "model", "input_credit_rate", "output_credit_rate", "created_at", "updated_at"}, conflict: []string{"provider_id", "model"}, recordCols: []string{"provider_id", "model"}},
		{name: "usage_records", columns: []string{"id", "api_key_id", "provider_id", "model", "prompt_tokens", "response_tokens", "total_tokens", "duration_ms", "timestamp", "input_rate", "output_rate", "credits_consumed", "status"}, conflict: []string{"id"}, recordCols: []string{"id"}},
		{name: "audit_events", columns: []string{"id", "actor", "action", "target_id", "outcome", "metadata", "timestamp"}, conflict: []string{"id"}, jsonbCols: jsonb("metadata"), recordCols: []string{"id"}},
		{name: "idempotency_keys", columns: []string{"key", "action_name", "fingerprint", "status", "response", "created_at", "expires_at"}, conflict: []string{"key"}, recordCols: []string{"key"}},
		{name: "semantic_cache_entries", columns: []string{"id", "scope", "model", "vector", "response", "usage_id", "hit_count", "enabled", "created_at", "expires_at"}, conflict: []string{"id"}, boolCols: bools("enabled"), recordCols: []string{"id"}},
		{name: "fallback_log", columns: []string{"id", "type", "payload", "status", "created_at", "updated_at"}, conflict: []string{"id"}, recordCols: []string{"id"}},
		{name: "combos", columns: []string{"id", "name", "enabled", "strategy", "members", "judge", "revision", "created_at", "updated_at"}, conflict: []string{"id"}, boolCols: bools("enabled"), jsonbCols: jsonb("members"), recordCols: []string{"id"}},
		{name: "limit_rules", columns: []string{"id", "scope", "target_id", "dimension", "window", "limit_val", "enabled", "created_at", "updated_at"}, conflict: []string{"id"}, boolCols: bools("enabled"), recordCols: []string{"id"}},
		{name: "session_blacklist", columns: []string{"session_hash", "reason", "expires_at", "created_at"}, conflict: []string{"session_hash"}, recordCols: []string{"session_hash"}},
	}
}
