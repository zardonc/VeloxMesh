package bff

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "veloxmesh_session"
const adminRequestLogLimit = 1000

type Config struct {
	DevAPIKey                    string
	AllowAdminRegistration       bool
	BootstrapAdminEmail          string
	BootstrapAdminUsername       string
	BootstrapAdminPassword       string
	ProviderName                 string
	BaseURL                      string
	DefaultModel                 string
	Models                       []string
	StatePath                    string
	RedisAddr                    string
	QdrantURL                    string
	QdrantAPIKey                 string
	QdrantBenchmarkCollection    string
	BenchmarkStore               benchmarkStore
	BenchmarkRequestStore        benchmarkRequestStore
	OperationalStore             operationalStore
	EmailOutboxPath              string
	SMTPHost                     string
	SMTPPort                     string
	SMTPUsername                 string
	SMTPPassword                 string
	SMTPPasswordFile             string
	SMTPFrom                     string
	SMTPTLSMode                  string
	SMTPServerName               string
	SMTPTimeout                  time.Duration
	MailSender                   MailSender
	DemoMode                     bool
	TestMode                     bool
	VerificationPepper           []byte
	VerificationSendEmailLimit   int
	VerificationSendIPLimit      int
	VerificationVerifyEmailLimit int
	VerificationVerifyIPLimit    int
	VerificationRateWindow       time.Duration
	SessionTTL                   time.Duration
	SessionCookieSecure          bool
	Now                          func() time.Time
	GatewayAdminURL              string
	GatewayDataURL               string
	GatewayMetricsURL            string
	GatewayAdminAPIKey           string
	GatewayDataAPIKey            string
	GatewayAPITimeout            time.Duration
	GatewayAdminClient           GatewayAdminClient
}

type Server struct {
	mux                 *http.ServeMux
	config              Config
	now                 func() time.Time
	state               *stateStore
	benchmarkStore      benchmarkStore
	benchmarkRequests   benchmarkRequestStore
	operationalStore    operationalStore
	gatewayAdmin        GatewayAdminClient
	gatewayAdminErr     error
	verificationPepper  []byte
	verificationLimiter *fixedWindowLimiter
	mailSender          MailSender
}

type stateStore struct {
	mu         sync.Mutex
	providers  []providerDTO
	routing    []routingDTO
	tenants    []tenantDTO
	apiKeys    []apiKeyDTO
	audit      []auditDTO
	settings   settingsDTO
	users      []userDTO
	sessions   map[string]sessionDTO
	challenges map[string]loginChallengeDTO
}

type providerDTO struct {
	Name          string          `json:"name"`
	BaseURL       string          `json:"baseUrl"`
	DefaultModel  string          `json:"defaultModel"`
	Models        []string        `json:"models"`
	Status        string          `json:"status"`
	P95LatencyMs  int             `json:"p95LatencyMs"`
	SuccessRate   float64         `json:"successRate"`
	RequestsToday int             `json:"requestsToday"`
	Application   *applicationDTO `json:"application,omitempty"`
}

type routingDTO struct {
	Policy      string          `json:"policy"`
	Selector    string          `json:"selector"`
	Target      string          `json:"target"`
	Status      string          `json:"status"`
	Revision    int64           `json:"revision,omitempty"`
	Application *applicationDTO `json:"application,omitempty"`
}

type applicationDTO struct {
	State      string `json:"state"`
	Applied    bool   `json:"applied"`
	Verified   bool   `json:"verified"`
	Revision   int64  `json:"revision"`
	RequestID  string `json:"requestId,omitempty"`
	ProviderID string `json:"providerId,omitempty"`
	Route      string `json:"route,omitempty"`
	Message    string `json:"message,omitempty"`
}

type tenantDTO struct {
	Tenant        string `json:"tenant"`
	Owner         string `json:"owner"`
	OwnerUsername string `json:"ownerUsername,omitempty"`
	DailyQuota    string `json:"dailyQuota"`
	Status        string `json:"status"`
	Revision      int64  `json:"revision,omitempty"`
}

type apiKeyDTO struct {
	ID        string `json:"id,omitempty"`
	Key       string `json:"key"`
	KeyHash   string `json:"keyHash,omitempty"`
	KeyPrefix string `json:"keyPrefix,omitempty"`
	Tenant    string `json:"tenant"`
	Scope     string `json:"scope"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	LastUsed  string `json:"lastUsed"`
}

type settingsDTO struct {
	DefaultProvider       string `json:"defaultProvider"`
	DefaultModel          string `json:"defaultModel"`
	RequestTimeoutSeconds int    `json:"requestTimeoutSeconds"`
	DataRetentionDays     int    `json:"dataRetentionDays"`
	Revision              int64  `json:"revision,omitempty"`
}

type auditDTO struct {
	Time   string `json:"time"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	Result string `json:"result"`
}

type userDTO struct {
	ID           string   `json:"id,omitempty"`
	Email        string   `json:"email"`
	Username     string   `json:"username"`
	Role         string   `json:"role"`
	TenantID     string   `json:"tenantId,omitempty"`
	Verified     bool     `json:"verified"`
	Scopes       []string `json:"scopes"`
	PasswordSalt string   `json:"passwordSalt"`
	PasswordHash string   `json:"passwordHash"`
}

type loginChallengeDTO struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	CodeHash       string    `json:"codeHash"`
	ExpiresAt      time.Time `json:"expiresAt"`
	FailedAttempts int       `json:"failedAttempts"`
	Consumed       bool      `json:"consumed"`
}

type sessionDTO struct {
	UserID    string
	Username  string
	TenantID  string
	Role      string
	ExpiresAt time.Time
}

type requestLogDTO struct {
	RequestID    string  `json:"requestId"`
	Tenant       string  `json:"tenant"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Method       string  `json:"method"`
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	Status       string  `json:"status"`
	LatencyMs    float64 `json:"latencyMs"`
	TTFTMs       float64 `json:"ttftMs"`
	ErrorMessage string  `json:"errorMessage"`
	Timestamp    string  `json:"timestamp"`
}

type providerHealthDTO struct {
	Provider     string  `json:"provider"`
	TargetModel  string  `json:"targetModel"`
	Status       string  `json:"status"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	ErrorRate    float64 `json:"errorRate"`
	TimeoutRate  float64 `json:"timeoutRate"`
	LastChecked  string  `json:"lastChecked"`
}

type benchmarkDTO struct {
	RunID                 string   `json:"runId"`
	MethodID              string   `json:"methodId,omitempty"`
	Method                string   `json:"method"`
	Dataset               string   `json:"dataset"`
	RequestCount          int      `json:"requestCount"`
	Concurrency           int      `json:"concurrency"`
	RequestRate           *float64 `json:"requestRate"`
	WarmUp                int      `json:"warmUp"`
	RepeatedRuns          int      `json:"repeatedRuns"`
	TimeoutSettingSeconds int      `json:"timeoutSettingSeconds"`
	Provider              string   `json:"provider"`
	TargetModel           string   `json:"targetModel"`
	ModelVersion          string   `json:"modelVersion,omitempty"`
	GatewayVersion        string   `json:"gatewayVersion"`
	AvgLatencyMs          *float64 `json:"avgLatencyMs"`
	P50LatencyMs          *float64 `json:"p50LatencyMs"`
	P95LatencyMs          *float64 `json:"p95LatencyMs"`
	P99LatencyMs          *float64 `json:"p99LatencyMs"`
	TTFTMs                *float64 `json:"ttftMs"`
	ThroughputRPS         *float64 `json:"throughputRps"`
	SuccessRatePct        float64  `json:"successRatePct"`
	ErrorRatePct          float64  `json:"errorRatePct"`
	TimeoutRatePct        float64  `json:"timeoutRatePct"`
	ImprovementPct        *float64 `json:"improvementPct"`
	TestDate              string   `json:"testDate"`
	Source                string   `json:"source"`
	RawFilePath           string   `json:"rawFilePath"`
	ExportID              string   `json:"exportId"`
	Status                string   `json:"status"`
	PartialData           bool     `json:"partialData"`
}

type storageStatusDTO struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type benchmarkSnapshot struct {
	Benchmarks  []benchmarkDTO
	Source      string
	GeneratedAt string
	Redis       storageStatusDTO
	Qdrant      storageStatusDTO
}

type benchmarkStore interface {
	Snapshot(ctx context.Context) benchmarkSnapshot
}

type operationalSnapshot struct {
	ProviderHealth []providerHealthDTO
	RequestLogs    []requestLogDTO
	Source         string
	GeneratedAt    string
	Redis          storageStatusDTO
}

type operationalStore interface {
	Snapshot(ctx context.Context) operationalSnapshot
}

type liveOperationalStore struct {
	redisAddr string
}

type liveBenchmarkStore struct {
	redisAddr        string
	qdrantURL        string
	qdrantAPIKey     string
	qdrantCollection string
	httpClient       *http.Client
}

type persistedState struct {
	Providers []providerDTO `json:"providers"`
	Routing   []routingDTO  `json:"routing"`
	Tenants   []tenantDTO   `json:"tenants"`
	APIKeys   []apiKeyDTO   `json:"apiKeys"`
	Audit     []auditDTO    `json:"audit"`
	Settings  settingsDTO   `json:"settings"`
	Users     []userDTO     `json:"users"`
}

func NewServer(config Config) http.Handler {
	config = config.withDefaults()
	store := config.BenchmarkStore
	if store == nil {
		store = newLiveBenchmarkStore(config)
	}
	operationalStore := config.OperationalStore
	if operationalStore == nil {
		operationalStore = liveOperationalStore{redisAddr: config.RedisAddr}
	}
	server := &Server{
		mux:                 http.NewServeMux(),
		config:              config,
		now:                 config.Now,
		benchmarkStore:      store,
		benchmarkRequests:   config.BenchmarkRequestStore,
		operationalStore:    operationalStore,
		gatewayAdmin:        config.GatewayAdminClient,
		verificationPepper:  append([]byte(nil), config.VerificationPepper...),
		verificationLimiter: newFixedWindowLimiter(),
		mailSender:          config.MailSender,
	}
	if server.now == nil {
		server.now = time.Now
	}
	if server.mailSender == nil && smtpConfigurationComplete(config) {
		server.mailSender, _ = newSMTPMailSender(SMTPConfig{
			Host: config.SMTPHost, Port: config.SMTPPort, Username: config.SMTPUsername,
			Password: config.SMTPPassword, From: config.SMTPFrom, TLSMode: config.SMTPTLSMode,
			ServerName: config.SMTPServerName, Timeout: config.SMTPTimeout,
		})
	}
	if len(server.verificationPepper) == 0 {
		server.verificationPepper = mustRandomBytes(32)
	}
	if server.benchmarkRequests == nil {
		server.benchmarkRequests = liveBenchmarkRequestStore{redisAddr: config.RedisAddr}
	}
	if !config.DemoMode && server.gatewayAdmin == nil && strings.TrimSpace(config.GatewayAdminURL) != "" {
		server.gatewayAdmin, server.gatewayAdminErr = NewHTTPGatewayAdminClientWithCredentials(config.GatewayAdminURL, config.GatewayDataURL, config.GatewayMetricsURL, config.GatewayAdminAPIKey, config.GatewayDataAPIKey, config.GatewayAPITimeout, nil)
	}
	server.state = newStateStore(server.config, server.now)
	server.loadState()
	server.ensureBootstrapAdmin()
	server.routes()
	return withCORS(server.mux)
}

func (config Config) withDefaults() Config {
	if config.GatewayAPITimeout <= 0 {
		config.GatewayAPITimeout = 10 * time.Second
	}
	if config.ProviderName == "" {
		config.ProviderName = "sans-primary"
	}
	if config.DefaultModel == "" && len(config.Models) > 0 {
		config.DefaultModel = config.Models[0]
	}
	if config.RedisAddr == "" {
		config.RedisAddr = "127.0.0.1:6379"
	}
	if config.QdrantURL == "" {
		config.QdrantURL = "http://127.0.0.1:6333"
	}
	if config.QdrantBenchmarkCollection == "" {
		config.QdrantBenchmarkCollection = "veloxmesh_benchmarks"
	}
	if config.EmailOutboxPath == "" {
		config.EmailOutboxPath = "tmp/email-outbox.log"
	}
	if config.SMTPPort == "" {
		config.SMTPPort = "587"
	}
	if config.SMTPFrom == "" {
		config.SMTPFrom = config.SMTPUsername
	}
	if config.SMTPTLSMode == "" {
		config.SMTPTLSMode = "starttls"
	}
	if config.SMTPTimeout <= 0 {
		config.SMTPTimeout = 10 * time.Second
	}
	if config.VerificationSendEmailLimit <= 0 {
		config.VerificationSendEmailLimit = 3
	}
	if config.VerificationSendIPLimit <= 0 {
		config.VerificationSendIPLimit = 20
	}
	if config.VerificationVerifyEmailLimit <= 0 {
		config.VerificationVerifyEmailLimit = 10
	}
	if config.VerificationVerifyIPLimit <= 0 {
		config.VerificationVerifyIPLimit = 50
	}
	if config.VerificationRateWindow <= 0 {
		config.VerificationRateWindow = defaultRateLimitWindow
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 8 * time.Hour
	}
	return config
}

