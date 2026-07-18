package bff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const gatewayAdminMaxResponseBytes = 1 << 20

var (
	ErrGatewayAdminTimeout          = errors.New("gateway admin request timed out")
	ErrGatewayAdminUnavailable      = errors.New("gateway admin is unavailable")
	ErrGatewayAdminInvalidResponse  = errors.New("gateway admin returned an invalid response")
	ErrGatewayAdminResponseTooLarge = errors.New("gateway admin response exceeded the size limit")
	ErrGatewayDataKeyRequired       = errors.New("gateway data API key is required for live verification")
)

type gatewayOperationContextKey string

const (
	gatewayActorContextKey   gatewayOperationContextKey = "gateway_actor"
	gatewayRequestContextKey gatewayOperationContextKey = "gateway_request_id"
)

func WithGatewayOperation(ctx context.Context, actor, requestID string) context.Context {
	ctx = context.WithValue(ctx, gatewayActorContextKey, strings.TrimSpace(actor))
	return context.WithValue(ctx, gatewayRequestContextKey, strings.TrimSpace(requestID))
}

// GatewayAdminHTTPError preserves the upstream status without retaining its
// response body, which may contain provider or credential details.
type GatewayAdminHTTPError struct {
	StatusCode int
	Method     string
	Path       string
}

func (err *GatewayAdminHTTPError) Error() string {
	return fmt.Sprintf("gateway admin returned HTTP %d for %s %s", err.StatusCode, err.Method, err.Path)
}

type GatewayAdminClient interface {
	ListProviders(ctx context.Context) ([]GatewayProvider, error)
	GetProvider(ctx context.Context, providerID string) (GatewayProvider, error)
	CreateProvider(ctx context.Context, request GatewayProviderMutation) (GatewayProvider, error)
	UpdateProvider(ctx context.Context, providerID string, request GatewayProviderMutation) (GatewayProvider, error)
	DeleteProvider(ctx context.Context, providerID string) error
	GetRouting(ctx context.Context) (GatewayRouting, error)
	PutRouting(ctx context.Context, request GatewayRoutingUpdateRequest) (GatewayRouting, error)
	ListTenants(ctx context.Context) ([]GatewayTenant, error)
	CreateTenant(ctx context.Context, request GatewayTenantCreateRequest) (GatewayTenant, error)
	UpdateTenant(ctx context.Context, tenantID string, request GatewayTenantUpdateRequest) (GatewayTenant, error)
	DeleteTenant(ctx context.Context, tenantID string) error
	ListAPIKeys(ctx context.Context) ([]GatewayAPIKey, error)
	CreateAPIKey(ctx context.Context, request GatewayAPIKeyCreateRequest) (GatewayAPIKeyCreateResponse, error)
	RevokeAPIKey(ctx context.Context, keyID string) error
	ListAudit(ctx context.Context, filter GatewayAuditFilter) ([]GatewayAuditEvent, error)
	ListUsage(ctx context.Context, filter GatewayUsageFilter) ([]GatewayUsageRecord, error)
	GetSettings(ctx context.Context) (GatewaySettings, error)
	PutSettings(ctx context.Context, request GatewaySettingsUpdateRequest) (GatewaySettings, error)
	GetHealth(ctx context.Context) (GatewayHealth, error)
	GetReadiness(ctx context.Context) (GatewayReadiness, error)
	GetTopology(ctx context.Context) (GatewayTopology, error)
	GetMetrics(ctx context.Context) (string, error)
	VerifyModels(ctx context.Context, model string) (GatewayVerification, error)
	VerifyChat(ctx context.Context, model string) (GatewayVerification, error)
}

