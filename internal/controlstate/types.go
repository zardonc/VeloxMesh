package controlstate

import (
	"encoding/json"
	"time"
)

type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ProviderSecretMetadata struct {
	SecretConfigured bool       `json:"secret_configured"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
}

type ProviderRecord struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	BaseURL      string                 `json:"base_url"`
	Enabled      bool                   `json:"enabled"`
	Models       []string               `json:"models,omitempty"`
	DefaultModel string                 `json:"default_model,omitempty"`
	Timeout      string                 `json:"timeout,omitempty"`
	Weight       int                    `json:"weight,omitempty"`
	HealthConfig json.RawMessage        `json:"health_config,omitempty"`
	Revision     int64                  `json:"revision"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Secret       ProviderSecretMetadata `json:"secret"`
}

type ProviderMutation struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	BaseURL      string          `json:"base_url"`
	Enabled      bool            `json:"enabled"`
	APIKey       *string         `json:"api_key,omitempty"` // cleartext for mutation only
	Models       []string        `json:"models,omitempty"`
	DefaultModel *string         `json:"default_model,omitempty"`
	Timeout      *string         `json:"timeout,omitempty"`
	Weight       *int            `json:"weight,omitempty"`
	HealthConfig json.RawMessage `json:"health_config,omitempty"`
	Revision     *int64          `json:"revision,omitempty"`
}

type ProviderFilter struct {
	Enabled *bool
	Type    string
	Search  string
}

type ComboRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Strategy  string    `json:"strategy"` // round-robin, fusion, capacity-auto-switch
	Members   []string  `json:"members"`
	Judge     string    `json:"judge,omitempty"`
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ComboMutation struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	Strategy string   `json:"strategy"`
	Members  []string `json:"members"`
	Judge    *string  `json:"judge,omitempty"`
	Revision *int64   `json:"revision,omitempty"`
}

type ComboFilter struct {
	Enabled *bool
	Search  string
}

type RoutingConfig struct {
	ID              string    `json:"id"`
	Strategy        string    `json:"strategy"`
	DefaultProvider string    `json:"default_provider"`
	FallbackEnabled bool      `json:"fallback_enabled"`
	MaxAttempts     int                     `json:"max_attempts"`
	Composite       *CompositeRoutingConfig `json:"composite,omitempty"`
	Revision        int64                   `json:"revision"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type CompositeRoutingConfig struct {
	PresetName          string             `json:"preset_name,omitempty"`
	LatencyWeight       float64            `json:"latency_weight"`
	LoadWeight          float64            `json:"load_weight"`
	ErrorRateWeight     float64            `json:"error_rate_weight"`
	HealthWeight        float64            `json:"health_weight"`
	ScoreThreshold      float64            `json:"score_threshold"`
	NearTieThreshold    float64            `json:"near_tie_threshold"`
	DegradedPenalty     float64            `json:"degraded_penalty"`
	WarmUpSuccesses     int                `json:"warm_up_successes"`
	StaleMetricWindow   string             `json:"stale_metric_window"`
	MinZScoreCandidates int                `json:"min_zscore_candidates"`
	MinVariance         float64            `json:"min_variance"`
	CostOverrides       map[string]float64 `json:"cost_overrides,omitempty"`
}

type APIKeyRecord struct {
	ID            string    `json:"id"`
	Prefix        string    `json:"prefix"`
	Hash          string    `json:"-"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	Enabled       bool      `json:"enabled"`
	CreditBalance int64     `json:"credit_balance"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SettlementStatus string

const (
	SettlementStatusUnsettled    SettlementStatus = "unsettled"
	SettlementStatusSettled      SettlementStatus = "settled"
	SettlementStatusMissingRate  SettlementStatus = "missing_rate"
	SettlementStatusMissingUsage SettlementStatus = "missing_usage"
)

type ProviderModelRate struct {
	ProviderID       string    `json:"provider_id"`
	Model            string    `json:"model"`
	InputCreditRate  int64     `json:"input_credit_rate"`
	OutputCreditRate int64     `json:"output_credit_rate"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type UsageRecord struct {
	ID              string           `json:"id"`
	APIKeyID        *string          `json:"api_key_id,omitempty"`
	ProviderID      string           `json:"provider_id"`
	Model           string           `json:"model"`
	PromptTokens    int              `json:"prompt_tokens"`
	ResponseTokens  int              `json:"response_tokens"`
	TotalTokens     int              `json:"total_tokens"`
	DurationMs      int64            `json:"duration_ms"`
	Timestamp       time.Time        `json:"timestamp"`
	InputRate       *int64           `json:"input_rate,omitempty"`
	OutputRate      *int64           `json:"output_rate,omitempty"`
	CreditsConsumed *int64           `json:"credits_consumed,omitempty"`
	Status          SettlementStatus `json:"status"`
}

type AuditEvent struct {
	ID        string          `json:"id"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	TargetID  string          `json:"target_id"`
	Outcome   string          `json:"outcome"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type IdempotencyRecord struct {
	Key         string    `json:"key"`
	ActionName  string    `json:"action_name"`
	Fingerprint string    `json:"fingerprint"`
	Status      string    `json:"status"`
	Response    string    `json:"response,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ConfigChangeEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"` // e.g. "provider_updated", "routing_updated"
	TargetID  string    `json:"target_id"`
	Timestamp time.Time `json:"timestamp"`
}

func RedactProviderRecord(p *ProviderRecord) *ProviderRecord {
	if p == nil {
		return nil
	}
	clone := *p
	// Already safe because ProviderRecord doesn't contain raw API keys
	return &clone
}

type SemanticCacheEntry struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"` // e.g. API key identity
	Model     string    `json:"model"`
	Vector    []byte    `json:"vector"`
	Response  string    `json:"response"`
	UsageID   *string   `json:"usage_id,omitempty"`
	HitCount  int       `json:"hit_count"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type FallbackLogRecord struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Payload   string    `json:"payload"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