func (server *Server) gatewayAdminUnavailable(w http.ResponseWriter) bool {
	if server.config.DemoMode {
		return false
	}
	if server.gatewayAdmin != nil {
		return false
	}
	detail := "VeloxMesh Admin API is not configured"
	if server.gatewayAdminErr != nil {
		detail = "VeloxMesh Admin API configuration is invalid"
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]any{
		"error":       "gateway_admin_unavailable",
		"message":     detail,
		"source":      "veloxmesh-admin",
		"partialData": true,
		"warnings":    []string{detail},
	})
	return true
}

func writeGatewayAdminError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := "gateway_admin_error"
	if errors.Is(err, ErrGatewayAdminTimeout) {
		status, code = http.StatusGatewayTimeout, "gateway_admin_timeout"
	} else if errors.Is(err, ErrGatewayAdminUnavailable) {
		status, code = http.StatusServiceUnavailable, "gateway_admin_unavailable"
	} else {
		var upstream *GatewayAdminHTTPError
		if errors.As(err, &upstream) {
			switch {
			case upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden:
				status, code = http.StatusBadGateway, "gateway_admin_auth_failed"
			case upstream.StatusCode >= 400 && upstream.StatusCode < 500:
				status, code = upstream.StatusCode, "gateway_admin_rejected"
			default:
				status, code = http.StatusBadGateway, "gateway_admin_bad_gateway"
			}
		}
	}
	writeJSON(w, status, map[string]any{
		"error":       code,
		"message":     "VeloxMesh Admin API request failed",
		"source":      "veloxmesh-admin",
		"partialData": true,
		"warnings":    []string{"The requested Gateway management data is unavailable or incomplete"},
	})
}

func (server *Server) routes() {
	server.mux.HandleFunc("GET /health", server.handleGatewayHealth)
	server.mux.HandleFunc("GET /bff/health", server.handleBFFHealth)
	server.mux.HandleFunc("POST /bff/auth/register", server.handleAuthRegister)
	server.mux.HandleFunc("POST /bff/auth/customer/register", server.handleCustomerRegister)
	server.mux.HandleFunc("POST /bff/auth/login", server.handleAuthLogin)
	server.mux.HandleFunc("POST /bff/auth/verify-login", server.handleAuthVerifyLogin)
	server.mux.HandleFunc("POST /bff/auth/verify", server.handleAuthVerifyLogin)
	server.mux.HandleFunc("POST /bff/auth/logout", server.handleAuthLogout)
	server.mux.HandleFunc("GET /bff/session", server.handleCurrentSession)
	server.mux.HandleFunc("GET /bff/auth/session", server.handleCurrentSession)
	server.mux.HandleFunc("GET /bff/admin/summary", server.requireAdmin(server.handleAdminSummary))
	server.mux.HandleFunc("GET /bff/admin/session", server.requireAdmin(server.handleAdminSession))
	server.mux.HandleFunc("GET /bff/admin/providers", server.requireAdmin(server.handleAdminProviders))
	server.mux.HandleFunc("POST /bff/admin/providers", server.requireAdmin(server.handleCreateProvider))
	server.mux.HandleFunc("PUT /bff/admin/providers/{name}", server.requireAdmin(server.handleUpdateProvider))
	server.mux.HandleFunc("DELETE /bff/admin/providers/{name}", server.requireAdmin(server.handleDeleteProvider))
	server.mux.HandleFunc("GET /bff/admin/routing", server.requireAdmin(server.handleAdminRouting))
	server.mux.HandleFunc("POST /bff/admin/routing", server.requireAdmin(server.handleCreateRouting))
	server.mux.HandleFunc("PUT /bff/admin/routing/{policy}", server.requireAdmin(server.handleUpdateRouting))
	server.mux.HandleFunc("DELETE /bff/admin/routing/{policy}", server.requireAdmin(server.handleDeleteRouting))
	server.mux.HandleFunc("POST /bff/admin/runtime/verify", server.requireAdmin(server.handleRuntimeVerification))
	server.mux.HandleFunc("GET /bff/admin/tenants", server.requireAdmin(server.handleAdminTenants))
	server.mux.HandleFunc("POST /bff/admin/tenants", server.requireAdmin(server.handleCreateTenant))
	server.mux.HandleFunc("PUT /bff/admin/tenants/{tenant}", server.requireAdmin(server.handleUpdateTenant))
	server.mux.HandleFunc("DELETE /bff/admin/tenants/{tenant}", server.requireAdmin(server.handleDeleteTenant))
	server.mux.HandleFunc("GET /bff/admin/api-keys", server.requireAdmin(server.handleAdminAPIKeys))
	server.mux.HandleFunc("POST /bff/admin/api-keys", server.requireAdmin(server.handleCreateAPIKey))
	server.mux.HandleFunc("DELETE /bff/admin/api-keys/{key}", server.requireAdmin(server.handleDeleteAPIKey))
	server.mux.HandleFunc("GET /bff/admin/audit", server.requireAdmin(server.handleAdminAudit))
	server.mux.HandleFunc("GET /bff/admin/audit.csv", server.requireAdmin(server.handleAuditCSV))
	server.mux.HandleFunc("GET /bff/admin/settings", server.requireAdmin(server.handleAdminSettings))
	server.mux.HandleFunc("PUT /bff/admin/settings", server.requireAdmin(server.handleUpdateSettings))
	server.mux.HandleFunc("GET /bff/admin/requests", server.requireAdmin(server.handleAdminRequests))
	server.mux.HandleFunc("GET /bff/admin/provider-health", server.requireAdmin(server.handleAdminProviderHealth))
	server.mux.HandleFunc("GET /bff/admin/request-logs", server.requireAdmin(server.handleAdminRequestLogs))
	server.mux.HandleFunc("GET /bff/admin/benchmarks", server.requireAdmin(server.handleAdminBenchmarks))
	server.mux.HandleFunc("GET /bff/admin/benchmarks/raw.csv", server.requireAdmin(server.handleAdminBenchmarkRawCSV))
	server.mux.HandleFunc("GET /bff/admin/benchmarks/export.zip", server.requireAdmin(server.handleAdminBenchmarkExportZIP))
	server.mux.HandleFunc("GET /bff/customer/summary", server.requireCustomer(server.handleCustomerSummary))
	server.mux.HandleFunc("GET /bff/customer/usage", server.requireCustomer(server.handleCustomerUsage))
	server.mux.HandleFunc("GET /bff/customer/requests", server.requireCustomer(server.handleCustomerRequests))
	server.mux.HandleFunc("GET /bff/customer/api-keys", server.requireCustomer(server.handleCustomerAPIKeys))
	server.mux.HandleFunc("POST /bff/customer/api-keys", server.requireCustomer(server.handleCreateCustomerAPIKey))
	server.mux.HandleFunc("DELETE /bff/customer/api-keys/{id}", server.requireCustomer(server.handleDeleteCustomerAPIKey))
}

func (server *Server) handleGatewayHealth(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		health, err := server.gatewayAdmin.GetHealth(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		readiness, err := server.gatewayAdmin.GetReadiness(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		topology, err := server.gatewayAdmin.GetTopology(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": health.Status, "service": "gateway", "readiness": readiness, "topology": topology, "source": "veloxmesh-admin", "partialData": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "gateway",
	})
}

func (server *Server) handleBFFHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "ai-gateway-dashboard-bff",
		"time":    server.now().UTC().Format(time.RFC3339),
	})
}

func (server *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	email := strings.TrimSpace(input.Email)
	username := strings.TrimSpace(input.Username)
	password := strings.TrimSpace(input.Password)
	if email == "" || username == "" || strings.TrimSpace(password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, username, and password are required"})
		return
	}
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email must be valid"})
		return
	}
	if len([]rune(username)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username must be at least 4 characters"})
		return
	}
	if len([]rune(password)) < 4 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 4 characters"})
		return
	}
	role, scopes, ok := authRole(input.Role)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be Admin or Customer"})
		return
	}
	if role == "Customer" {
		server.registerCustomer(w, r, customerRegistrationInput{
			Email:           email,
			Username:        username,
			Organization:    username,
			Password:        password,
			ConfirmPassword: password,
		})
		return
	}
	if !server.config.AllowAdminRegistration {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Admin accounts cannot be created through public registration"})
		return
	}

	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	if server.findUserLocked(username) != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account registration unavailable"})
		return
	}
	if server.findUserLocked(email) != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account registration unavailable"})
		return
	}

	passwordHash, err := hashPasswordAdaptive(password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
		return
	}
	user := userDTO{
		ID:           stableUserID(username),
		Email:        email,
		Username:     username,
		Role:         role,
		Scopes:       scopes,
		PasswordHash: passwordHash,
	}
	server.state.users = append(server.state.users, user)
	server.appendAuditLocked(username, "Registered "+strings.ToLower(role)+" account", "Success")
	server.saveStateLocked()
	writeJSON(w, http.StatusCreated, map[string]any{
		"status":  "registered",
		"message": "Account created. Please sign in.",
		"user":    user.Username,
		"role":    user.Role,
		"scopes":  append([]string(nil), user.Scopes...),
	})
}

type customerRegistrationInput struct {
	Email           string `json:"email"`
	Username        string `json:"username"`
	Organization    string `json:"organization"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

func (server *Server) handleCustomerRegister(w http.ResponseWriter, r *http.Request) {
	var input customerRegistrationInput
	if !decodeJSON(w, r, &input) {
		return
	}
	server.registerCustomer(w, r, input)
}

func (server *Server) registerCustomer(w http.ResponseWriter, r *http.Request, input customerRegistrationInput) {
	email := strings.TrimSpace(input.Email)
	username := strings.TrimSpace(input.Username)
	organization := strings.TrimSpace(input.Organization)
	password := strings.TrimSpace(input.Password)
	if email == "" || username == "" || organization == "" || password == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "email, username, organization, and password are required"})
		return
	}
	if !strings.Contains(email, "@") {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "email must be valid"})
		return
	}
	if len([]rune(username)) < 4 || len([]rune(password)) < 4 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "username and password must be at least 4 characters"})
		return
	}
	if input.ConfirmPassword != "" && input.Password != input.ConfirmPassword {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "password confirmation does not match"})
		return
	}
	if !server.verificationDeliveryAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verification_delivery_unavailable", "message": "Email verification is temporarily unavailable"})
		return
	}
	if !server.allowVerificationSend(w, r, email) {
		return
	}

	server.state.mu.Lock()
	if server.findUserLocked(username) != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account registration unavailable"})
		return
	}
	if server.findUserLocked(email) != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "account registration unavailable"})
		return
	}

	userSuffix, userErr := randomHex(8)
	tenantSuffix, tenantErr := randomHex(8)
	passwordHash, hashErr := hashPasswordAdaptive(password)
	challengeID, challengeErr := randomHex(16)
	code, codeErr := randomDigits(6)
	if userErr != nil || tenantErr != nil || hashErr != nil || challengeErr != nil || codeErr != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create customer account"})
		return
	}
	tenantID := "tenant-" + tenantSuffix
	user := userDTO{
		ID:           "user-" + userSuffix,
		Email:        email,
		Username:     username,
		Role:         "Customer",
		TenantID:     tenantID,
		Verified:     false,
		Scopes:       []string{"gateway:invoke"},
		PasswordHash: passwordHash,
	}
	tenant := tenantDTO{
		Tenant:        tenantID,
		Owner:         organization,
		OwnerUsername: username,
		DailyQuota:    "5,000",
		Status:        "Healthy",
	}
	audit := auditDTO{
		Time:   server.now().Format("15:04"),
		Actor:  username,
		Action: "Registered customer account and tenant " + tenantID,
		Result: "Success",
	}
	candidate := server.persistedStateLocked()
	candidate.Tenants = append(candidate.Tenants, tenant)
	candidate.Users = append(candidate.Users, user)
	candidate.Audit = append([]auditDTO{audit}, candidate.Audit...)
	if err := server.writePersistedState(candidate); err != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist customer account"})
		return
	}
	server.state.tenants = candidate.Tenants
	server.state.users = candidate.Users
	server.state.audit = candidate.Audit
	server.state.challenges[challengeID] = newLoginChallenge(challengeID, username, code, server.verificationPepper, server.now())
	server.state.mu.Unlock()

	if err := server.sendVerificationEmail(user, code); err != nil {
		server.state.mu.Lock()
		delete(server.state.challenges, challengeID)
		server.state.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verification_delivery_unavailable", "message": "Email verification is temporarily unavailable"})
		return
	}
	response := map[string]any{
		"status":               "verification_required",
		"message":              "Account created. Verify the email address to continue.",
		"user":                 username,
		"role":                 "Customer",
		"tenantId":             tenantID,
		"scopes":               []string{"gateway:invoke"},
		"verificationRequired": true,
		"challengeId":          challengeID,
		"delivery":             "email",
	}
	if server.developmentVerificationEnabled() {
		response["devCode"] = code
	}
	writeJSON(w, http.StatusCreated, response)
}

func (server *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Identifier string `json:"identifier"`
		Password   string `json:"password"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	identifier := strings.TrimSpace(input.Identifier)
	if identifier == "" || strings.TrimSpace(input.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "identifier and password are required"})
		return
	}

	server.state.mu.Lock()
	user := server.findUserLocked(identifier)
	if user == nil || !passwordMatches(*user, input.Password) {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}
	if !strings.HasPrefix(user.PasswordHash, "$2") {
		if upgraded, err := hashPasswordAdaptive(input.Password); err == nil {
			user.PasswordHash = upgraded
			user.PasswordSalt = ""
			server.saveStateLocked()
		}
	}
	userCopy := *user
	if !server.verificationDeliveryAvailable() {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verification_delivery_unavailable", "message": "Email verification is temporarily unavailable"})
		return
	}
	if !server.allowVerificationSend(w, r, userCopy.Email) {
		server.state.mu.Unlock()
		return
	}
	challengeID, code, err := server.createLoginChallengeLocked(userCopy)
	if err != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create verification code"})
		return
	}
	server.appendAuditLocked(userCopy.Username, "Requested email verification code", "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()

	if err := server.sendVerificationEmail(userCopy, code); err != nil {
		server.state.mu.Lock()
		delete(server.state.challenges, challengeID)
		server.state.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "verification_delivery_unavailable", "message": "Email verification is temporarily unavailable"})
		return
	}
	response := map[string]any{
		"verificationRequired": true,
		"challengeId":          challengeID,
		"delivery":             "email",
		"message":              "Verification code sent.",
	}
	if server.developmentVerificationEnabled() {
		response["devCode"] = code
	}
	writeJSON(w, http.StatusOK, response)
}

