package bff

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestVerificationChallengeStoresOnlyHashAndExpiresAfterFiveMinutes(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	pepper := []byte("verification-test-pepper")
	challenge := newLoginChallenge("challenge-1", "alice", "123456", pepper, now)

	if challenge.CodeHash == "" || strings.Contains(challenge.CodeHash, "123456") {
		t.Fatalf("verification challenge must store only a non-plaintext hash: %+v", challenge)
	}
	if challenge.ExpiresAt != now.Add(5*time.Minute) {
		t.Fatalf("verification challenge expiry = %s, want %s", challenge.ExpiresAt, now.Add(5*time.Minute))
	}
	if got := challenge.verify("123456", pepper, now.Add(5*time.Minute+time.Nanosecond)); got != verificationExpired {
		t.Fatalf("verification after expiry = %v, want expired", got)
	}
}

func TestVerificationChallengeIsSingleUseAndLocksAfterFiveFailures(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	pepper := []byte("verification-test-pepper")
	challenge := newLoginChallenge("challenge-1", "alice", "123456", pepper, now)

	for attempt := 1; attempt <= 4; attempt++ {
		if got := challenge.verify("000000", pepper, now); got != verificationInvalid {
			t.Fatalf("attempt %d = %v, want invalid", attempt, got)
		}
	}
	if got := challenge.verify("000000", pepper, now); got != verificationExhausted {
		t.Fatalf("fifth failed attempt = %v, want exhausted", got)
	}
	if got := challenge.verify("123456", pepper, now); got != verificationExhausted {
		t.Fatalf("correct code after lockout = %v, want exhausted", got)
	}

	fresh := newLoginChallenge("challenge-2", "alice", "654321", pepper, now)
	if got := fresh.verify("654321", pepper, now); got != verificationAccepted {
		t.Fatalf("valid code = %v, want accepted", got)
	}
	if got := fresh.verify("654321", pepper, now); got != verificationConsumed {
		t.Fatalf("replayed code = %v, want consumed", got)
	}
}

func TestProductionWithoutSMTPRejectsVerificationWithoutDevCodeOrOutbox(t *testing.T) {
	outbox := t.TempDir() + "/verification.log"
	handler := NewServer(Config{
		DemoMode:        false,
		TestMode:        false,
		EmailOutboxPath: outbox,
	})

	response := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{
		"email":"secure@example.test",
		"username":"secure_customer",
		"organization":"Security",
		"password":"correct-horse-battery-staple",
		"confirmPassword":"correct-horse-battery-staple"
	}`, nil)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("production registration without SMTP = %d %s, want 503", response.Code, response.Body.String())
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "devcode") || strings.Contains(response.Body.String(), "123456") {
		t.Fatalf("production response leaked a verification code: %s", response.Body.String())
	}
	if _, err := os.Stat(outbox); !os.IsNotExist(err) {
		t.Fatalf("production mode wrote verification outbox %s", outbox)
	}
}

func TestVerificationSendRateLimitsNormalizedEmail(t *testing.T) {
	handler := NewServer(Config{
		TestMode:                   true,
		BootstrapAdminEmail:        "rate-admin@example.test",
		BootstrapAdminUsername:     "rate_admin",
		BootstrapAdminPassword:     "correct-horse-battery-staple",
		VerificationSendEmailLimit: 1,
		VerificationSendIPLimit:    10,
	})

	first := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{"identifier":"RATE-ADMIN@EXAMPLE.TEST","password":"correct-horse-battery-staple"}`, nil)
	second := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{"identifier":"rate-admin@example.test","password":"correct-horse-battery-staple"}`, nil)

	if first.Code != http.StatusOK {
		t.Fatalf("first verification send = %d %s", first.Code, first.Body.String())
	}
	assertRateLimited(t, second)
}

func TestVerificationSendAndVerifyRateLimitClientIP(t *testing.T) {
	sendLimited := NewServer(Config{
		TestMode:                   true,
		VerificationSendEmailLimit: 10,
		VerificationSendIPLimit:    1,
	})
	register := func(email string, username string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"email":%q,"username":%q,"organization":"Rate Test","password":"correct-horse-battery-staple","confirmPassword":"correct-horse-battery-staple"}`, email, username)
		return authRequest(t, sendLimited, http.MethodPost, "/bff/auth/customer/register", body, nil)
	}
	if first := register("first@example.test", "first_rate"); first.Code != http.StatusCreated {
		t.Fatalf("first IP send = %d %s", first.Code, first.Body.String())
	}
	assertRateLimited(t, register("second@example.test", "second_rate"))

	verifyLimited := NewServer(Config{
		TestMode:                     true,
		VerificationSendEmailLimit:   10,
		VerificationSendIPLimit:      10,
		VerificationVerifyEmailLimit: 10,
		VerificationVerifyIPLimit:    1,
	})
	registered := authRequest(t, verifyLimited, http.MethodPost, "/bff/auth/customer/register", `{"email":"verify@example.test","username":"verify_rate","organization":"Verify","password":"correct-horse-battery-staple","confirmPassword":"correct-horse-battery-staple"}`, nil)
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode verification challenge: %v", err)
	}
	wrongCode := "000000"
	if challenge.DevCode == wrongCode {
		wrongCode = "999999"
	}
	wrong := authRequest(t, verifyLimited, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{"challengeId":%q,"code":%q}`, challenge.ChallengeID, wrongCode), nil)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("first verification attempt = %d %s", wrong.Code, wrong.Body.String())
	}
	assertRateLimited(t, authRequest(t, verifyLimited, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{"challengeId":%q,"code":%q}`, challenge.ChallengeID, challenge.DevCode), nil))
}

