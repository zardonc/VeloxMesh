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
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
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

type Config struct {
	DevAPIKey                 string
	AllowAdminRegistration    bool
	BootstrapAdminEmail       string
	BootstrapAdminUsername    string
	BootstrapAdminPassword    string
	ProviderName              string
	BaseURL                   string
	DefaultModel              string
	Models                    []string
	StatePath                 string
	RedisAddr                 string
	QdrantURL                 string
	QdrantAPIKey              string
	QdrantBenchmarkCollection string
	BenchmarkStore            benchmarkStore
	OperationalStore          operationalStore
	EmailOutboxPath           string
	SMTPHost                  string
	SMTPPort                  string
	SMTPUsername              string
	SMTPPassword              string
	SMTPFrom                  string
	DemoMode                  bool
}

type Server struct {
	mux              *http.ServeMux
	config           Config
	now              func() time.Time
	state            *stateStore
	benchmarkStore   benchmarkStore
	operationalStore operationalStore
}

type stateStore struct {
	mu         sync.Mutex
	providers  []providerDTO
	routing    []routingDTO
	tenants    []tenantDTO
	apiKeys    []apiKeyDTO
	audit      []auditDTO
	users      []userDTO
	sessions   map[string]sessionDTO
	challenges map[string]loginChallengeDTO
}

type providerDTO struct {
	Name          string   `json:"name"`
	BaseURL       string   `json:"baseUrl"`
	DefaultModel  string   `json:"defaultModel"`
	Models        []string `json:"models"`
	Status        string   `json:"status"`
	P95LatencyMs  int      `json:"p95LatencyMs"`
	SuccessRate   float64  `json:"successRate"`
	RequestsToday int      `json:"requestsToday"`
}

type routingDTO struct {
	Policy   string `json:"policy"`
	Selector string `json:"selector"`
	Target   string `json:"target"`
	Status   string `json:"status"`
}

type tenantDTO struct {
	Tenant        string `json:"tenant"`
	Owner         string `json:"owner"`
	OwnerUsername string `json:"ownerUsername,omitempty"`
	DailyQuota    string `json:"dailyQuota"`
	Status        string `json:"status"`
}