func (server *Server) handleAuthVerifyLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ChallengeID string `json:"challengeId"`
		Code        string `json:"code"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	challengeID := strings.TrimSpace(input.ChallengeID)
	code := strings.TrimSpace(input.Code)
	if challengeID == "" || code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "challengeId and code are required"})
		return
	}
	if len(code) != 6 || !isDigitsOnly(code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "verification code must be 6 digits"})
		return
	}

	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	challenge, ok := server.state.challenges[challengeID]
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired verification code"})
		return
	}
	user := server.findUserLocked(challenge.Username)
	email := "unknown"
	if user != nil {
		email = user.Email
	}
	if !server.allowVerificationAttempt(w, r, email, challengeID) {
		return
	}
	result := challenge.verify(code, server.verificationPepper, server.now())
	if result != verificationAccepted {
		if result == verificationExpired || result == verificationExhausted || result == verificationConsumed {
			delete(server.state.challenges, challengeID)
		} else {
			server.state.challenges[challengeID] = challenge
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired verification code"})
		return
	}
	if user == nil {
		delete(server.state.challenges, challengeID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired verification code"})
		return
	}
	delete(server.state.challenges, challengeID)
	user.Verified = true
	server.appendAuditLocked(user.Username, "Signed in to AI gateway dashboard", "Success")
	server.saveStateLocked()
	server.createSessionLocked(w, *user)
	writeJSON(w, http.StatusOK, sessionResponse(*user))
}

func (server *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	server.state.mu.Lock()
	if err == nil {
		delete(server.state.sessions, cookie.Value)
	}
	server.state.mu.Unlock()
	server.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed out"})
}

func (server *Server) handleCurrentSession(w http.ResponseWriter, r *http.Request) {
	user, ok := server.userFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(user))
}

func (server *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := server.userFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
			return
		}
		if user.Role != "Admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
			return
		}
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID, _ = randomHex(16)
		}
		actor := firstNonEmpty([]string{user.Email, user.Username}, "admin")
		w.Header().Set("X-Request-ID", requestID)
		ctx := WithGatewayOperation(r.Context(), actor, requestID)
		next(w, r.WithContext(ctx))
	}
}

func (server *Server) requireCustomer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := server.userFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
			return
		}
		if user.Role != "Customer" || strings.TrimSpace(user.TenantID) == "" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Customer access required"})
			return
		}
		next(w, r)
	}
}

type customerSummaryDTO struct {
	TenantID     string         `json:"tenantId"`
	Requests     int            `json:"requests"`
	InputTokens  int            `json:"inputTokens"`
	OutputTokens int            `json:"outputTokens"`
	TotalTokens  int            `json:"totalTokens"`
	AvgLatencyMs float64        `json:"avgLatencyMs"`
	P95LatencyMs float64        `json:"p95LatencyMs"`
	SuccessRate  float64        `json:"successRate"`
	ErrorRate    float64        `json:"errorRate"`
	TimeoutRate  float64        `json:"timeoutRate"`
	ModelUsage   map[string]int `json:"modelUsage"`
	Source       string         `json:"source"`
	GeneratedAt  string         `json:"generatedAt"`
	PartialData  bool           `json:"partialData"`
}

func (server *Server) handleCustomerSummary(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	snapshot := server.operationalStore.Snapshot(r.Context())
	logs := filterCustomerLogs(snapshot.RequestLogs, user.TenantID, r)
	writeJSON(w, http.StatusOK, customerSummary(user.TenantID, snapshot, logs))
}

func (server *Server) handleCustomerUsage(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	snapshot := server.operationalStore.Snapshot(r.Context())
	logs := filterCustomerLogs(snapshot.RequestLogs, user.TenantID, r)
	type usageBucket struct {
		Date         string  `json:"date"`
		Requests     int     `json:"requests"`
		TotalTokens  int     `json:"totalTokens"`
		AvgLatencyMs float64 `json:"avgLatencyMs"`
	}
	type modelBucket struct {
		Model       string `json:"model"`
		Requests    int    `json:"requests"`
		TotalTokens int    `json:"totalTokens"`
	}
	type accumulator struct {
		Requests int
		Tokens   int
		Latency  float64
	}
	byDate := map[string]*accumulator{}
	byModel := map[string]*accumulator{}
	for _, log := range logs {
		date := log.Timestamp
		if parsed, err := time.Parse(time.RFC3339, log.Timestamp); err == nil {
			date = parsed.UTC().Format("2006-01-02")
		} else if len(date) >= 10 {
			date = date[:10]
		}
		if date == "" {
			date = "unknown"
		}
		day := byDate[date]
		if day == nil {
			day = &accumulator{}
			byDate[date] = day
		}
		model := byModel[log.Model]
		if model == nil {
			model = &accumulator{}
			byModel[log.Model] = model
		}
		tokens := log.InputTokens + log.OutputTokens
		day.Requests++
		day.Tokens += tokens
		day.Latency += log.LatencyMs
		model.Requests++
		model.Tokens += tokens
	}
	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	series := make([]usageBucket, 0, len(dates))
	for _, date := range dates {
		value := byDate[date]
		series = append(series, usageBucket{Date: date, Requests: value.Requests, TotalTokens: value.Tokens, AvgLatencyMs: roundMetric(value.Latency / float64(value.Requests))})
	}
	models := make([]modelBucket, 0, len(byModel))
	for model, value := range byModel {
		models = append(models, modelBucket{Model: model, Requests: value.Requests, TotalTokens: value.Tokens})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Requests > models[j].Requests })
	writeJSON(w, http.StatusOK, map[string]any{
		"tenantId":    user.TenantID,
		"summary":     customerSummary(user.TenantID, snapshot, logs),
		"series":      series,
		"models":      models,
		"source":      customerSource(snapshot.Source),
		"generatedAt": snapshot.GeneratedAt,
		"partialData": customerPartial(snapshot.Source),
	})
}

func (server *Server) handleCustomerRequests(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	snapshot := server.operationalStore.Snapshot(r.Context())
	logs := filterCustomerLogs(snapshot.RequestLogs, user.TenantID, r)
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].Timestamp > logs[j].Timestamp })
	page := boundedQueryInt(r, "page", 1, 1, 100000)
	pageSize := boundedQueryInt(r, "pageSize", 25, 1, 100)
	start := (page - 1) * pageSize
	if start > len(logs) {
		start = len(logs)
	}
	end := start + pageSize
	if end > len(logs) {
		end = len(logs)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenantId":    user.TenantID,
		"requests":    logs[start:end],
		"page":        page,
		"pageSize":    pageSize,
		"total":       len(logs),
		"source":      customerSource(snapshot.Source),
		"generatedAt": snapshot.GeneratedAt,
		"partialData": customerPartial(snapshot.Source),
	})
}

func (server *Server) handleCustomerAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	keys := make([]map[string]any, 0)
	for _, key := range server.state.apiKeys {
		if key.Tenant != user.TenantID {
			continue
		}
		keys = append(keys, map[string]any{
			"id":        key.ID,
			"maskedKey": key.Key,
			"scope":     key.Scope,
			"status":    firstNonEmpty([]string{key.Status}, "Active"),
			"createdAt": key.CreatedAt,
			"lastUsed":  key.LastUsed,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenantId": user.TenantID, "keys": keys})
}

func (server *Server) handleCreateCustomerAPIKey(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	var input struct {
		Scope string `json:"scope"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = "gateway:invoke"
	}
	if scope != "gateway:invoke" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Customer API keys require gateway:invoke scope"})
		return
	}
	idSuffix, idErr := randomHex(8)
	secretSuffix, secretErr := randomHex(24)
	if idErr != nil || secretErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create API key"})
		return
	}
	secret := "vx_live_" + secretSuffix
	key := apiKeyDTO{
		ID:        "key-" + idSuffix,
		Key:       maskAPIKeySecret(secret),
		KeyHash:   hashAPIKeySecret(secret),
		KeyPrefix: apiKeyPrefix(secret),
		Tenant:    user.TenantID,
		Scope:     scope,
		Status:    "Active",
		CreatedAt: server.now().UTC().Format(time.RFC3339),
		LastUsed:  "never",
	}
	server.state.mu.Lock()
	candidate := server.persistedStateLocked()
	candidate.APIKeys = append(candidate.APIKeys, key)
	if err := server.writePersistedState(candidate); err != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist API key"})
		return
	}
	server.state.apiKeys = candidate.APIKeys
	server.appendAuditLocked(user.Username, "Created Customer API key "+key.ID, "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        key.ID,
		"key":       secret,
		"maskedKey": key.Key,
		"scope":     key.Scope,
		"status":    key.Status,
		"createdAt": key.CreatedAt,
	})
}

func (server *Server) handleDeleteCustomerAPIKey(w http.ResponseWriter, r *http.Request) {
	user, _ := server.userFromRequest(r)
	id := strings.TrimSpace(r.PathValue("id"))
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, key := range server.state.apiKeys {
		if key.ID != id || key.Tenant != user.TenantID {
			continue
		}
		candidate := server.persistedStateLocked()
		candidate.APIKeys = append(candidate.APIKeys[:index], candidate.APIKeys[index+1:]...)
		if err := server.writePersistedState(candidate); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke API key"})
			return
		}
		server.state.apiKeys = candidate.APIKeys
		server.appendAuditLocked(user.Username, "Revoked Customer API key "+id, "Success")
		server.saveStateLocked()
		writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
}

func filterCustomerLogs(logs []requestLogDTO, tenantID string, r *http.Request) []requestLogDTO {
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	from := parseQueryTime(r.URL.Query().Get("from"))
	to := parseQueryTime(r.URL.Query().Get("to"))
	result := make([]requestLogDTO, 0)
	for _, log := range logs {
		if log.Tenant != tenantID {
			continue
		}
		if status != "" && strings.ToLower(log.Status) != status {
			continue
		}
		if model != "" && log.Model != model {
			continue
		}
		if !from.IsZero() || !to.IsZero() {
			timestamp, err := time.Parse(time.RFC3339, log.Timestamp)
			if err != nil || (!from.IsZero() && timestamp.Before(from)) || (!to.IsZero() && timestamp.After(to)) {
				continue
			}
		}
		result = append(result, log)
	}
	return result
}