type GatewayVerification struct {
	Verified   bool   `json:"verified"`
	RequestID  string `json:"request_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Route      string `json:"route,omitempty"`
	Message    string `json:"message,omitempty"`
}

type GatewayProviderSecret struct {
	Configured bool       `json:"configured"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type GatewayProvider struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Type         string                `json:"type"`
	BaseURL      string                `json:"base_url"`
	Enabled      bool                  `json:"enabled"`
	Models       []string              `json:"models,omitempty"`
	DefaultModel string                `json:"default_model,omitempty"`
	Timeout      string                `json:"timeout,omitempty"`
	Weight       int                   `json:"weight,omitempty"`
	Revision     int64                 `json:"revision"`
	Secret       GatewayProviderSecret `json:"secret"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

type GatewayProviderMutation struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	BaseURL      string   `json:"base_url"`
	Enabled      bool     `json:"enabled"`
	APIKey       *string  `json:"api_key,omitempty"`
	Models       []string `json:"models"`
	DefaultModel *string  `json:"default_model,omitempty"`
	Timeout      *string  `json:"timeout,omitempty"`
	Weight       *int     `json:"weight,omitempty"`
	Revision     int64    `json:"revision,omitempty"`
}

type GatewayCompositeRouting struct {
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

type GatewayRouting struct {
	ID              string                           `json:"id"`
	Strategy        string                           `json:"strategy"`
	DefaultProvider string                           `json:"default_provider"`
	FallbackEnabled bool                             `json:"fallback_enabled"`
	MaxAttempts     int                              `json:"max_attempts"`
	Composite       *GatewayCompositeRouting         `json:"composite,omitempty"`
	Revision        int64                            `json:"revision"`
	CreatedAt       time.Time                        `json:"created_at"`
	UpdatedAt       time.Time                        `json:"updated_at"`
	Application     *GatewayConfigurationApplication `json:"application,omitempty"`
}

type GatewayConfigurationApplication struct {
	State      string `json:"state"`
	Applied    bool   `json:"applied"`
	Verified   bool   `json:"verified"`
	Revision   int64  `json:"revision"`
	RequestID  string `json:"request_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Route      string `json:"route,omitempty"`
	Message    string `json:"message,omitempty"`
}

type GatewayRoutingUpdateRequest struct {
	Strategy        string                   `json:"strategy"`
	DefaultProvider string                   `json:"default_provider"`
	FallbackEnabled bool                     `json:"fallback_enabled"`
	MaxAttempts     int                      `json:"max_attempts"`
	Composite       *GatewayCompositeRouting `json:"composite,omitempty"`
	Revision        int64                    `json:"revision"`
}

type GatewayTenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Owner      string    `json:"owner"`
	DailyQuota int64     `json:"daily_quota"`
	Status     string    `json:"status"`
	Revision   int64     `json:"revision"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type GatewayTenantCreateRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	DailyQuota int64  `json:"daily_quota"`
	Status     string `json:"status"`
}

type GatewayTenantUpdateRequest struct {
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	DailyQuota int64  `json:"daily_quota"`
	Status     string `json:"status"`
	Revision   int64  `json:"revision"`
}

type GatewayAPIKey struct {
	ID            string     `json:"id"`
	Prefix        string     `json:"prefix"`
	Name          string     `json:"name"`
	TenantID      string     `json:"tenant_id"`
	Role          string     `json:"role"`
	Enabled       bool       `json:"enabled"`
	CreditBalance int64      `json:"credit_balance"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
}

type GatewayAPIKeyCreateRequest struct {
	Name          string `json:"name"`
	TenantID      string `json:"tenant_id"`
	Role          string `json:"role"`
	CreditBalance int64  `json:"credit_balance"`
}

type GatewayAPIKeyCreateResponse struct {
	Record GatewayAPIKey `json:"record"`
	Secret string        `json:"secret"`
}