func TestLoginDoesNotRevealWhetherAccountExists(t *testing.T) {
	handler := NewServer(Config{
		TestMode:               true,
		BootstrapAdminEmail:    "known@example.test",
		BootstrapAdminUsername: "known_admin",
		BootstrapAdminPassword: "correct-horse-battery-staple",
	})
	unknown := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{"identifier":"unknown@example.test","password":"wrong-password"}`, nil)
	known := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{"identifier":"known@example.test","password":"wrong-password"}`, nil)
	if unknown.Code != http.StatusUnauthorized || known.Code != http.StatusUnauthorized || unknown.Body.String() != known.Body.String() {
		t.Fatalf("login responses reveal account state: unknown=%d %s known=%d %s", unknown.Code, unknown.Body.String(), known.Code, known.Body.String())
	}
}

func TestRegistrationDoesNotRevealWhetherEmailOrUsernameExists(t *testing.T) {
	handler := NewServer(Config{TestMode: true})
	register := func(email string, username string) *httptest.ResponseRecorder {
		return authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", fmt.Sprintf(`{"email":%q,"username":%q,"organization":"Enumeration Test","password":"correct-horse-battery-staple","confirmPassword":"correct-horse-battery-staple"}`, email, username), nil)
	}
	if first := register("existing@example.test", "existing_customer"); first.Code != http.StatusCreated {
		t.Fatalf("initial registration = %d %s", first.Code, first.Body.String())
	}
	emailConflict := register("existing@example.test", "different_customer")
	usernameConflict := register("different@example.test", "existing_customer")
	if emailConflict.Code != http.StatusConflict || usernameConflict.Code != http.StatusConflict || emailConflict.Body.String() != usernameConflict.Body.String() {
		t.Fatalf("registration responses reveal conflict field: email=%d %s username=%d %s", emailConflict.Code, emailConflict.Body.String(), usernameConflict.Code, usernameConflict.Body.String())
	}
}