func customerSummary(tenantID string, snapshot operationalSnapshot, logs []requestLogDTO) customerSummaryDTO {
	summary := customerSummaryDTO{TenantID: tenantID, ModelUsage: map[string]int{}, Source: customerSource(snapshot.Source), GeneratedAt: snapshot.GeneratedAt, PartialData: customerPartial(snapshot.Source)}
	latencies := make([]float64, 0, len(logs))
	successes, errors, timeouts := 0, 0, 0
	for _, log := range logs {
		summary.Requests++
		summary.InputTokens += log.InputTokens
		summary.OutputTokens += log.OutputTokens
		summary.AvgLatencyMs += log.LatencyMs
		latencies = append(latencies, log.LatencyMs)
		summary.ModelUsage[log.Model]++
		switch strings.ToLower(log.Status) {
		case "success", "passed":
			successes++
		case "timeout", "timed_out":
			timeouts++
		default:
			errors++
		}
	}
	summary.TotalTokens = summary.InputTokens + summary.OutputTokens
	if summary.Requests > 0 {
		summary.AvgLatencyMs = roundMetric(summary.AvgLatencyMs / float64(summary.Requests))
		sort.Float64s(latencies)
		index := int(float64(len(latencies)-1) * 0.95)
		summary.P95LatencyMs = roundMetric(latencies[index])
		summary.SuccessRate = roundMetric(float64(successes) * 100 / float64(summary.Requests))
		summary.ErrorRate = roundMetric(float64(errors) * 100 / float64(summary.Requests))
		summary.TimeoutRate = roundMetric(float64(timeouts) * 100 / float64(summary.Requests))
	}
	return summary
}

func boundedQueryInt(r *http.Request, key string, fallback int, minimum int, maximum int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil {
		return fallback
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func parseQueryTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339, strings.TrimSpace(value))
	return parsed
}

func customerSource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "empty"
	}
	return source
}

func customerPartial(source string) bool {
	normalized := strings.ToLower(source)
	return strings.Contains(normalized, "partial") || strings.Contains(normalized, "fallback")
}

func roundMetric(value float64) float64 {
	parsed, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", value), 64)
	return parsed
}

func maskAPIKeySecret(secret string) string {
	return maskAPIKeyPrefix(apiKeyPrefix(secret))
}

func maskAPIKeyPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "********"
	}
	return prefix + "...********"
}

func apiKeyPrefix(secret string) string {
	secret = strings.TrimSpace(secret)
	if index := strings.LastIndex(secret, "_"); index >= 0 {
		return secret[:index+1]
	}
	if len(secret) > 8 {
		return secret[:8]
	}
	return secret
}

func hashAPIKeySecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func migrateLegacyAPIKeys(keys []apiKeyDTO, now func() time.Time) ([]apiKeyDTO, bool) {
	migrated := false
	result := append([]apiKeyDTO(nil), keys...)
	for index := range result {
		key := &result[index]
		masked := key.Key == "****" || strings.Contains(key.Key, "...")
		if !masked && key.Key != "" {
			key.KeyPrefix = apiKeyPrefix(key.Key)
			if key.KeyHash == "" {
				key.KeyHash = hashAPIKeySecret(key.Key)
			}
			key.Key = maskAPIKeyPrefix(key.KeyPrefix)
			migrated = true
		}
		if key.KeyPrefix == "" {
			switch {
			case strings.HasPrefix(key.Key, "vx_live"):
				key.KeyPrefix = "vx_live_"
			case strings.HasPrefix(key.Key, "vx_admin"):
				key.KeyPrefix = "vx_admin_"
			}
			if key.KeyPrefix != "" {
				key.Key = maskAPIKeyPrefix(key.KeyPrefix)
				migrated = true
			}
		}
		if key.ID == "" {
			identity := hashAPIKeySecret(key.Tenant + "\x00" + key.Scope + "\x00" + key.Key)
			key.ID = "key-legacy-" + identity[:12]
			migrated = true
		}
		if key.Status == "" {
			key.Status = "Active"
			migrated = true
		}
		if key.CreatedAt == "" {
			key.CreatedAt = now().UTC().Format(time.RFC3339)
			migrated = true
		}
	}
	return result, migrated
}

func (server *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	user, ok := server.userFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(user))
}

func (server *Server) handleAdminProviders(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		items, err := server.gatewayAdmin.ListProviders(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		providers := make([]providerDTO, 0, len(items))
		for _, item := range items {
			status := "healthy"
			if !item.Enabled {
				status = "disabled"
			}
			providers = append(providers, providerDTO{Name: item.ID, BaseURL: item.BaseURL, DefaultModel: item.DefaultModel, Models: item.Models, Status: status})
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": providers, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}
	server.state.mu.Lock()
	providers := append([]providerDTO(nil), server.state.providers...)
	server.state.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"providers": providers,
	})
}

func (server *Server) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string   `json:"name"`
		BaseURL      string   `json:"baseUrl"`
		DefaultModel string   `json:"defaultModel"`
		Models       []string `json:"models"`
		APIKey       string   `json:"apiKey"`
		Type         string   `json:"type"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.BaseURL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and baseUrl are required"})
		return
	}
	if len(input.Models) == 0 && strings.TrimSpace(input.DefaultModel) != "" {
		input.Models = []string{input.DefaultModel}
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		if strings.TrimSpace(input.APIKey) == "" {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "apiKey is required when creating a real Gateway provider"})
			return
		}
		providerType := strings.TrimSpace(input.Type)
		if providerType == "" {
			providerType = "openai-compatible"
		}
		defaultModel, secret := strings.TrimSpace(input.DefaultModel), strings.TrimSpace(input.APIKey)
		item, err := server.gatewayAdmin.CreateProvider(r.Context(), GatewayProviderMutation{ID: strings.TrimSpace(input.Name), Name: strings.TrimSpace(input.Name), Type: providerType, BaseURL: strings.TrimSpace(input.BaseURL), Enabled: true, APIKey: &secret, Models: input.Models, DefaultModel: &defaultModel})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		application, verifyErr := server.verifyProviderApplication(r.Context(), item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": verifyErr.Error(), "application": application})
			return
		}
		response := mapGatewayProvider(item)
		response.Application = application
		writeJSON(w, http.StatusCreated, response)
		return
	}
	provider := providerDTO{
		Name:          strings.TrimSpace(input.Name),
		BaseURL:       strings.TrimSpace(input.BaseURL),
		DefaultModel:  strings.TrimSpace(input.DefaultModel),
		Models:        input.Models,
		Status:        "healthy",
		P95LatencyMs:  900,
		SuccessRate:   98.7,
		RequestsToday: 0,
	}
	server.state.mu.Lock()
	server.state.providers = append(server.state.providers, provider)
	server.appendAuditLocked("admin", "Created provider "+provider.Name, "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, provider)
}

func (server *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	var input struct {
		BaseURL      string   `json:"baseUrl"`
		DefaultModel string   `json:"defaultModel"`
		Models       []string `json:"models"`
		Status       string   `json:"status"`
		APIKey       *string  `json:"apiKey,omitempty"`
		Type         string   `json:"type"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.BaseURL) == "" || strings.TrimSpace(input.DefaultModel) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "baseUrl and defaultModel are required"})
		return
	}
	if len(input.Models) == 0 {
		input.Models = []string{strings.TrimSpace(input.DefaultModel)}
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "healthy"
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		providers, err := server.gatewayAdmin.ListProviders(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		var current *GatewayProvider
		for index := range providers {
			if providers[index].ID == name || providers[index].Name == name {
				current = &providers[index]
				break
			}
		}
		if current == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
			return
		}
		providerType := strings.TrimSpace(input.Type)
		if providerType == "" {
			providerType = current.Type
		}
		defaultModel := strings.TrimSpace(input.DefaultModel)
		item, err := server.gatewayAdmin.UpdateProvider(r.Context(), current.ID, GatewayProviderMutation{Name: current.Name, Type: providerType, BaseURL: strings.TrimSpace(input.BaseURL), Enabled: !strings.EqualFold(input.Status, "disabled"), APIKey: input.APIKey, Models: input.Models, DefaultModel: &defaultModel, Revision: current.Revision})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		application, verifyErr := server.verifyProviderApplication(r.Context(), item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": verifyErr.Error(), "application": application})
			return
		}
		response := mapGatewayProvider(item)
		response.Application = application
		writeJSON(w, http.StatusOK, response)
		return
	}

	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index := range server.state.providers {
		if server.state.providers[index].Name == name {
			server.state.providers[index].BaseURL = strings.TrimSpace(input.BaseURL)
			server.state.providers[index].DefaultModel = strings.TrimSpace(input.DefaultModel)
			server.state.providers[index].Models = input.Models
			server.state.providers[index].Status = strings.TrimSpace(input.Status)
			server.appendAuditLocked("admin", "Updated provider "+name, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, server.state.providers[index])
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
}

func (server *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		if err := server.gatewayAdmin.DeleteProvider(r.Context(), name); err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, provider := range server.state.providers {
		if provider.Name == name {
			server.state.providers = append(server.state.providers[:index], server.state.providers[index+1:]...)
			server.appendAuditLocked("admin", "Deleted provider "+name, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
}

func mapGatewayProvider(item GatewayProvider) providerDTO {
	status := "healthy"
	if !item.Enabled {
		status = "disabled"
	}
	return providerDTO{Name: item.ID, BaseURL: item.BaseURL, DefaultModel: item.DefaultModel, Models: item.Models, Status: status}
}

func (server *Server) verifyProviderApplication(ctx context.Context, written GatewayProvider) (*applicationDTO, error) {
	application := &applicationDTO{State: "applied", Applied: true, Revision: written.Revision, ProviderID: written.ID}
	readback, err := server.gatewayAdmin.GetProvider(ctx, written.ID)
	if err != nil || readback.Revision != written.Revision || readback.UpdatedAt.Before(written.UpdatedAt) {
		application.State = "failed"
		application.Applied = false
		application.Message = "provider readback did not confirm the persisted revision"
		return application, errors.New(application.Message)
	}
	model := firstNonEmpty([]string{readback.DefaultModel}, "")
	if model == "" && len(readback.Models) > 0 {
		model = readback.Models[0]
	}
	if model == "" {
		application.State = "warning"
		application.Message = "provider is active but no model was available for live verification"
		return application, nil
	}
	verification, err := server.gatewayAdmin.VerifyModels(ctx, model)
	application.RequestID = verification.RequestID
	if err != nil || !verification.Verified {
		application.State = "warning"
		application.Message = "provider is active but the live model verification did not complete"
		if verification.Message != "" {
			application.Message = verification.Message
		}
		return application, nil
	}
	application.State = "verified"
	application.Verified = true
	return application, nil
}

func (server *Server) handleAdminRouting(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		item, err := server.gatewayAdmin.GetRouting(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		rules := []routingDTO{{Policy: item.ID, Selector: item.Strategy, Target: item.DefaultProvider, Status: "Active", Revision: item.Revision}}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}, "singleton": true, "revision": item.Revision})
		return
	}
	server.state.mu.Lock()
	rules := append([]routingDTO(nil), server.state.routing...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"rules":       rules,
		"source":      "dashboard-state",
		"partialData": true,
		"warnings":    []string{"VeloxMesh Admin API is not connected"},
	})
}

func (server *Server) handleCreateRouting(w http.ResponseWriter, r *http.Request) {
	var input routingDTO
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Policy) == "" || strings.TrimSpace(input.Selector) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy and selector are required"})
		return
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "Draft"
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		current, err := server.gatewayAdmin.GetRouting(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		item, err := server.gatewayAdmin.PutRouting(r.Context(), GatewayRoutingUpdateRequest{Strategy: strings.TrimSpace(input.Selector), DefaultProvider: strings.TrimSpace(input.Target), FallbackEnabled: current.FallbackEnabled, MaxAttempts: positiveOrDefault(current.MaxAttempts, 2), Composite: current.Composite, Revision: input.Revision})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		observed := current
		observed.Revision = input.Revision
		application, verifyErr := server.verifyRoutingApplication(r.Context(), observed, item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": verifyErr.Error(), "application": application})
			return
		}
		writeJSON(w, http.StatusCreated, routingDTO{Policy: item.ID, Selector: item.Strategy, Target: item.DefaultProvider, Status: "Active", Revision: item.Revision, Application: application})
		return
	}
	server.state.mu.Lock()
	server.state.routing = append(server.state.routing, input)
	server.appendAuditLocked("admin", "Created routing rule "+input.Policy, "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, input)
}