type GatewayAuditEvent struct {
	ID        string          `json:"id"`
	Actor     string          `json:"actor"`
	Action    string          `json:"action"`
	TargetID  string          `json:"target_id"`
	Outcome   string          `json:"outcome"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type GatewayAuditFilter struct {
	TargetID string
	Actor    string
	Action   string
	Outcome  string
	Limit    int
}

type GatewayUsageRecord struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id,omitempty"`
	APIKeyID        *string   `json:"api_key_id,omitempty"`
	ProviderID      string    `json:"provider_id"`
	Model           string    `json:"model"`
	PromptTokens    int       `json:"prompt_tokens"`
	ResponseTokens  int       `json:"response_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	DurationMs      int64     `json:"duration_ms"`
	Timestamp       time.Time `json:"timestamp"`
	InputRate       *int64    `json:"input_rate,omitempty"`
	OutputRate      *int64    `json:"output_rate,omitempty"`
	CreditsConsumed *int64    `json:"credits_consumed,omitempty"`
	Status          string    `json:"status"`
}

type GatewayUsageFilter struct {
	TenantID   string
	APIKeyID   string
	ProviderID string
	Model      string
	Status     string
	Start      *time.Time
	End        *time.Time
	Limit      int
}

type GatewaySettings struct {
	ID                    string    `json:"id"`
	DefaultProvider       string    `json:"default_provider"`
	DefaultModel          string    `json:"default_model"`
	RequestTimeoutSeconds int       `json:"request_timeout_seconds"`
	DataRetentionDays     int       `json:"data_retention_days"`
	Revision              int64     `json:"revision"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type GatewaySettingsUpdateRequest struct {
	DefaultProvider       string `json:"default_provider"`
	DefaultModel          string `json:"default_model"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	DataRetentionDays     int    `json:"data_retention_days"`
	Revision              int64  `json:"revision"`
}

type GatewayHealth struct {
	Status string `json:"status"`
}

type GatewayReadiness struct {
	Status              string `json:"status"`
	ConfiguredProviders int    `json:"configured_providers"`
	Healthy             int    `json:"healthy"`
	Degraded            int    `json:"degraded"`
	Unhealthy           int    `json:"unhealthy"`
	RoutingStrategy     string `json:"routing_strategy"`
}

type GatewayTopology struct {
	NodeID         string `json:"node_id"`
	Role           string `json:"role"`
	LeaderID       string `json:"leader_id"`
	Writable       bool   `json:"writable"`
	WALLagElapsed  int64  `json:"wal_lag_elapsed"`
	WALLagPending  int64  `json:"wal_lag_pending"`
	DegradedReason string `json:"degraded_reason,omitempty"`
}

type httpGatewayAdminClient struct {
	adminBaseURL   string
	dataBaseURL    string
	metricsBaseURL string
	adminAPIKey    string
	dataAPIKey     string
	httpClient     *http.Client
}

func NewHTTPGatewayAdminClient(adminURL, adminAPIKey string, timeout time.Duration, httpClient *http.Client) (GatewayAdminClient, error) {
	return NewHTTPGatewayAdminClientWithCredentials(adminURL, adminURL, adminURL, adminAPIKey, "", timeout, httpClient)
}

func NewHTTPGatewayAdminClientWithEndpoints(adminURL, dataURL, metricsURL, adminAPIKey string, timeout time.Duration, httpClient *http.Client) (GatewayAdminClient, error) {
	return NewHTTPGatewayAdminClientWithCredentials(adminURL, dataURL, metricsURL, adminAPIKey, "", timeout, httpClient)
}

func NewHTTPGatewayAdminClientWithCredentials(adminURL, dataURL, metricsURL, adminAPIKey, dataAPIKey string, timeout time.Duration, httpClient *http.Client) (GatewayAdminClient, error) {
	parsedAdminURL, err := parseGatewayBaseURL(adminURL, "admin")
	if err != nil {
		return nil, err
	}
	parsedDataURL, err := parseGatewayBaseURL(firstGatewayURL(dataURL, adminURL), "data")
	if err != nil {
		return nil, err
	}
	parsedMetricsURL, err := parseGatewayBaseURL(firstGatewayURL(metricsURL, dataURL, adminURL), "metrics")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(adminAPIKey) == "" {
		return nil, errors.New("gateway admin API key is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if timeout > 0 {
		clientCopy.Timeout = timeout
	} else if clientCopy.Timeout <= 0 {
		clientCopy.Timeout = 10 * time.Second
	}

	return &httpGatewayAdminClient{
		adminBaseURL:   strings.TrimRight(parsedAdminURL.String(), "/"),
		dataBaseURL:    strings.TrimRight(parsedDataURL.String(), "/"),
		metricsBaseURL: strings.TrimRight(parsedMetricsURL.String(), "/"),
		adminAPIKey:    adminAPIKey,
		dataAPIKey:     strings.TrimSpace(dataAPIKey),
		httpClient:     &clientCopy,
	}, nil
}

func parseGatewayBaseURL(value, label string) (*url.URL, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("gateway %s URL must be an absolute HTTP(S) URL", label)
	}
	if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, fmt.Errorf("gateway %s URL must not contain a query or fragment", label)
	}
	return parsedURL, nil
}

func firstGatewayURL(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (client *httpGatewayAdminClient) String() string {
	return "GatewayAdminClient{admin_url:" + client.adminBaseURL + ", admin_api_key:[REDACTED], data_api_key:[REDACTED]}"
}

func (client *httpGatewayAdminClient) GoString() string {
	return client.String()
}

func (client *httpGatewayAdminClient) ListProviders(ctx context.Context) ([]GatewayProvider, error) {
	var response gatewayDataResponse[GatewayProvider]
	if err := client.doJSON(ctx, http.MethodGet, "/admin/v1/providers", nil, nil, &response); err != nil {
		return nil, err
	}
	return unwrapGatewayData(response, func(item GatewayProvider) bool { return item.ID != "" && item.Name != "" && item.BaseURL != "" })
}

func (client *httpGatewayAdminClient) GetProvider(ctx context.Context, providerID string) (GatewayProvider, error) {
	var response GatewayProvider
	err := client.doJSON(ctx, http.MethodGet, resourcePath("/admin/v1/providers", providerID), nil, nil, &response)
	if err == nil && (response.ID == "" || response.Name == "" || response.BaseURL == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) CreateProvider(ctx context.Context, request GatewayProviderMutation) (GatewayProvider, error) {
	var response GatewayProvider
	err := client.doJSON(ctx, http.MethodPost, "/admin/v1/providers", nil, request, &response)
	if err == nil && (response.ID == "" || response.Name == "" || response.BaseURL == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) UpdateProvider(ctx context.Context, providerID string, request GatewayProviderMutation) (GatewayProvider, error) {
	var response GatewayProvider
	err := client.doJSON(ctx, http.MethodPut, resourcePath("/admin/v1/providers", providerID), nil, request, &response)
	if err == nil && (response.ID == "" || response.Name == "" || response.BaseURL == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) DeleteProvider(ctx context.Context, providerID string) error {
	return client.doJSON(ctx, http.MethodDelete, resourcePath("/admin/v1/providers", providerID), nil, nil, nil)
}

func (client *httpGatewayAdminClient) GetRouting(ctx context.Context) (GatewayRouting, error) {
	var response GatewayRouting
	err := client.doJSON(ctx, http.MethodGet, "/admin/v1/routing", nil, nil, &response)
	if err == nil && (response.ID == "" || response.Strategy == "" || response.MaxAttempts < 1) {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) PutRouting(ctx context.Context, request GatewayRoutingUpdateRequest) (GatewayRouting, error) {
	var response GatewayRouting
	err := client.doJSON(ctx, http.MethodPut, "/admin/v1/routing", nil, request, &response)
	if err == nil && (response.ID == "" || response.Strategy == "" || response.MaxAttempts < 1) {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) ListTenants(ctx context.Context) ([]GatewayTenant, error) {
	var response gatewayDataResponse[GatewayTenant]
	if err := client.doJSON(ctx, http.MethodGet, "/admin/v1/tenants", nil, nil, &response); err != nil {
		return nil, err
	}
	return unwrapGatewayData(response, func(item GatewayTenant) bool { return item.ID != "" && item.Name != "" && item.Status != "" })
}

func (client *httpGatewayAdminClient) CreateTenant(ctx context.Context, request GatewayTenantCreateRequest) (GatewayTenant, error) {
	var response GatewayTenant
	err := client.doJSON(ctx, http.MethodPost, "/admin/v1/tenants", nil, request, &response)
	if err == nil && (response.ID == "" || response.Name == "" || response.Status == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) UpdateTenant(ctx context.Context, tenantID string, request GatewayTenantUpdateRequest) (GatewayTenant, error) {
	var response GatewayTenant
	err := client.doJSON(ctx, http.MethodPut, resourcePath("/admin/v1/tenants", tenantID), nil, request, &response)
	if err == nil && (response.ID == "" || response.Name == "" || response.Status == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) DeleteTenant(ctx context.Context, tenantID string) error {
	return client.doJSON(ctx, http.MethodDelete, resourcePath("/admin/v1/tenants", tenantID), nil, nil, nil)
}

func (client *httpGatewayAdminClient) ListAPIKeys(ctx context.Context) ([]GatewayAPIKey, error) {
	var response gatewayDataResponse[GatewayAPIKey]
	if err := client.doJSON(ctx, http.MethodGet, "/admin/v1/api-keys", nil, nil, &response); err != nil {
		return nil, err
	}
	return unwrapGatewayData(response, func(item GatewayAPIKey) bool { return item.ID != "" && item.Prefix != "" && item.TenantID != "" })
}

func (client *httpGatewayAdminClient) CreateAPIKey(ctx context.Context, request GatewayAPIKeyCreateRequest) (GatewayAPIKeyCreateResponse, error) {
	var response GatewayAPIKeyCreateResponse
	err := client.doJSON(ctx, http.MethodPost, "/admin/v1/api-keys", nil, request, &response)
	if err == nil && (response.Secret == "" || response.Record.ID == "" || response.Record.Prefix == "" || response.Record.TenantID == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) RevokeAPIKey(ctx context.Context, keyID string) error {
	return client.doJSON(ctx, http.MethodDelete, resourcePath("/admin/v1/api-keys", keyID), nil, nil, nil)
}

func (client *httpGatewayAdminClient) ListAudit(ctx context.Context, filter GatewayAuditFilter) ([]GatewayAuditEvent, error) {
	query := make(url.Values)
	setQuery(query, "target_id", filter.TargetID)
	setQuery(query, "actor", filter.Actor)
	setQuery(query, "action", filter.Action)
	setQuery(query, "outcome", filter.Outcome)
	setPositiveIntQuery(query, "limit", filter.Limit)
	var response gatewayDataResponse[GatewayAuditEvent]
	if err := client.doJSON(ctx, http.MethodGet, "/admin/v1/audit", query, nil, &response); err != nil {
		return nil, err
	}
	return unwrapGatewayData(response, func(item GatewayAuditEvent) bool {
		return item.ID != "" && item.Action != "" && !item.Timestamp.IsZero()
	})
}

func (client *httpGatewayAdminClient) ListUsage(ctx context.Context, filter GatewayUsageFilter) ([]GatewayUsageRecord, error) {
	query := make(url.Values)
	setQuery(query, "tenant_id", filter.TenantID)
	setQuery(query, "api_key_id", filter.APIKeyID)
	setQuery(query, "provider_id", filter.ProviderID)
	setQuery(query, "model", filter.Model)
	setQuery(query, "status", filter.Status)
	if filter.Start != nil {
		query.Set("start", filter.Start.UTC().Format(time.RFC3339))
	}
	if filter.End != nil {
		query.Set("end", filter.End.UTC().Format(time.RFC3339))
	}
	setPositiveIntQuery(query, "limit", filter.Limit)
	var response gatewayDataResponse[GatewayUsageRecord]
	if err := client.doJSON(ctx, http.MethodGet, "/admin/v1/usage", query, nil, &response); err != nil {
		return nil, err
	}
	return unwrapGatewayData(response, func(item GatewayUsageRecord) bool {
		return item.ID != "" && item.ProviderID != "" && item.Model != "" && !item.Timestamp.IsZero()
	})
}

func (client *httpGatewayAdminClient) GetSettings(ctx context.Context) (GatewaySettings, error) {
	var response GatewaySettings
	err := client.doJSON(ctx, http.MethodGet, "/admin/v1/settings", nil, nil, &response)
	if err == nil && (response.ID == "" || response.RequestTimeoutSeconds < 1 || response.DataRetentionDays < 1) {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) PutSettings(ctx context.Context, request GatewaySettingsUpdateRequest) (GatewaySettings, error) {
	var response GatewaySettings
	err := client.doJSON(ctx, http.MethodPut, "/admin/v1/settings", nil, request, &response)
	if err == nil && (response.ID == "" || response.RequestTimeoutSeconds < 1 || response.DataRetentionDays < 1) {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) GetHealth(ctx context.Context) (GatewayHealth, error) {
	text, err := client.doTextAt(ctx, client.dataBaseURL, false, http.MethodGet, "/healthz")
	if err != nil {
		return GatewayHealth{}, err
	}
	status := strings.TrimSpace(text)
	if status != "ok" {
		return GatewayHealth{}, ErrGatewayAdminInvalidResponse
	}
	return GatewayHealth{Status: status}, nil
}

func (client *httpGatewayAdminClient) GetReadiness(ctx context.Context) (GatewayReadiness, error) {
	var response GatewayReadiness
	err := client.doJSONAt(ctx, client.dataBaseURL, false, http.MethodGet, "/readyz", nil, nil, &response)
	if err == nil && response.Status == "" {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) GetTopology(ctx context.Context) (GatewayTopology, error) {
	var response GatewayTopology
	err := client.doJSON(ctx, http.MethodGet, "/admin/v1/topology", nil, nil, &response)
	if err == nil && (response.NodeID == "" || response.Role == "") {
		err = ErrGatewayAdminInvalidResponse
	}
	return response, err
}

func (client *httpGatewayAdminClient) GetMetrics(ctx context.Context) (string, error) {
	metrics, err := client.doTextAt(ctx, client.metricsBaseURL, false, http.MethodGet, "/metrics")
	if err == nil && strings.TrimSpace(metrics) == "" {
		err = ErrGatewayAdminInvalidResponse
	}
	return metrics, err
}

func (client *httpGatewayAdminClient) VerifyModels(ctx context.Context, model string) (GatewayVerification, error) {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	headers, err := client.doDataJSON(ctx, http.MethodGet, "/v1/models", nil, &response)
	if err != nil {
		return GatewayVerification{}, err
	}
	for _, item := range response.Data {
		if item.ID == model {
			return GatewayVerification{Verified: true, RequestID: headers.Get("X-Request-ID")}, nil
		}
	}
	return GatewayVerification{Verified: false, RequestID: headers.Get("X-Request-ID"), Message: "target model was not present in the live model catalog"}, nil
}

func (client *httpGatewayAdminClient) VerifyChat(ctx context.Context, model string) (GatewayVerification, error) {
	body := map[string]interface{}{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "Reply OK."}},
		"max_tokens": 1,
		"stream":     false,
	}
	var response map[string]interface{}
	headers, err := client.doDataJSON(ctx, http.MethodPost, "/v1/chat/completions", body, &response)
	if err != nil {
		return GatewayVerification{}, err
	}
	providerID := strings.TrimSpace(headers.Get("X-Provider"))
	route := strings.TrimSpace(headers.Get("X-Routing-Strategy"))
	requestID := strings.TrimSpace(headers.Get("X-Request-ID"))
	if providerID == "" || route == "" || requestID == "" {
		return GatewayVerification{RequestID: requestID, ProviderID: providerID, Route: route, Message: "Gateway verification headers were incomplete"}, nil
	}
	return GatewayVerification{Verified: true, RequestID: requestID, ProviderID: providerID, Route: route}, nil
}

type gatewayDataResponse[T any] struct {
	Data *[]T `json:"data"`
}

func unwrapGatewayData[T any](response gatewayDataResponse[T], valid func(T) bool) ([]T, error) {
	if response.Data == nil {
		return nil, ErrGatewayAdminInvalidResponse
	}
	items := *response.Data
	if items == nil {
		items = []T{}
	}
	for _, item := range items {
		if !valid(item) {
			return nil, ErrGatewayAdminInvalidResponse
		}
	}
	return items, nil
}

func (client *httpGatewayAdminClient) doJSON(ctx context.Context, method, path string, query url.Values, requestBody, responseBody any) error {
	return client.doJSONAt(ctx, client.adminBaseURL, true, method, path, query, requestBody, responseBody)
}

func (client *httpGatewayAdminClient) doJSONAt(ctx context.Context, baseURL string, authenticate bool, method, path string, query url.Values, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return ErrGatewayAdminInvalidResponse
		}
		body = bytes.NewReader(encoded)
	}

	requestURL := baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return ErrGatewayAdminUnavailable
	}
	if authenticate {
		request.Header.Set("Authorization", "Bearer "+client.adminAPIKey)
		if actor, _ := ctx.Value(gatewayActorContextKey).(string); actor != "" {
			request.Header.Set("X-Admin-Actor", actor)
		}
	}
	if requestID, _ := ctx.Value(gatewayRequestContextKey).(string); requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		if isGatewayAdminTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w for %s %s", ErrGatewayAdminTimeout, method, path)
		}
		return fmt.Errorf("%w for %s %s", ErrGatewayAdminUnavailable, method, path)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.CopyN(io.Discard, response.Body, gatewayAdminMaxResponseBytes+1)
		return &GatewayAdminHTTPError{StatusCode: response.StatusCode, Method: method, Path: path}
	}
	if responseBody == nil {
		_, _ = io.CopyN(io.Discard, response.Body, gatewayAdminMaxResponseBytes+1)
		return nil
	}

	limited := io.LimitReader(response.Body, gatewayAdminMaxResponseBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil {
		return ErrGatewayAdminInvalidResponse
	}
	if len(encoded) > gatewayAdminMaxResponseBytes {
		return ErrGatewayAdminResponseTooLarge
	}
	if len(bytes.TrimSpace(encoded)) == 0 || json.Unmarshal(encoded, responseBody) != nil {
		return ErrGatewayAdminInvalidResponse
	}
	return nil
}

func (client *httpGatewayAdminClient) doDataJSON(ctx context.Context, method, path string, requestBody, responseBody any) (http.Header, error) {
	if client.dataAPIKey == "" {
		return nil, ErrGatewayDataKeyRequired
	}
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return nil, ErrGatewayAdminInvalidResponse
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.dataBaseURL+path, body)
	if err != nil {
		return nil, ErrGatewayAdminUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+client.dataAPIKey)
	request.Header.Set("Accept", "application/json")
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if requestID, _ := ctx.Value(gatewayRequestContextKey).(string); requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		if isGatewayAdminTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w for %s %s", ErrGatewayAdminTimeout, method, path)
		}
		return nil, fmt.Errorf("%w for %s %s", ErrGatewayAdminUnavailable, method, path)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.CopyN(io.Discard, response.Body, gatewayAdminMaxResponseBytes+1)
		return response.Header.Clone(), &GatewayAdminHTTPError{StatusCode: response.StatusCode, Method: method, Path: path}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, gatewayAdminMaxResponseBytes+1))
	if err != nil || len(encoded) == 0 || json.Unmarshal(encoded, responseBody) != nil {
		return response.Header.Clone(), ErrGatewayAdminInvalidResponse
	}
	if len(encoded) > gatewayAdminMaxResponseBytes {
		return response.Header.Clone(), ErrGatewayAdminResponseTooLarge
	}
	return response.Header.Clone(), nil
}

func (client *httpGatewayAdminClient) doTextAt(ctx context.Context, baseURL string, authenticate bool, method, path string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	if err != nil {
		return "", ErrGatewayAdminUnavailable
	}
	if authenticate {
		request.Header.Set("Authorization", "Bearer "+client.adminAPIKey)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		if isGatewayAdminTimeout(err) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("%w for %s %s", ErrGatewayAdminTimeout, method, path)
		}
		return "", fmt.Errorf("%w for %s %s", ErrGatewayAdminUnavailable, method, path)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.CopyN(io.Discard, response.Body, gatewayAdminMaxResponseBytes+1)
		return "", &GatewayAdminHTTPError{StatusCode: response.StatusCode, Method: method, Path: path}
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, gatewayAdminMaxResponseBytes+1))
	if err != nil {
		return "", ErrGatewayAdminInvalidResponse
	}
	if len(encoded) > gatewayAdminMaxResponseBytes {
		return "", ErrGatewayAdminResponseTooLarge
	}
	return string(encoded), nil
}

func resourcePath(collection, id string) string {
	return strings.TrimRight(collection, "/") + "/" + url.PathEscape(strings.TrimSpace(id))
}

func setQuery(query url.Values, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		query.Set(key, value)
	}
}

func setPositiveIntQuery(query url.Values, key string, value int) {
	if value > 0 {
		query.Set(key, strconv.Itoa(value))
	}
}

func isGatewayAdminTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