func TestProductionSessionCookieUsesSecurePolicyAndConfiguredLifetime(t *testing.T) {
	now := time.Date(2026, time.July, 18, 15, 0, 0, 0, time.UTC)
	sender := &recordingMailSender{}
	handler := NewServer(Config{MailSender: sender, SessionTTL: 30 * time.Minute, Now: func() time.Time { return now }})

	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{"email":"cookie@example.test","username":"cookie_customer","organization":"Cookie","password":"correct-horse-battery-staple","confirmPassword":"correct-horse-battery-staple"}`, nil)
	var challenge struct {
		ChallengeID string `json:"challengeId"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode registration challenge: %v", err)
	}
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(sender.body)
	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{"challengeId":%q,"code":%q}`, challenge.ChallengeID, code), nil)
	cookie := sessionCookie(t, verified)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("production session cookie policy = %+v", cookie)
	}
	if cookie.MaxAge != 30*60 || !cookie.Expires.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("production session cookie lifetime = max-age %d expires %s", cookie.MaxAge, cookie.Expires)
	}

	loggedOut := authRequest(t, handler, http.MethodPost, "/bff/auth/logout", `{}`, cookie)
	cleared := cookieNamed(loggedOut, sessionCookieName)
	if cleared == nil || cleared.MaxAge != -1 || !cleared.Secure || !cleared.HttpOnly || cleared.SameSite != http.SameSiteLaxMode {
		t.Fatalf("logout cookie did not preserve secure deletion policy: %+v", cleared)
	}
}

func TestServerRejectsSessionAfterConfiguredExpiry(t *testing.T) {
	now := time.Date(2026, time.July, 18, 15, 0, 0, 0, time.UTC)
	handler := NewServer(Config{TestMode: true, SessionTTL: 30 * time.Minute, Now: func() time.Time { return now }})
	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{"email":"expiry@example.test","username":"expiry_customer","organization":"Expiry","password":"correct-horse-battery-staple","confirmPassword":"correct-horse-battery-staple"}`, nil)
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("decode registration challenge: %v", err)
	}
	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{"challengeId":%q,"code":%q}`, challenge.ChallengeID, challenge.DevCode), nil)
	cookie := sessionCookie(t, verified)
	now = now.Add(30*time.Minute + time.Nanosecond)
	afterExpiry := authRequest(t, handler, http.MethodGet, "/bff/auth/session", "", cookie)
	if afterExpiry.Code != http.StatusUnauthorized {
		t.Fatalf("expired session = %d %s, want 401", afterExpiry.Code, afterExpiry.Body.String())
	}
}

func TestAPIKeyPlaintextIsCreateOnlyAndNeverLeaksThroughAdminData(t *testing.T) {
	statePath := t.TempDir() + "/secure-state.json"
	handler := NewServer(Config{
		DemoMode:               true,
		TestMode:               true,
		AllowAdminRegistration: true,
		StatePath:              statePath,
	})
	customerCookie, _ := registeredCustomerCookie(t, handler, "secret_customer", "Secret Customer")
	created := authRequest(t, handler, http.MethodPost, "/bff/customer/api-keys", `{"scope":"gateway:invoke"}`, customerCookie)
	var createdKey struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdKey); err != nil || !strings.HasPrefix(createdKey.Key, "vx_live_") {
		t.Fatalf("create response did not return one-time API key: %v %s", err, created.Body.String())
	}

	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read persisted state: %v", err)
	}
	if strings.Contains(string(state), createdKey.Key) || !strings.Contains(string(state), `"keyHash"`) || !strings.Contains(string(state), `"keyPrefix": "vx_live_"`) {
		t.Fatalf("persisted API key is not hash/prefix-only: %s", string(state))
	}
	listed := authRequest(t, handler, http.MethodGet, "/bff/customer/api-keys", "", customerCookie)
	if strings.Contains(listed.Body.String(), createdKey.Key) {
		t.Fatalf("API key list leaked one-time secret: %s", listed.Body.String())
	}

	admin := adminCookie(t, handler)
	for _, path := range []string{"/bff/admin/audit", "/bff/admin/audit.csv", "/bff/admin/settings"} {
		response := authRequest(t, handler, http.MethodGet, path, "", admin)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), createdKey.Key) {
			t.Fatalf("GET %s leaked a secret or failed: %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestCustomerIsForbiddenFromEveryAdminSecurityEndpoint(t *testing.T) {
	handler := NewServer(Config{DemoMode: true, TestMode: true})
	cookie, _ := registeredCustomerCookie(t, handler, "forbidden_customer", "Forbidden Customer")
	paths := []string{
		"/bff/admin/summary", "/bff/admin/providers", "/bff/admin/routing", "/bff/admin/tenants",
		"/bff/admin/api-keys", "/bff/admin/audit", "/bff/admin/audit.csv", "/bff/admin/settings",
		"/bff/admin/requests", "/bff/admin/provider-health", "/bff/admin/request-logs",
		"/bff/admin/benchmarks", "/bff/admin/benchmarks/raw.csv", "/bff/admin/benchmarks/export.zip",
	}
	for _, path := range paths {
		response := authRequest(t, handler, http.MethodGet, path, "", cookie)
		if response.Code != http.StatusForbidden {
			t.Fatalf("Customer GET %s = %d %s, want 403", path, response.Code, response.Body.String())
		}
	}
}

func assertRateLimited(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "verification_rate_limited") {
		t.Fatalf("rate-limited response = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited response is missing Retry-After")
	}
}