func (server *Server) handleUpdateRouting(w http.ResponseWriter, r *http.Request) {
	policy := strings.TrimSpace(r.PathValue("policy"))
	var input routingDTO
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Policy) == "" || strings.TrimSpace(input.Selector) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "policy and selector are required"})
		return
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "Draft"
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		current, err := server.gatewayAdmin.GetRouting(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		item, err := server.gatewayAdmin.PutRouting(r.Context(), GatewayRoutingUpdateRequest{Strategy: strings.TrimSpace(input.Selector), DefaultProvider: strings.TrimSpace(input.Target), FallbackEnabled: current.FallbackEnabled, MaxAttempts: positiveOrDefault(current.MaxAttempts, 2), Composite: current.Composite, Revision: input.Revision})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		observed := current
		observed.Revision = input.Revision
		application, verifyErr := server.verifyRoutingApplication(r.Context(), observed, item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": verifyErr.Error(), "application": application})
			return
		}
		writeJSON(w, http.StatusOK, routingDTO{Policy: item.ID, Selector: item.Strategy, Target: item.DefaultProvider, Status: "Active", Revision: item.Revision, Application: application})
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index := range server.state.routing {
		if server.state.routing[index].Policy == policy {
			server.state.routing[index] = input
			server.appendAuditLocked("admin", "Updated routing rule "+input.Policy, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, input)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "routing rule not found"})
}

func (server *Server) verifyRoutingApplication(ctx context.Context, previous, written GatewayRouting) (*applicationDTO, error) {
	application := &applicationDTO{State: "applied", Applied: true, Revision: written.Revision, ProviderID: written.DefaultProvider, Route: written.Strategy}
	readback, err := server.gatewayAdmin.GetRouting(ctx)
	if err != nil || readback.Revision != written.Revision || readback.Revision <= previous.Revision || readback.DefaultProvider != written.DefaultProvider || readback.Strategy != written.Strategy {
		application.State = "failed"
		application.Applied = false
		application.Message = "routing readback did not confirm the persisted revision"
		return application, errors.New(application.Message)
	}
	if written.Application != nil && !written.Application.Applied {
		application.State = "warning"
		application.Applied = false
		application.Message = firstNonEmpty([]string{written.Application.Message}, "Gateway persisted routing but did not confirm runtime activation")
		return application, nil
	}
	providers, err := server.gatewayAdmin.ListProviders(ctx)
	model := ""
	if err == nil {
		for _, provider := range providers {
			if provider.ID == readback.DefaultProvider {
				model = provider.DefaultModel
				if model == "" && len(provider.Models) > 0 {
					model = provider.Models[0]
				}
				break
			}
		}
	}
	if model == "" {
		application.State = "warning"
		application.Message = "routing is active but its target model could not be resolved for verification"
		return application, nil
	}
	verification, err := server.gatewayAdmin.VerifyChat(ctx, model)
	application.RequestID = verification.RequestID
	if err != nil || !verification.Verified {
		application.State = "warning"
		application.Message = "routing is active but the live request verification did not complete"
		if verification.Message != "" {
			application.Message = verification.Message
		}
		return application, nil
	}
	application.ProviderID = verification.ProviderID
	application.Route = verification.Route
	if verification.ProviderID != readback.DefaultProvider || verification.Route != readback.Strategy {
		application.State = "warning"
		application.Message = "live request used a different provider or route than the saved configuration"
		return application, nil
	}
	if written.Application != nil && written.Application.State == "warning" {
		application.State = "warning"
		application.Message = written.Application.Message
		return application, nil
	}
	application.State = "verified"
	application.Verified = true
	return application, nil
}

func (server *Server) handleRuntimeVerification(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Resource string `json:"resource"`
		Target   string `json:"target"`
		Revision int64  `json:"revision"`
		Model    string `json:"model"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if server.config.DemoMode || server.gatewayAdminUnavailable(w) {
		return
	}
	switch strings.ToLower(strings.TrimSpace(input.Resource)) {
	case "provider":
		item, err := server.gatewayAdmin.GetProvider(r.Context(), input.Target)
		if err != nil || (input.Revision > 0 && item.Revision != input.Revision) {
			writeJSON(w, http.StatusBadGateway, map[string]any{"application": applicationDTO{State: "failed", Revision: input.Revision, Message: "provider readback revision mismatch"}})
			return
		}
		if strings.TrimSpace(input.Model) != "" {
			item.DefaultModel = strings.TrimSpace(input.Model)
		}
		application, verifyErr := server.verifyProviderApplication(r.Context(), item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"application": application})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"application": application})
	case "routing":
		item, err := server.gatewayAdmin.GetRouting(r.Context())
		if err != nil || (input.Revision > 0 && item.Revision != input.Revision) {
			writeJSON(w, http.StatusBadGateway, map[string]any{"application": applicationDTO{State: "failed", Revision: input.Revision, Message: "routing readback revision mismatch"}})
			return
		}
		previous := item
		previous.Revision--
		application, verifyErr := server.verifyRoutingApplication(r.Context(), previous, item)
		if verifyErr != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"application": application})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"application": application})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "resource must be provider or routing"})
	}
}

func (server *Server) handleDeleteRouting(w http.ResponseWriter, r *http.Request) {
	policy := strings.TrimSpace(r.PathValue("policy"))
	if !server.config.DemoMode {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "the Gateway global routing configuration cannot be deleted; update it instead"})
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, rule := range server.state.routing {
		if rule.Policy == policy {
			server.state.routing = append(server.state.routing[:index], server.state.routing[index+1:]...)
			server.appendAuditLocked("admin", "Deleted routing rule "+policy, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "routing rule not found"})
}

func positiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func (server *Server) handleAdminTenants(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		items, err := server.gatewayAdmin.ListTenants(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		tenants := make([]tenantDTO, 0, len(items))
		for _, item := range items {
			tenants = append(tenants, mapGatewayTenant(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}
	server.state.mu.Lock()
	tenants := append([]tenantDTO(nil), server.state.tenants...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"tenants":     tenants,
		"source":      "dashboard-state",
		"partialData": true,
		"warnings":    []string{"VeloxMesh Admin API is not connected"},
	})
}

func (server *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var input tenantDTO
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Tenant) == "" || strings.TrimSpace(input.Owner) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant and owner are required"})
		return
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "Healthy"
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		quota, err := parseDailyQuota(input.DailyQuota)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "dailyQuota must be a non-negative integer"})
			return
		}
		item, err := server.gatewayAdmin.CreateTenant(r.Context(), GatewayTenantCreateRequest{ID: strings.TrimSpace(input.Tenant), Name: strings.TrimSpace(input.Tenant), Owner: strings.TrimSpace(input.Owner), DailyQuota: quota, Status: gatewayTenantStatus(input.Status)})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, mapGatewayTenant(item))
		return
	}
	server.state.mu.Lock()
	server.state.tenants = append(server.state.tenants, input)
	server.appendAuditLocked("admin", "Created tenant "+input.Tenant, "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, input)
}

func (server *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	tenantName := strings.TrimSpace(r.PathValue("tenant"))
	var input tenantDTO
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Tenant) == "" || strings.TrimSpace(input.Owner) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant and owner are required"})
		return
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "Healthy"
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		quota, err := parseDailyQuota(input.DailyQuota)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "dailyQuota must be a non-negative integer"})
			return
		}
		item, err := server.gatewayAdmin.UpdateTenant(r.Context(), tenantName, GatewayTenantUpdateRequest{Name: strings.TrimSpace(input.Tenant), Owner: strings.TrimSpace(input.Owner), DailyQuota: quota, Status: gatewayTenantStatus(input.Status), Revision: input.Revision})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mapGatewayTenant(item))
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index := range server.state.tenants {
		if server.state.tenants[index].Tenant == tenantName {
			server.state.tenants[index] = input
			server.appendAuditLocked("admin", "Updated tenant "+input.Tenant, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, input)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
}

func (server *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantName := strings.TrimSpace(r.PathValue("tenant"))
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		if err := server.gatewayAdmin.DeleteTenant(r.Context(), tenantName); err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, tenant := range server.state.tenants {
		if tenant.Tenant == tenantName {
			server.state.tenants = append(server.state.tenants[:index], server.state.tenants[index+1:]...)
			server.appendAuditLocked("admin", "Deleted tenant "+tenantName, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
}

func mapGatewayTenant(item GatewayTenant) tenantDTO {
	status := "Healthy"
	if item.Status != "active" {
		status = "Inactive"
	}
	return tenantDTO{Tenant: item.ID, Owner: item.Owner, DailyQuota: strconv.FormatInt(item.DailyQuota, 10), Status: status, Revision: item.Revision}
}

func gatewayTenantStatus(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), "inactive") {
		return "inactive"
	}
	return "active"
}

func parseDailyQuota(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	quota, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || quota < 0 {
		return 0, errors.New("invalid daily quota")
	}
	return quota, nil
}

func (server *Server) handleAdminAPIKeys(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		items, err := server.gatewayAdmin.ListAPIKeys(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		keys := make([]map[string]string, 0, len(items))
		for _, item := range items {
			status := "Revoked"
			if item.Enabled {
				status = "Active"
			}
			keys = append(keys, map[string]string{"id": item.ID, "key": maskedGatewayKey(item.Prefix), "tenant": item.TenantID, "scope": item.Role, "status": status, "createdAt": formatGatewayTime(item.CreatedAt), "lastUsed": formatOptionalGatewayTime(item.LastUsedAt)})
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}
	server.state.mu.Lock()
	keys := make([]map[string]string, 0, len(server.state.apiKeys))
	for _, key := range server.state.apiKeys {
		keys = append(keys, map[string]string{
			"id":        key.ID,
			"key":       key.Key,
			"tenant":    key.Tenant,
			"scope":     key.Scope,
			"status":    key.Status,
			"createdAt": key.CreatedAt,
			"lastUsed":  key.LastUsed,
		})
	}
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"keys":        keys,
		"source":      "dashboard-state",
		"partialData": true,
		"warnings":    []string{"VeloxMesh Admin API is not connected"},
	})
}

func (server *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tenant string `json:"tenant"`
		Scope  string `json:"scope"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if strings.TrimSpace(input.Tenant) == "" || strings.TrimSpace(input.Scope) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant and scope are required"})
		return
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		created, err := server.gatewayAdmin.CreateAPIKey(r.Context(), GatewayAPIKeyCreateRequest{TenantID: strings.TrimSpace(input.Tenant), Name: "Dashboard key for " + strings.TrimSpace(input.Tenant), Role: strings.TrimSpace(input.Scope)})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": created.Record.ID, "key": created.Secret, "maskedKey": maskedGatewayKey(created.Record.Prefix), "tenant": created.Record.TenantID, "scope": created.Record.Role, "status": "Active", "createdAt": formatGatewayTime(created.Record.CreatedAt)})
		return
	}
	keyID, err := randomHex(8)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not issue api key"})
		return
	}
	keySecret, err := randomHex(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not issue api key"})
		return
	}
	secret := "vx_admin_" + keySecret
	key := apiKeyDTO{
		ID:        "key-" + keyID,
		Key:       maskAPIKeySecret(secret),
		KeyHash:   hashAPIKeySecret(secret),
		KeyPrefix: apiKeyPrefix(secret),
		Tenant:    strings.TrimSpace(input.Tenant),
		Scope:     strings.TrimSpace(input.Scope),
		Status:    "Active",
		CreatedAt: server.now().UTC().Format(time.RFC3339),
		LastUsed:  "never",
	}
	server.state.mu.Lock()
	candidate := server.persistedStateLocked()
	candidate.APIKeys = append(candidate.APIKeys, key)
	candidate.Audit = prependAudit(candidate.Audit, server.now, "admin", "Issued API key for "+key.Tenant, "Success")
	if err := server.writePersistedState(candidate); err != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist api key"})
		return
	}
	server.state.apiKeys = candidate.APIKeys
	server.state.audit = candidate.Audit
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{
		"id":        key.ID,
		"key":       secret,
		"maskedKey": key.Key,
		"tenant":    key.Tenant,
		"scope":     key.Scope,
		"status":    key.Status,
		"createdAt": key.CreatedAt,
	})
}