type apiKeyDTO struct {
	ID        string `json:"id,omitempty"`
	Key       string `json:"key"`
	KeyHash   string `json:"-"`
	Tenant    string `json:"tenant"`
	Scope     string `json:"scope"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	LastUsed  string `json:"lastUsed"`
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
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
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
		mux:              http.NewServeMux(),
		config:           config,
		now:              time.Now,
		benchmarkStore:   store,
		operationalStore: operationalStore,
	}
	server.state = newStateStore(server.config, server.now)
	server.loadState()
	server.ensureBootstrapAdmin()
	server.routes()
	return withCORS(server.mux)
}

func (config Config) withDefaults() Config {
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
	return config
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
	server.mux.HandleFunc("GET /bff/admin/tenants", server.requireAdmin(server.handleAdminTenants))
	server.mux.HandleFunc("POST /bff/admin/tenants", server.requireAdmin(server.handleCreateTenant))
	server.mux.HandleFunc("PUT /bff/admin/tenants/{tenant}", server.requireAdmin(server.handleUpdateTenant))
	server.mux.HandleFunc("DELETE /bff/admin/tenants/{tenant}", server.requireAdmin(server.handleDeleteTenant))
	server.mux.HandleFunc("GET /bff/admin/api-keys", server.requireAdmin(server.handleAdminAPIKeys))
	server.mux.HandleFunc("POST /bff/admin/api-keys", server.requireAdmin(server.handleCreateAPIKey))
	server.mux.HandleFunc("DELETE /bff/admin/api-keys/{key}", server.requireAdmin(server.handleDeleteAPIKey))
	server.mux.HandleFunc("GET /bff/admin/audit", server.requireAdmin(server.handleAdminAudit))
	server.mux.HandleFunc("GET /bff/admin/audit.csv", server.requireAdmin(server.handleAuditCSV))
	server.mux.HandleFunc("GET /bff/admin/requests", server.requireAdmin(server.handleAdminRequests))
	server.mux.HandleFunc("GET /bff/admin/provider-health", server.requireAdmin(server.handleAdminProviderHealth))
	server.mux.HandleFunc("GET /bff/admin/request-logs", server.requireAdmin(server.handleAdminRequestLogs))
	server.mux.HandleFunc("GET /bff/admin/benchmarks", server.requireAdmin(server.handleAdminBenchmarks))
	server.mux.HandleFunc("GET /bff/customer/summary", server.requireCustomer(server.handleCustomerSummary))
	server.mux.HandleFunc("GET /bff/customer/usage", server.requireCustomer(server.handleCustomerUsage))
	server.mux.HandleFunc("GET /bff/customer/requests", server.requireCustomer(server.handleCustomerRequests))
	server.mux.HandleFunc("GET /bff/customer/api-keys", server.requireCustomer(server.handleCustomerAPIKeys))
	server.mux.HandleFunc("POST /bff/customer/api-keys", server.requireCustomer(server.handleCreateCustomerAPIKey))
	server.mux.HandleFunc("DELETE /bff/customer/api-keys/{id}", server.requireCustomer(server.handleDeleteCustomerAPIKey))
}

func (server *Server) handleGatewayHealth(w http.ResponseWriter, _ *http.Request) {
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
		server.registerCustomer(w, customerRegistrationInput{
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
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
		return
	}
	if server.findUserLocked(email) != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
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
	server.registerCustomer(w, input)
}

func (server *Server) registerCustomer(w http.ResponseWriter, input customerRegistrationInput) {
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

	server.state.mu.Lock()
	if server.findUserLocked(username) != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already taken"})
		return
	}
	if server.findUserLocked(email) != nil {
		server.state.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email already registered"})
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
	server.state.challenges[challengeID] = loginChallengeDTO{
		ID:        challengeID,
		Username:  username,
		Code:      code,
		ExpiresAt: server.now().Add(10 * time.Minute),
	}
	server.state.mu.Unlock()

	if err := server.sendVerificationEmail(user, code); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "account created but verification email could not be sent"})
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
	if !server.smtpConfigured() {
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to send verification code"})
		return
	}
	response := map[string]any{
		"verificationRequired": true,
		"challengeId":          challengeID,
		"delivery":             "email",
		"message":              "Verification code sent to " + userCopy.Email,
	}
	if !server.smtpConfigured() {
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
	if server.now().After(challenge.ExpiresAt) {
		delete(server.state.challenges, challengeID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "verification code expired"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(challenge.Code), []byte(code)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired verification code"})
		return
	}
	user := server.findUserLocked(challenge.Username)
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
	clearSessionCookie(w)
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
		next(w, r)
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
	if len(secret) <= 12 {
		return "****"
	}
	return secret[:7] + "..." + secret[len(secret)-4:]
}

func hashAPIKeySecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func (server *Server) handleAdminSummary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"defaultProvider": server.config.ProviderName,
		"defaultModel":    server.config.DefaultModel,
		"modelCount":      len(server.config.Models),
		"activeTenants":   4,
		"requestVolume":   18420,
		"successRate":     99.2,
		"p95LatencyMs":    842,
		"queueDepth":      17,
		"updatedAt":       server.now().UTC().Format(time.RFC3339),
	})
}

func (server *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	user, ok := server.userFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse(user))
}

func (server *Server) handleAdminProviders(w http.ResponseWriter, _ *http.Request) {
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

func (server *Server) handleAdminRouting(w http.ResponseWriter, _ *http.Request) {
	server.state.mu.Lock()
	rules := append([]routingDTO(nil), server.state.routing...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
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

func (server *Server) handleDeleteRouting(w http.ResponseWriter, r *http.Request) {
	policy := strings.TrimSpace(r.PathValue("policy"))
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

func (server *Server) handleAdminTenants(w http.ResponseWriter, _ *http.Request) {
	server.state.mu.Lock()
	tenants := append([]tenantDTO(nil), server.state.tenants...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
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

func (server *Server) handleAdminAPIKeys(w http.ResponseWriter, _ *http.Request) {
	server.state.mu.Lock()
	keys := append([]apiKeyDTO(nil), server.state.apiKeys...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
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
	key := apiKeyDTO{
		Key:      "vx-" + strings.ReplaceAll(strings.ToLower(input.Tenant), " ", "-"),
		Tenant:   strings.TrimSpace(input.Tenant),
		Scope:    strings.TrimSpace(input.Scope),
		LastUsed: "never",
	}
	server.state.mu.Lock()
	server.state.apiKeys = append(server.state.apiKeys, key)
	server.appendAuditLocked("admin", "Issued API key for "+key.Tenant, "Success")
	server.saveStateLocked()
	server.state.mu.Unlock()
	writeJSON(w, http.StatusCreated, key)
}

func (server *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	keyName := strings.TrimSpace(r.PathValue("key"))
	server.state.mu.Lock()
	defer server.state.mu.Unlock()
	for index, key := range server.state.apiKeys {
		if key.Key == keyName {
			server.state.apiKeys = append(server.state.apiKeys[:index], server.state.apiKeys[index+1:]...)
			server.appendAuditLocked("admin", "Deleted API key "+keyName, "Success")
			server.saveStateLocked()
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
}

func (server *Server) handleAdminAudit(w http.ResponseWriter, _ *http.Request) {
	server.state.mu.Lock()
	events := append([]auditDTO(nil), server.state.audit...)
	server.state.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (server *Server) handleAuditCSV(w http.ResponseWriter, _ *http.Request) {
	server.state.mu.Lock()
	events := append([]auditDTO(nil), server.state.audit...)
	server.state.mu.Unlock()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="veloxmesh-audit.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"time", "actor", "action", "result"})
	for _, event := range events {
		_ = writer.Write([]string{event.Time, event.Actor, event.Action, event.Result})
	}
	writer.Flush()
}

func (server *Server) handleAdminRequests(w http.ResponseWriter, _ *http.Request) {
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
	logs := snapshot.RequestLogs
	if len(logs) == 0 && server.config.DemoMode {
		logs = server.requestLogs()
		snapshot.Source = "demo"
	}
	if logs == nil {
		logs = []requestLogDTO{}
	}
	source := strings.TrimSpace(snapshot.Source)
	if source == "" {
		source = "empty"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs":        logs,
		"source":      source,
		"generatedAt": snapshot.GeneratedAt,
		"storage":     map[string]storageStatusDTO{"redis": snapshot.Redis},
	})
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
			{Key: "vx-dev", Tenant: "capstone-demo", Scope: "admin:read", LastUsed: "just now"},
			{Key: "vx-coursework", Tenant: "coursework-lab", Scope: "gateway:invoke", LastUsed: "12 min ago"},
			{Key: "vx-ops", Tenant: "ops-sandbox", Scope: "admin:write", LastUsed: "1 hour ago"},
		},
		audit: []auditDTO{
			{Time: now().Format("15:04"), Actor: "admin", Action: "Refreshed provider health", Result: "Success"},
			{Time: now().Format("15:04"), Actor: "gateway", Action: "Applied tenant quota", Result: "Rate Limited"},
			{Time: now().Format("15:04"), Actor: "admin", Action: "Viewed routing policy", Result: "Success"},
		},
		sessions:   map[string]sessionDTO{},
		challenges: map[string]loginChallengeDTO{},
	}
}

func (server *Server) appendAuditLocked(actor string, action string, result string) {
	server.state.audit = append([]auditDTO{{
		Time:   server.now().Format("15:04"),
		Actor:  actor,
		Action: action,
		Result: result,
	}}, server.state.audit...)
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
	server.state.challenges[challengeID] = loginChallengeDTO{
		ID:        challengeID,
		Username:  user.Username,
		Code:      code,
		ExpiresAt: server.now().Add(10 * time.Minute),
	}
	return challengeID, code, nil
}

func (server *Server) sendVerificationEmail(user userDTO, code string) error {
	subject := "VeloxMesh login verification code"
	body := "Your VeloxMesh login verification code is " + code + ". It expires in 10 minutes."
	if server.smtpConfigured() {
		return server.sendSMTP(user.Email, subject, body)
	}
	return server.writeEmailOutbox(user.Email, subject, body)
}

func (server *Server) smtpConfigured() bool {
	return strings.TrimSpace(server.config.SMTPHost) != "" &&
		strings.TrimSpace(server.config.SMTPUsername) != "" &&
		strings.TrimSpace(server.config.SMTPPassword) != "" &&
		strings.TrimSpace(server.config.SMTPFrom) != ""
}

func (server *Server) sendSMTP(to string, subject string, body string) error {
	addr := net.JoinHostPort(server.config.SMTPHost, server.config.SMTPPort)
	auth := smtp.PlainAuth("", server.config.SMTPUsername, server.config.SMTPPassword, server.config.SMTPHost)
	message := strings.Join([]string{
		"From: " + server.config.SMTPFrom,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	return smtp.SendMail(addr, auth, server.config.SMTPFrom, []string{to}, []byte(message))
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
		ExpiresAt: server.now().Add(8 * time.Hour),
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 8,
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

func randomDigits(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, value := range bytes {
		builder.WriteString(strconv.Itoa(int(value) % 10))
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

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
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
	server.state.providers = saved.Providers
	server.state.routing = saved.Routing
	server.state.tenants = saved.Tenants
	server.state.apiKeys = saved.APIKeys
	server.state.audit = saved.Audit
	server.state.users = saved.Users
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