func (server *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	keyName := strings.TrimSpace(r.PathValue("key"))
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		if err := server.gatewayAdmin.RevokeAPIKey(r.Context(), keyName); err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, key := range server.state.apiKeys {
		if key.ID == keyName || key.Key == keyName {
			server.state.apiKeys = append(server.state.apiKeys[:index], server.state.apiKeys[index+1:]...)
			server.appendAuditLocked("admin", "Revoked API key "+keyName, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
}

func (server *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		items, err := server.gatewayAdmin.ListAudit(r.Context(), GatewayAuditFilter{TargetID: r.URL.Query().Get("target_id"), Actor: r.URL.Query().Get("actor"), Action: r.URL.Query().Get("action"), Outcome: r.URL.Query().Get("outcome")})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		events := make([]auditDTO, 0, len(items))
		for _, item := range items {
			events = append(events, auditDTO{Time: formatGatewayTime(item.Timestamp), Actor: item.Actor, Action: item.Action, Result: item.Outcome})
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": events, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}
	server.state.mu.Lock()
	events := append([]auditDTO(nil), server.state.audit...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"events":      events,
		"source":      "dashboard-state",
		"partialData": true,
		"warnings":    []string{"VeloxMesh Admin API is not connected"},
	})
}

func (server *Server) handleAuditCSV(w http.ResponseWriter, r *http.Request) {
	var events []auditDTO
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		items, err := server.gatewayAdmin.ListAudit(r.Context(), GatewayAuditFilter{TargetID: r.URL.Query().Get("target_id"), Actor: r.URL.Query().Get("actor"), Action: r.URL.Query().Get("action"), Outcome: r.URL.Query().Get("outcome")})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		events = make([]auditDTO, 0, len(items))
		for _, item := range items {
			events = append(events, auditDTO{Time: formatGatewayTime(item.Timestamp), Actor: item.Actor, Action: item.Action, Result: item.Outcome})
		}
	} else {
		server.state.mu.Lock()
		events = append([]auditDTO(nil), server.state.audit...)
		server.state.mu.Unlock()
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="veloxmesh-audit.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"time", "actor", "action", "result"})
	for _, event := range events {
		_ = writer.Write([]string{event.Time, event.Actor, event.Action, event.Result})
	}
	writer.Flush()
}

func configuredLabel(configured bool) string {
	if configured {
		return "Configured"
	}
	return "Not configured"
}

func (server *Server) settingsResponse(settings settingsDTO) map[string]any {
	return map[string]any{
		"settings": settings,
		"integrations": map[string]string{
			"gateway": "Not connected",
			"redis":   configuredLabel(strings.TrimSpace(server.config.RedisAddr) != ""),
			"qdrant":  configuredLabel(strings.TrimSpace(server.config.QdrantURL) != ""),
			"smtp":    configuredLabel(server.smtpConfigured()),
		},
		"source":      "dashboard-state",
		"partialData": true,
		"warnings":    []string{"Settings are local to the Dashboard BFF"},
	}
}

func (server *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		item, err := server.gatewayAdmin.GetSettings(r.Context())
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		settings := settingsDTO{DefaultProvider: item.DefaultProvider, DefaultModel: item.DefaultModel, RequestTimeoutSeconds: item.RequestTimeoutSeconds, DataRetentionDays: item.DataRetentionDays, Revision: item.Revision}
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings, "integrations": map[string]string{"gateway": "Connected", "redis": configuredLabel(strings.TrimSpace(server.config.RedisAddr) != ""), "qdrant": configuredLabel(strings.TrimSpace(server.config.QdrantURL) != ""), "smtp": configuredLabel(server.smtpConfigured())}, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}
	server.state.mu.Lock()
	settings := server.state.settings
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, server.settingsResponse(settings))
}

func maskedGatewayKey(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "********"
	}
	return prefix + "********"
}

func formatGatewayTime(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalGatewayTime(value *time.Time) string {
	if value == nil {
		return "never"
	}
	return formatGatewayTime(*value)
}

func (server *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var input settingsDTO
	if !decodeJSON(w, r, &input) {
		return
	}
	input.DefaultProvider = strings.TrimSpace(input.DefaultProvider)
	input.DefaultModel = strings.TrimSpace(input.DefaultModel)
	if input.DefaultProvider == "" || input.DefaultModel == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "default provider and model are required"})
		return
	}
	if input.RequestTimeoutSeconds < 1 || input.RequestTimeoutSeconds > 600 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "request timeout must be between 1 and 600 seconds"})
		return
	}
	if input.DataRetentionDays < 1 || input.DataRetentionDays > 3650 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "data retention must be between 1 and 3650 days"})
		return
	}
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		item, err := server.gatewayAdmin.PutSettings(r.Context(), GatewaySettingsUpdateRequest{DefaultProvider: input.DefaultProvider, DefaultModel: input.DefaultModel, RequestTimeoutSeconds: input.RequestTimeoutSeconds, DataRetentionDays: input.DataRetentionDays, Revision: input.Revision})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		settings := settingsDTO{DefaultProvider: item.DefaultProvider, DefaultModel: item.DefaultModel, RequestTimeoutSeconds: item.RequestTimeoutSeconds, DataRetentionDays: item.DataRetentionDays, Revision: item.Revision}
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings, "integrations": map[string]string{"gateway": "Connected"}, "source": "veloxmesh-admin", "partialData": false, "warnings": []string{}})
		return
	}

	server.state.mu.Lock()
	candidate := server.persistedStateLocked()
	candidate.Settings = input
	candidate.Audit = prependAudit(candidate.Audit, server.now, "admin", "Updated Dashboard settings", "Success")
	if err := server.writePersistedState(candidate); err != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist settings"})
		return
	}
	server.state.settings = candidate.Settings
	server.state.audit = candidate.Audit
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, server.settingsResponse(input))
}

func (server *Server) handleAdminRequests(w http.ResponseWriter, r *http.Request) {
	if !server.config.DemoMode {
		if server.gatewayAdminUnavailable(w) {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		startValue, endValue := parseQueryTime(r.URL.Query().Get("start")), parseQueryTime(r.URL.Query().Get("end"))
		var start, end *time.Time
		if !startValue.IsZero() {
			start = &startValue
		}
		if !endValue.IsZero() {
			end = &endValue
		}
		items, err := server.gatewayAdmin.ListUsage(r.Context(), GatewayUsageFilter{TenantID: r.URL.Query().Get("tenant_id"), APIKeyID: r.URL.Query().Get("api_key_id"), ProviderID: r.URL.Query().Get("provider_id"), Model: r.URL.Query().Get("model"), Status: r.URL.Query().Get("status"), Start: start, End: end, Limit: limit})
		if err != nil {
			writeGatewayAdminError(w, err)
			return
		}
		requests := make([]map[string]any, 0, len(items))
		partialData := false
		for _, item := range items {
			if item.TenantID == "" {
				partialData = true
			}
			status := item.Status
			if status == "settled" {
				status = "success"
			}
			requests = append(requests, map[string]any{"id": item.ID, "tenant": item.TenantID, "provider": item.ProviderID, "model": item.Model, "status": status, "latencyMs": item.DurationMs, "route": "gateway-usage"})
		}
		warnings := []string{}
		if partialData {
			warnings = []string{"Some legacy usage rows do not contain tenant_id"}
		}
		writeJSON(w, http.StatusOK, map[string]any{"requests": requests, "source": "veloxmesh-admin", "partialData": partialData, "warnings": warnings})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requests": []map[string]any{
			{
				"id":        "req_10291",
				"tenant":    "coursework-lab",
				"provider":  server.config.ProviderName,
				"model":     server.config.DefaultModel,
				"status":    "success",
				"latencyMs": 714,
				"route":     "latency-aware",
			},
			{
				"id":        "req_10290",
				"tenant":    "capstone-demo",
				"provider":  server.config.ProviderName,
				"model":     firstNonEmpty(server.config.Models, server.config.DefaultModel),
				"status":    "success",
				"latencyMs": 931,
				"route":     "quality-fallback",
			},
			{
				"id":        "req_10289",
				"tenant":    "ops-sandbox",
				"provider":  server.config.ProviderName,
				"model":     server.config.DefaultModel,
				"status":    "rate_limited",
				"latencyMs": 0,
				"route":     "tenant-quota",
			},
		},
	})
}

func (server *Server) handleAdminRequestLogs(w http.ResponseWriter, r *http.Request) {
	snapshot := server.operationalStore.Snapshot(r.Context())
	benchmarkSnapshot := server.benchmarkRequests.Snapshot(r.Context())
	operationalLogs := snapshot.RequestLogs
	if len(operationalLogs) == 0 && len(benchmarkSnapshot.Requests) == 0 && server.config.DemoMode {
		operationalLogs = server.requestLogs()
		snapshot.Source = "demo"
	}
	logs := mergeRequestLogs(operationalLogs, benchmarkSnapshot.Requests)
	if logs == nil {
		logs = []requestLogDTO{}
	}
	totalRows := len(logs)
	truncated := totalRows > adminRequestLogLimit
	if truncated {
		logs = logs[:adminRequestLogLimit]
	}
	warnings := requestLogWarnings(snapshot, benchmarkSnapshot, truncated)
	source := requestLogSource(snapshot, benchmarkSnapshot)
	generatedAt := latestTimestamp(snapshot.GeneratedAt, benchmarkSnapshot.GeneratedAt)
	if generatedAt == "" {
		generatedAt = snapshot.GeneratedAt
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":         logs,
		"source":       source,
		"generatedAt":  generatedAt,
		"storage":      map[string]storageStatusDTO{"redis": snapshot.Redis, "benchmarkRedis": benchmarkSnapshot.Redis},
		"totalRows":    totalRows,
		"returnedRows": len(logs),
		"truncated":    truncated,
		"partialData":  len(warnings) > 0,
		"warnings":     warnings,
	})
}

func mergeRequestLogs(operational []requestLogDTO, benchmark []benchmarkRequestDTO) []requestLogDTO {
	byID := make(map[string]requestLogDTO, len(operational)+len(benchmark))
	for _, row := range operational {
		byID[row.RequestID] = row
	}
	for _, row := range benchmark {
		mapped := benchmarkRequestLog(row)
		byID[mapped.RequestID] = mapped
	}
	result := make([]requestLogDTO, 0, len(byID))
	for _, row := range byID {
		result = append(result, row)
	}
	sort.SliceStable(result, func(left, right int) bool {
		return parseQueryTime(result[left].Timestamp).After(parseQueryTime(result[right].Timestamp))
	})
	return result
}

func benchmarkRequestLog(row benchmarkRequestDTO) requestLogDTO {
	status := "Error"
	switch strings.ToLower(strings.TrimSpace(row.Status)) {
	case "success", "passed", "settled":
		status = "Success"
	case "timeout":
		status = "Timeout"
	}
	ttft := 0.0
	if row.TTFTMs != nil {
		ttft = *row.TTFTMs
	}
	errorMessage := ""
	if status != "Success" {
		errorMessage = strings.TrimSpace(row.ErrorType)
		if row.HTTPStatus > 0 {
			if errorMessage == "" {
				errorMessage = "request failed"
			}
			errorMessage += " (HTTP " + strconv.Itoa(row.HTTPStatus) + ")"
		}
	}
	tenant := "benchmark"
	if strings.TrimSpace(row.Dataset) != "" {
		tenant += "/" + strings.TrimSpace(row.Dataset)
	}
	return requestLogDTO{
		RequestID: row.RequestID, Tenant: tenant, Provider: row.Provider, Model: row.Model, Method: row.Method,
		InputTokens: row.InputTokens, OutputTokens: row.OutputTokens, Status: status, LatencyMs: row.LatencyMs,
		TTFTMs: ttft, ErrorMessage: errorMessage, Timestamp: firstNonEmpty([]string{row.StartedAt, row.EndedAt}, ""),
	}
}

func requestLogSource(operational operationalSnapshot, benchmark benchmarkRequestSnapshot) string {
	hasOperational := len(operational.RequestLogs) > 0
	hasBenchmark := len(benchmark.Requests) > 0
	if hasOperational && hasBenchmark {
		return "operational+benchmark"
	}
	if hasBenchmark {
		return "benchmark"
	}
	if source := strings.TrimSpace(operational.Source); source != "" {
		return source
	}
	return "empty"
}

func requestLogWarnings(operational operationalSnapshot, benchmark benchmarkRequestSnapshot, truncated bool) []string {
	warnings := make([]string, 0, 3)
	if status := strings.ToLower(strings.TrimSpace(operational.Redis.Status)); status == "unreachable" || status == "error" {
		warnings = append(warnings, "Operational request log source is unavailable: "+operational.Redis.Detail)
	}
	if status := strings.ToLower(strings.TrimSpace(benchmark.Redis.Status)); status == "unreachable" || status == "error" {
		warnings = append(warnings, "Benchmark request log source is unavailable: "+benchmark.Redis.Detail)
	}
	if truncated {
		warnings = append(warnings, "Request logs are limited to the newest "+strconv.Itoa(adminRequestLogLimit)+" rows")
	}
	return warnings
}

func latestTimestamp(values ...string) string {
	latest := ""
	latestTime := time.Time{}
	for _, value := range values {
		parsed := parseQueryTime(value)
		if parsed.After(latestTime) {
			latest = value
			latestTime = parsed
		}
	}
	return latest
}

func (server *Server) handleAdminProviderHealth(w http.ResponseWriter, r *http.Request) {
	snapshot := server.operationalStore.Snapshot(r.Context())
	providers := snapshot.ProviderHealth
	if len(providers) == 0 && server.config.DemoMode {
		providers = server.providerHealth()
		snapshot.Source = "demo"
	}
	if providers == nil {
		providers = []providerHealthDTO{}
	}
	source := strings.TrimSpace(snapshot.Source)
	if source == "" {
		source = "empty"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":   providers,
		"source":      source,
		"generatedAt": snapshot.GeneratedAt,
		"storage":     map[string]storageStatusDTO{"redis": snapshot.Redis},
	})
}

func (server *Server) handleAdminBenchmarks(w http.ResponseWriter, r *http.Request) {
	snapshot := server.benchmarkStore.Snapshot(r.Context())
	if len(snapshot.Benchmarks) == 0 && server.config.DemoMode {
		snapshot.Benchmarks = fallbackBenchmarks()
		snapshot.Source = "demo"
	}
	if strings.TrimSpace(snapshot.Source) == "" {
		snapshot.Source = "empty"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"benchmarks":  snapshot.Benchmarks,
		"source":      snapshot.Source,
		"generatedAt": snapshot.GeneratedAt,
		"storage": map[string]storageStatusDTO{
			"redis":  snapshot.Redis,
			"qdrant": snapshot.Qdrant,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	return true
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func firstNonEmpty(values []string, fallback string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}

func newStateStore(config Config, now func() time.Time) *stateStore {
	seedDevKey := "vx_admin_seed_development"
	seedCourseworkKey := "vx_admin_seed_coursework"
	seedOperationsKey := "vx_admin_seed_operations"
	providers := []providerDTO{
		{
			Name:          config.ProviderName,
			BaseURL:       config.BaseURL,
			DefaultModel:  config.DefaultModel,
			Models:        config.Models,
			Status:        "healthy",
			P95LatencyMs:  842,
			SuccessRate:   99.2,
			RequestsToday: 18420,
		},
	}
	return &stateStore{
		providers: providers,
		routing: []routingDTO{
			{Policy: "Primary route", Selector: "latency-aware", Target: config.ProviderName, Status: "Active"},
			{Policy: "Fallback", Selector: "quality-fallback", Target: config.ProviderName, Status: "Active"},
			{Policy: "Admission control", Selector: "tenant-quota", Target: "shared queue", Status: "Enforced"},
		},
		tenants: []tenantDTO{
			{Tenant: "coursework-lab", Owner: "Evaluation", DailyQuota: "8,000", Status: "Healthy"},
			{Tenant: "capstone-demo", Owner: "Demo", DailyQuota: "5,000", Status: "Healthy"},
			{Tenant: "ops-sandbox", Owner: "Operations", DailyQuota: "1,000", Status: "Rate Limited"},
		},
		apiKeys: []apiKeyDTO{
			{ID: "key-seed-dev", Key: maskAPIKeySecret(seedDevKey), KeyHash: hashAPIKeySecret(seedDevKey), KeyPrefix: apiKeyPrefix(seedDevKey), Tenant: "capstone-demo", Scope: "admin:read", Status: "Active", CreatedAt: now().UTC().Format(time.RFC3339), LastUsed: "just now"},
			{ID: "key-seed-coursework", Key: maskAPIKeySecret(seedCourseworkKey), KeyHash: hashAPIKeySecret(seedCourseworkKey), KeyPrefix: apiKeyPrefix(seedCourseworkKey), Tenant: "coursework-lab", Scope: "gateway:invoke", Status: "Active", CreatedAt: now().UTC().Format(time.RFC3339), LastUsed: "12 min ago"},
			{ID: "key-seed-operations", Key: maskAPIKeySecret(seedOperationsKey), KeyHash: hashAPIKeySecret(seedOperationsKey), KeyPrefix: apiKeyPrefix(seedOperationsKey), Tenant: "ops-sandbox", Scope: "admin:write", Status: "Active", CreatedAt: now().UTC().Format(time.RFC3339), LastUsed: "1 hour ago"},
		},
		audit: []auditDTO{
			{Time: now().Format("15:04"), Actor: "admin", Action: "Refreshed provider health", Result: "Success"},
			{Time: now().Format("15:04"), Actor: "gateway", Action: "Applied tenant quota", Result: "Rate Limited"},
			{Time: now().Format("15:04"), Actor: "admin", Action: "Viewed routing policy", Result: "Success"},
		},
		settings: settingsDTO{
			DefaultProvider:       config.ProviderName,
			DefaultModel:          config.DefaultModel,
			RequestTimeoutSeconds: 30,
			DataRetentionDays:     30,
		},
		sessions:   map[string]sessionDTO{},
		challenges: map[string]loginChallengeDTO{},
	}
}

func (server *Server) appendAuditLocked(actor string, action string, result string) {
	server.state.audit = prependAudit(server.state.audit, server.now, actor, action, result)
}

func prependAudit(events []auditDTO, now func() time.Time, actor string, action string, result string) []auditDTO {
	return append([]auditDTO{{
		Time:   now().Format("15:04"),
		Actor:  actor,
		Action: action,
		Result: result,
	}}, events...)
}

func (server *Server) findUserLocked(identifier string) *userDTO {
	normalized := strings.ToLower(strings.TrimSpace(identifier))
	for index := range server.state.users {
		if strings.ToLower(server.state.users[index].Username) == normalized || strings.ToLower(server.state.users[index].Email) == normalized {
			return &server.state.users[index]
		}
	}
	return nil
}

func (server *Server) createLoginChallengeLocked(user userDTO) (string, string, error) {
	challengeID, err := randomHex(16)
	if err != nil {
		return "", "", err
	}
	code, err := randomDigits(6)
	if err != nil {
		return "", "", err
	}
	if server.state.challenges == nil {
		server.state.challenges = map[string]loginChallengeDTO{}
	}
	server.state.challenges[challengeID] = newLoginChallenge(challengeID, user.Username, code, server.verificationPepper, server.now())
	return challengeID, code, nil
}

func (server *Server) sendVerificationEmail(user userDTO, code string) error {
	subject := "VeloxMesh login verification code"
	body := "Your VeloxMesh login verification code is " + code + ". It expires in 5 minutes."
	if server.mailSender != nil {
		return server.mailSender.Send(user.Email, subject, body)
	}
	if !server.developmentVerificationEnabled() {
		return errors.New("verification delivery unavailable")
	}
	return server.writeEmailOutbox(user.Email, subject, body)
}

func (server *Server) developmentVerificationEnabled() bool {
	return server.config.DemoMode || server.config.TestMode
}

func (server *Server) verificationDeliveryAvailable() bool {
	return server.smtpConfigured() || server.developmentVerificationEnabled()
}

func (server *Server) smtpConfigured() bool {
	return server.mailSender != nil
}

func smtpConfigurationComplete(config Config) bool {
	return strings.TrimSpace(config.SMTPHost) != "" &&
		strings.TrimSpace(config.SMTPUsername) != "" &&
		strings.TrimSpace(config.SMTPPassword) != "" &&
		strings.TrimSpace(config.SMTPFrom) != ""
}

func (server *Server) writeEmailOutbox(to string, subject string, body string) error {
	path := strings.TrimSpace(server.config.EmailOutboxPath)
	if path == "" {
		path = "tmp/email-outbox.log"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := fmt.Sprintf("time=%s\nto=%s\nsubject=%s\n%s\n\n", server.now().Format(time.RFC3339), to, subject, body)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(entry)
	return err
}

func (server *Server) createSessionLocked(w http.ResponseWriter, user userDTO) {
	token, err := randomHex(32)
	if err != nil {
		return
	}
	if server.state.sessions == nil {
		server.state.sessions = map[string]sessionDTO{}
	}
	server.state.sessions[token] = sessionDTO{
		UserID:    user.ID,
		Username:  user.Username,
		TenantID:  user.TenantID,
		Role:      user.Role,
		ExpiresAt: server.now().Add(server.config.SessionTTL),
	}
	maxAge := int(server.config.SessionTTL / time.Second)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   server.secureSessionCookie(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  server.now().Add(server.config.SessionTTL),
	})
}

func (server *Server) userFromRequest(r *http.Request) (userDTO, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return userDTO{}, false
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	session, ok := server.state.sessions[cookie.Value]
	if !ok {
		return userDTO{}, false
	}
	if !session.ExpiresAt.IsZero() && server.now().After(session.ExpiresAt) {
		delete(server.state.sessions, cookie.Value)
		return userDTO{}, false
	}
	user := server.findUserLocked(session.Username)
	if user == nil {
		return userDTO{}, false
	}
	if session.UserID != "" && user.ID != session.UserID {
		return userDTO{}, false
	}
	if user.Role != session.Role || user.TenantID != session.TenantID {
		return userDTO{}, false
	}
	return *user, true
}

func sessionResponse(user userDTO) map[string]any {
	return map[string]any{
		"user":     user.Username,
		"userId":   user.ID,
		"tenantId": user.TenantID,
		"role":     user.Role,
		"scopes":   append([]string(nil), user.Scopes...),
	}
}

func stableUserID(username string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(username))))
	return "user-" + hex.EncodeToString(sum[:8])
}

func authRole(input string) (string, []string, bool) {
	role := strings.ToLower(strings.TrimSpace(input))
	if role == "" || role == "admin" {
		return "Admin", []string{"admin:write", "gateway:read", "audit:export"}, true
	}
	if role == "customer" {
		return "Customer", []string{"gateway:invoke"}, true
	}
	return "", nil, false
}

func passwordMatches(user userDTO, password string) bool {
	if strings.HasPrefix(user.PasswordHash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil
	}
	expected := hashPassword(password, user.PasswordSalt)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(user.PasswordHash)) == 1
}

func hashPasswordAdaptive(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func hashPassword(password string, salt string) string {
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return hex.EncodeToString(sum[:])
}

func randomHex(byteCount int) (string, error) {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func mustRandomBytes(byteCount int) []byte {
	data := make([]byte, byteCount)
	if _, err := rand.Read(data); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return data
}

func randomDigits(length int) (string, error) {
	var builder strings.Builder
	for builder.Len() < length {
		value := []byte{0}
		if _, err := rand.Read(value); err != nil {
			return "", err
		}
		if value[0] >= 250 {
			continue
		}
		builder.WriteString(strconv.Itoa(int(value[0] % 10)))
	}
	return builder.String(), nil
}

func isDigitsOnly(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (server *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   server.secureSessionCookie(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
	})
}

func (server *Server) secureSessionCookie() bool {
	return server.config.SessionCookieSecure || (!server.config.DemoMode && !server.config.TestMode)
}

func (server *Server) requestLogs() []requestLogDTO {
	return []requestLogDTO{
		{
			RequestID:    "req_10291",
			Tenant:       "coursework-lab",
			Provider:     server.config.ProviderName,
			Model:        server.config.DefaultModel,
			Method:       "Our Gateway Method",
			InputTokens:  812,
			OutputTokens: 248,
			Status:       "Success",
			LatencyMs:    714,
			TTFTMs:       210,
			Timestamp:    server.now().UTC().Format(time.RFC3339),
		},
		{
			RequestID:    "req_10290",
			Tenant:       "capstone-demo",
			Provider:     server.config.ProviderName,
			Model:        firstNonEmpty(server.config.Models, server.config.DefaultModel),
			Method:       "Our Gateway Method",
			InputTokens:  1890,
			OutputTokens: 420,
			Status:       "Success",
			LatencyMs:    931,
			TTFTMs:       260,
			Timestamp:    server.now().UTC().Format(time.RFC3339),
		},
		{
			RequestID:    "req_10289",
			Tenant:       "ops-sandbox",
			Provider:     server.config.ProviderName,
			Model:        server.config.DefaultModel,
			Method:       "Our Gateway Method",
			InputTokens:  0,
			OutputTokens: 0,
			Status:       "Error",
			LatencyMs:    0,
			ErrorMessage: "tenant quota exceeded",
			Timestamp:    server.now().UTC().Format(time.RFC3339),
		},
	}
}

func (server *Server) providerHealth() []providerHealthDTO {
	return []providerHealthDTO{{
		Provider:     server.config.ProviderName,
		TargetModel:  server.config.DefaultModel,
		Status:       "Healthy",
		AvgLatencyMs: 714,
		ErrorRate:    0,
		TimeoutRate:  0,
		LastChecked:  server.now().UTC().Format(time.RFC3339),
	}}
}

func (store liveOperationalStore) Snapshot(ctx context.Context) operationalSnapshot {
	var providerDocument struct {
		GeneratedAt string              `json:"generatedAt"`
		Providers   []providerHealthDTO `json:"providers"`
	}
	var logDocument struct {
		GeneratedAt string          `json:"generatedAt"`
		Logs        []requestLogDTO `json:"logs"`
	}
	providerErr := redisJSONDocument(ctx, store.redisAddr, "veloxmesh:provider_health", &providerDocument)
	logErr := redisJSONDocument(ctx, store.redisAddr, "veloxmesh:request_logs", &logDocument)
	status := storageStatusDTO{Status: "connected", Detail: "operational keys loaded"}
	if providerErr != nil && logErr != nil {
		status = storageStatusDTO{Status: "connected", Detail: "no operational snapshot keys"}
		if isRedisConnectionError(providerErr) || isRedisConnectionError(logErr) {
			status = storageStatusDTO{Status: "unreachable", Detail: shortError(providerErr)}
		}
	}
	generatedAt := providerDocument.GeneratedAt
	if logDocument.GeneratedAt > generatedAt {
		generatedAt = logDocument.GeneratedAt
	}
	source := "empty"
	if len(providerDocument.Providers) > 0 || len(logDocument.Logs) > 0 {
		source = "redis"
	}
	return operationalSnapshot{
		ProviderHealth: providerDocument.Providers,
		RequestLogs:    logDocument.Logs,
		Source:         source,
		GeneratedAt:    generatedAt,
		Redis:          status,
	}
}

func redisJSONDocument(ctx context.Context, redisAddr string, key string, target any) error {
	if strings.TrimSpace(redisAddr) == "" {
		return fmt.Errorf("redis connection: REDIS_ADDR is empty")
	}
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", redisAddr)
	if err != nil {
		return fmt.Errorf("redis connection: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(conn)
	if _, err := fmt.Fprintf(conn, "*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key); err != nil {
		return fmt.Errorf("redis connection: %w", err)
	}
	value, err := readRedisBulkString(reader)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(value), target)
}

func isRedisConnectionError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "redis connection:")
}

func (server *Server) loadState() {
	if strings.TrimSpace(server.config.StatePath) == "" {
		return
	}
	data, err := os.ReadFile(server.config.StatePath)
	if err != nil {
		return
	}
	var saved persistedState
	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}
	var migratedAPIKeys bool
	saved.APIKeys, migratedAPIKeys = migrateLegacyAPIKeys(saved.APIKeys, server.now)
	server.state.providers = saved.Providers
	server.state.routing = saved.Routing
	server.state.tenants = saved.Tenants
	server.state.apiKeys = saved.APIKeys
	server.state.audit = saved.Audit
	server.state.users = saved.Users
	if saved.Settings.DefaultProvider != "" && saved.Settings.DefaultModel != "" {
		server.state.settings = saved.Settings
	}
	for index := range server.state.users {
		if server.state.users[index].ID == "" {
			server.state.users[index].ID = stableUserID(server.state.users[index].Username)
		}
	}
	if server.state.sessions == nil {
		server.state.sessions = map[string]sessionDTO{}
	}
	if server.state.challenges == nil {
		server.state.challenges = map[string]loginChallengeDTO{}
	}
	if migratedAPIKeys {
		_ = server.writePersistedState(server.persistedStateLocked())
	}
}

func (server *Server) ensureBootstrapAdmin() {
	email := strings.TrimSpace(server.config.BootstrapAdminEmail)
	username := strings.TrimSpace(server.config.BootstrapAdminUsername)
	password := server.config.BootstrapAdminPassword
	if email == "" || username == "" || password == "" {
		return
	}
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for _, user := range server.state.users {
		if user.Role == "Admin" || strings.EqualFold(user.Email, email) || strings.EqualFold(user.Username, username) {
			return
		}
	}
	passwordHash, err := hashPasswordAdaptive(password)
	if err != nil {
		return
	}
	server.state.users = append(server.state.users, userDTO{
		ID:           stableUserID(username),
		Email:        email,
		Username:     username,
		Role:         "Admin",
		Verified:     true,
		Scopes:       []string{"admin:write", "gateway:read", "audit:export"},
		PasswordHash: passwordHash,
	})
	server.appendAuditLocked(username, "Bootstrapped Admin account", "Success")
	server.saveStateLocked()
}

func (server *Server) persistedStateLocked() persistedState {
	return persistedState{
		Providers: append([]providerDTO(nil), server.state.providers...),
		Routing:   append([]routingDTO(nil), server.state.routing...),
		Tenants:   append([]tenantDTO(nil), server.state.tenants...),
		APIKeys:   append([]apiKeyDTO(nil), server.state.apiKeys...),
		Audit:     append([]auditDTO(nil), server.state.audit...),
		Settings:  server.state.settings,
		Users:     append([]userDTO(nil), server.state.users...),
	}

}

func (server *Server) writePersistedState(saved persistedState) error {
	if strings.TrimSpace(server.config.StatePath) == "" {
		return nil
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(server.config.StatePath), 0o755); err != nil {
		return err
	}
	temporary := server.config.StatePath + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, server.config.StatePath)
}

func (server *Server) saveStateLocked() {
	_ = server.writePersistedState(server.persistedStateLocked())
}

func newLiveBenchmarkStore(config Config) benchmarkStore {
	return liveBenchmarkStore{
		redisAddr:        config.RedisAddr,
		qdrantURL:        strings.TrimRight(config.QdrantURL, "/"),
		qdrantAPIKey:     config.QdrantAPIKey,
		qdrantCollection: config.QdrantBenchmarkCollection,
		httpClient:       &http.Client{Timeout: 2 * time.Second},
	}
}

func (store liveBenchmarkStore) Snapshot(ctx context.Context) benchmarkSnapshot {
	redisStatus, benchmarks, generatedAt := store.redisBenchmarks(ctx)
	qdrantStatus := store.qdrantStatus(ctx)
	source := "empty"
	if len(benchmarks) > 0 {
		source = "redis"
	}
	return benchmarkSnapshot{
		Benchmarks:  benchmarks,
		Source:      source,
		GeneratedAt: generatedAt,
		Redis:       redisStatus,
		Qdrant:      qdrantStatus,
	}
}

func (store liveBenchmarkStore) redisBenchmarks(ctx context.Context) (storageStatusDTO, []benchmarkDTO, string) {
	if strings.TrimSpace(store.redisAddr) == "" {
		return storageStatusDTO{Status: "not configured", Detail: "REDIS_ADDR is empty"}, nil, ""
	}
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", store.redisAddr)
	if err != nil {
		return storageStatusDTO{Status: "unreachable", Detail: shortError(err)}, nil, ""
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	reader := bufio.NewReader(conn)
	if _, err := fmt.Fprintf(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		return storageStatusDTO{Status: "unreachable", Detail: shortError(err)}, nil, ""
	}
	if line, err := reader.ReadString('\n'); err != nil || !strings.HasPrefix(line, "+PONG") {
		return storageStatusDTO{Status: "unreachable", Detail: "PING did not return PONG"}, nil, ""
	}

	benchmarks, generatedAt, err := store.redisJSONValue(conn, reader, "veloxmesh:benchmarks")
	if err != nil {
		return storageStatusDTO{Status: "connected", Detail: "PING ok; no benchmark snapshot key"}, nil, ""
	}
	return storageStatusDTO{Status: "connected", Detail: "loaded veloxmesh:benchmarks"}, benchmarks, generatedAt
}

func (store liveBenchmarkStore) redisJSONValue(conn net.Conn, reader *bufio.Reader, key string) ([]benchmarkDTO, string, error) {
	if _, err := fmt.Fprintf(conn, "*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(key), key); err != nil {
		return nil, "", err
	}
	value, err := readRedisBulkString(reader)
	if err != nil {
		return nil, "", err
	}
	var response struct {
		GeneratedAt string         `json:"generatedAt"`
		Benchmarks  []benchmarkDTO `json:"benchmarks"`
	}
	if err := json.Unmarshal([]byte(value), &response); err != nil {
		return nil, "", err
	}
	if len(response.Benchmarks) == 0 {
		return nil, "", fmt.Errorf("empty benchmark snapshot")
	}
	return response.Benchmarks, response.GeneratedAt, nil
}

func (store liveBenchmarkStore) qdrantStatus(ctx context.Context) storageStatusDTO {
	if strings.TrimSpace(store.qdrantURL) == "" {
		return storageStatusDTO{Status: "not configured", Detail: "QDRANT_URL is empty"}
	}
	client := store.httpClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, store.qdrantURL+"/healthz", nil)
	if err != nil {
		return storageStatusDTO{Status: "unreachable", Detail: shortError(err)}
	}
	if strings.TrimSpace(store.qdrantAPIKey) != "" {
		req.Header.Set("api-key", store.qdrantAPIKey)
	}
	response, err := client.Do(req)
	if err != nil {
		return storageStatusDTO{Status: "unreachable", Detail: shortError(err)}
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 256))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return storageStatusDTO{Status: "unreachable", Detail: response.Status}
	}
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		detail = "healthz ok"
	}
	if strings.TrimSpace(store.qdrantCollection) != "" {
		collectionDetail := store.qdrantCollectionDetail(ctx, client)
		if collectionDetail != "" {
			detail = collectionDetail
		}
	}
	return storageStatusDTO{Status: "connected", Detail: detail}
}

func (store liveBenchmarkStore) qdrantCollectionDetail(ctx context.Context, client *http.Client) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, store.qdrantURL+"/collections/"+store.qdrantCollection, nil)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(store.qdrantAPIKey) != "" {
		req.Header.Set("api-key", store.qdrantAPIKey)
	}
	response, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "healthz ok; collection " + store.qdrantCollection + " missing"
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ""
	}
	var body struct {
		Result struct {
			PointsCount int `json:"points_count"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body); err != nil {
		return "collection " + store.qdrantCollection + " ready"
	}
	unit := "points"
	if body.Result.PointsCount == 1 {
		unit = "point"
	}
	return fmt.Sprintf("collection %s ready (%d %s)", store.qdrantCollection, body.Result.PointsCount, unit)
}

func readRedisBulkString(reader *bufio.Reader) (string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	header = strings.TrimSpace(header)
	if header == "$-1" {
		return "", fmt.Errorf("missing key")
	}
	if !strings.HasPrefix(header, "$") {
		return "", fmt.Errorf("unexpected redis response: %s", header)
	}
	length := 0
	if _, err := fmt.Sscanf(header, "$%d", &length); err != nil {
		return "", err
	}
	data := make([]byte, length+2)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data[:length]), nil
}

func fallbackBenchmarks() []benchmarkDTO {
	return []benchmarkDTO{
		{
			RunID:                 "demo-local-baseline",
			Method:                "Local Baseline",
			Dataset:               "Demo workload",
			RequestCount:          100,
			Concurrency:           1,
			TimeoutSettingSeconds: 120,
			Provider:              "demo-provider",
			TargetModel:           "demo-model",
			GatewayVersion:        "demo",
			AvgLatencyMs:          benchmarkFloat(940),
			P50LatencyMs:          benchmarkFloat(800),
			P95LatencyMs:          benchmarkFloat(1200),
			P99LatencyMs:          benchmarkFloat(1400),
			TTFTMs:                benchmarkFloat(220),
			ThroughputRPS:         benchmarkFloat(1.7),
			SuccessRatePct:        99,
			ErrorRatePct:          1,
			TimeoutRatePct:        0,
			TestDate:              "demo",
			Source:                "demo",
			RawFilePath:           "demo",
			ExportID:              "demo-local-baseline",
			Status:                "passed",
		},
		{
			RunID:                 "demo-gateway",
			Method:                "Our Gateway Method",
			Dataset:               "Demo workload",
			RequestCount:          100,
			Concurrency:           1,
			TimeoutSettingSeconds: 120,
			Provider:              "demo-provider",
			TargetModel:           "demo-model",
			GatewayVersion:        "demo",
			AvgLatencyMs:          benchmarkFloat(760),
			P50LatencyMs:          benchmarkFloat(650),
			P95LatencyMs:          benchmarkFloat(980),
			P99LatencyMs:          benchmarkFloat(1100),
			TTFTMs:                benchmarkFloat(180),
			ThroughputRPS:         benchmarkFloat(2.1),
			SuccessRatePct:        99,
			ErrorRatePct:          1,
			TimeoutRatePct:        0,
			ImprovementPct:        benchmarkFloat(19.15),
			TestDate:              "demo",
			Source:                "demo",
			RawFilePath:           "demo",
			ExportID:              "demo-gateway",
			Status:                "passed",
		},
	}
}

func benchmarkFloat(value float64) *float64 {
	return &value
}

func shortError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 120 {
		return message[:120]
	}
	return message
}
