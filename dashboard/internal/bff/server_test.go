package bff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func newTestServer(config Config) http.Handler {
	config.AllowAdminRegistration = true
	config.DemoMode = true
	return NewServer(config)
}

func TestAdminTenantsUseRealGatewayOutsideDemoMode(t *testing.T) {
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if r.URL.Path != "/admin/v1/tenants" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"tenant-a","name":"Tenant A","owner":"owner@example.test","daily_quota":2000,"status":"active","revision":1}]}`)
	}))
	defer upstream.Close()

	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		GatewayAdminURL:        upstream.URL,
		GatewayAdminAPIKey:     "server-only-admin-key",
		GatewayAPITimeout:      time.Second,
	})
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/tenants", "", adminCookie(t, handler))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if authorization != "Bearer server-only-admin-key" {
		t.Fatalf("upstream Authorization = %q", authorization)
	}
	if !strings.Contains(response.Body.String(), `"source":"veloxmesh-admin"`) || strings.Contains(response.Body.String(), "server-only-admin-key") {
		t.Fatalf("unexpected real Gateway response: %s", response.Body.String())
	}
}

func TestAdminManagementFailsClosedOutsideDemoMode(t *testing.T) {
	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true})
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/tenants", "", adminCookie(t, handler))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "dashboard-state") {
		t.Fatalf("production response silently fell back to local state: %s", response.Body.String())
	}
}

func TestAdminManagementMapsUpstreamAdminAuthFailureToBadGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid admin key"})
	}))
	defer upstream.Close()

	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayAdminAPIKey: "wrong-key"})
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/tenants", "", adminCookie(t, handler))
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"error":"gateway_admin_auth_failed"`) {
		t.Fatalf("upstream admin auth failure leaked as client auth response: %d %s", response.Code, response.Body.String())
	}
}

func TestAdminRequestsMarksLegacyUsageWithoutTenantAsPartial(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/v1/usage" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"legacy-usage","provider_id":"provider-a","model":"model-a","duration_ms":240,"timestamp":"2026-07-17T12:00:00Z","status":"settled"}]}`)
	}))
	defer upstream.Close()

	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayAdminAPIKey: "admin-key"})
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/requests", "", adminCookie(t, handler))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"partialData":true`) || !strings.Contains(body, "legacy usage rows") {
		t.Fatalf("legacy usage partial-data response = %d %s", response.Code, body)
	}
}

func TestAdminManagementReadsAllRealGatewayResources(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/admin/v1/providers":
			_, _ = io.WriteString(w, `{"data":[{"id":"provider-a","name":"Provider A","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a"}]}`)
		case "/admin/v1/routing":
			_, _ = io.WriteString(w, `{"id":"global","strategy":"latency","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":3}`)
		case "/admin/v1/api-keys":
			_, _ = io.WriteString(w, `{"data":[{"id":"key-a","tenant_id":"tenant-a","prefix":"vx_live_abc","name":"benchmark","role":"customer","enabled":true}]}`)
		case "/admin/v1/audit":
			_, _ = io.WriteString(w, `{"data":[{"id":"audit-a","actor":"admin","action":"routing.update","outcome":"success","timestamp":"2026-07-17T12:00:00Z"}]}`)
		case "/admin/v1/settings":
			_, _ = io.WriteString(w, `{"id":"global","default_provider":"provider-a","default_model":"model-a","request_timeout_seconds":30,"data_retention_days":30,"revision":2}`)
		case "/admin/v1/usage":
			_, _ = io.WriteString(w, `{"data":[{"id":"usage-a","tenant_id":"tenant-a","api_key_id":"key-a","provider_id":"provider-a","model":"model-a","prompt_tokens":10,"response_tokens":5,"total_tokens":15,"duration_ms":240,"timestamp":"2026-07-17T12:00:00Z","status":"settled"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayAdminAPIKey: "admin-key"})
	cookie := adminCookie(t, handler)

	for _, path := range []string{"/bff/admin/providers", "/bff/admin/routing", "/bff/admin/api-keys", "/bff/admin/audit", "/bff/admin/settings", "/bff/admin/requests"} {
		response := authRequest(t, handler, http.MethodGet, path, "", cookie)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source":"veloxmesh-admin"`) {
			t.Fatalf("GET %s returned %d: %s", path, response.Code, response.Body.String())
		}
		if path == "/bff/admin/routing" && (!strings.Contains(response.Body.String(), `"singleton":true`) || !strings.Contains(response.Body.String(), `"revision":3`)) {
			t.Fatalf("real routing response did not expose singleton/revision contract: %s", response.Body.String())
		}
	}
}

func TestAdminManagementWritesThroughToRealGateway(t *testing.T) {
	seen := map[string]int{}
	statePath := filepath.Join(t.TempDir(), "production-dashboard-state.json")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.Method+" "+r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /admin/v1/providers":
			_, _ = io.WriteString(w, `{"id":"provider-a","name":"Provider A","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":1}`)
		case "GET /admin/v1/providers/provider-a":
			_, _ = io.WriteString(w, `{"id":"provider-a","name":"Provider A","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":1}`)
		case "GET /admin/v1/providers":
			_, _ = io.WriteString(w, `{"data":[{"id":"provider-a","name":"Provider A","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":1}]}`)
		case "GET /v1/models":
			_, _ = io.WriteString(w, `{"data":[{"id":"model-a"}]}`)
		case "POST /v1/chat/completions":
			w.Header().Set("X-Provider", "provider-a")
			w.Header().Set("X-Routing-Strategy", "latency")
			w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
			_, _ = io.WriteString(w, `{"id":"verified","choices":[]}`)
		case "GET /admin/v1/routing":
			if seen["PUT /admin/v1/routing"] > 0 {
				_, _ = io.WriteString(w, `{"id":"global","strategy":"latency","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":3}`)
			} else {
				_, _ = io.WriteString(w, `{"id":"global","strategy":"round-robin","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":2}`)
			}
		case "PUT /admin/v1/routing":
			_, _ = io.WriteString(w, `{"id":"global","strategy":"latency","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":3}`)
		case "POST /admin/v1/api-keys":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"record":{"id":"key-a","tenant_id":"tenant-a","prefix":"vx_live_abc","name":"benchmark","role":"customer","enabled":true},"secret":"vx_live_one_time_secret"}`)
		case "GET /admin/v1/settings":
			_, _ = io.WriteString(w, `{"id":"global","default_provider":"provider-a","default_model":"model-a","request_timeout_seconds":30,"data_retention_days":30,"revision":4}`)
		case "PUT /admin/v1/settings":
			_, _ = io.WriteString(w, `{"id":"global","default_provider":"provider-a","default_model":"model-a","request_timeout_seconds":45,"data_retention_days":60,"revision":5}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, StatePath: statePath, GatewayAdminURL: upstream.URL, GatewayDataURL: upstream.URL, GatewayAdminAPIKey: "admin-key", GatewayDataAPIKey: "data-key"})
	cookie := adminCookie(t, handler)
	before := readManagementStateForTest(t, statePath)

	requests := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, "/bff/admin/providers", `{"name":"provider-a","baseUrl":"https://provider.example/v1","defaultModel":"model-a","models":["model-a"],"apiKey":"provider-secret"}`, http.StatusCreated},
		{http.MethodPost, "/bff/admin/routing", `{"policy":"global","selector":"latency","target":"provider-a","status":"Active"}`, http.StatusCreated},
		{http.MethodPost, "/bff/admin/api-keys", `{"tenant":"tenant-a","scope":"customer"}`, http.StatusCreated},
		{http.MethodPut, "/bff/admin/settings", `{"defaultProvider":"provider-a","defaultModel":"model-a","requestTimeoutSeconds":45,"dataRetentionDays":60}`, http.StatusOK},
	}
	for _, item := range requests {
		response := authRequest(t, handler, item.method, item.path, item.body, cookie)
		if response.Code != item.status {
			t.Fatalf("%s %s returned %d: %s", item.method, item.path, response.Code, response.Body.String())
		}
	}
	for _, call := range []string{"POST /admin/v1/providers", "PUT /admin/v1/routing", "POST /admin/v1/api-keys", "PUT /admin/v1/settings"} {
		if seen[call] != 1 {
			t.Fatalf("real Gateway call %s count = %d", call, seen[call])
		}
	}
	after := readManagementStateForTest(t, statePath)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("production Gateway writes mutated Dashboard-local management state: before=%v after=%v", before, after)
	}
}

func TestAdminManagementForwardsBrowserObservedRevisions(t *testing.T) {
	revisions := map[string]int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/v1/routing":
			if revisions["routing"] != 0 {
				_, _ = io.WriteString(w, `{"id":"global","strategy":"latency","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":4}`)
			} else {
				_, _ = io.WriteString(w, `{"id":"global","strategy":"round-robin","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":9}`)
			}
		case "PUT /admin/v1/routing":
			var body GatewayRoutingUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			revisions["routing"] = body.Revision
			_, _ = io.WriteString(w, `{"id":"global","strategy":"latency","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":4}`)
		case "GET /admin/v1/providers":
			_, _ = io.WriteString(w, `{"data":[{"id":"provider-a","name":"Provider A","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":1}]}`)
		case "POST /v1/chat/completions":
			w.Header().Set("X-Provider", "provider-a")
			w.Header().Set("X-Routing-Strategy", "latency")
			w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
			_, _ = io.WriteString(w, `{"id":"verified","choices":[]}`)
		case "GET /admin/v1/tenants":
			_, _ = io.WriteString(w, `{"data":[{"id":"tenant-a","name":"Tenant A","owner":"owner@example.test","daily_quota":1000,"status":"active","revision":9}]}`)
		case "PUT /admin/v1/tenants/tenant-a":
			var body GatewayTenantUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			revisions["tenant"] = body.Revision
			_, _ = io.WriteString(w, `{"id":"tenant-a","name":"Tenant A","owner":"owner@example.test","daily_quota":1000,"status":"active","revision":4}`)
		case "GET /admin/v1/settings":
			_, _ = io.WriteString(w, `{"id":"global","default_provider":"provider-a","default_model":"model-a","request_timeout_seconds":30,"data_retention_days":30,"revision":9}`)
		case "PUT /admin/v1/settings":
			var body GatewaySettingsUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			revisions["settings"] = body.Revision
			_, _ = io.WriteString(w, `{"id":"global","default_provider":"provider-a","default_model":"model-a","request_timeout_seconds":30,"data_retention_days":30,"revision":4}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayDataURL: upstream.URL, GatewayAdminAPIKey: "admin-key", GatewayDataAPIKey: "data-key"})
	cookie := adminCookie(t, handler)
	requests := []struct {
		path string
		body string
	}{
		{"/bff/admin/routing/global", `{"policy":"global","selector":"latency","target":"provider-a","status":"Active","revision":3}`},
		{"/bff/admin/tenants/tenant-a", `{"tenant":"tenant-a","owner":"owner@example.test","dailyQuota":"1000","status":"Healthy","revision":3}`},
		{"/bff/admin/settings", `{"defaultProvider":"provider-a","defaultModel":"model-a","requestTimeoutSeconds":30,"dataRetentionDays":30,"revision":3}`},
	}
	for _, request := range requests {
		response := authRequest(t, handler, http.MethodPut, request.path, request.body, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("PUT %s = %d: %s", request.path, response.Code, response.Body.String())
		}
	}
	for resource, revision := range revisions {
		if revision != 3 {
			t.Fatalf("%s revision = %d, want browser-observed 3", resource, revision)
		}
	}
}

func TestRoutingClosedLoopReportsWarningAndReadbackFailureTruthfully(t *testing.T) {
	t.Run("missing data key is warning after confirmed apply", func(t *testing.T) {
		written := false
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch r.Method + " " + r.URL.Path {
			case "GET /admin/v1/routing":
				if written {
					_, _ = io.WriteString(w, `{"id":"global","strategy":"default-provider","default_provider":"provider-b","fallback_enabled":true,"max_attempts":2,"revision":2}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"global","strategy":"default-provider","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":1}`)
				}
			case "PUT /admin/v1/routing":
				written = true
				_, _ = io.WriteString(w, `{"id":"global","strategy":"default-provider","default_provider":"provider-b","fallback_enabled":true,"max_attempts":2,"revision":2,"application":{"state":"applied","applied":true,"revision":2}}`)
			case "GET /admin/v1/providers":
				_, _ = io.WriteString(w, `{"data":[{"id":"provider-b","name":"Provider B","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":1}]}`)
			default:
				http.NotFound(w, r)
			}
		}))
		defer upstream.Close()
		handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayAdminAPIKey: "admin-key"})
		response := authRequest(t, handler, http.MethodPut, "/bff/admin/routing/global", `{"policy":"global","selector":"default-provider","target":"provider-b","status":"Active","revision":1}`, adminCookie(t, handler))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"state":"warning"`) || !strings.Contains(response.Body.String(), `"applied":true`) {
			t.Fatalf("warning response = %d %s", response.Code, response.Body.String())
		}
	})

	t.Run("stale readback is failed", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == http.MethodPut {
				_, _ = io.WriteString(w, `{"id":"global","strategy":"default-provider","default_provider":"provider-b","fallback_enabled":true,"max_attempts":2,"revision":2}`)
				return
			}
			_, _ = io.WriteString(w, `{"id":"global","strategy":"default-provider","default_provider":"provider-a","fallback_enabled":true,"max_attempts":2,"revision":1}`)
		}))
		defer upstream.Close()
		handler := NewServer(Config{AllowAdminRegistration: true, TestMode: true, GatewayAdminURL: upstream.URL, GatewayAdminAPIKey: "admin-key"})
		response := authRequest(t, handler, http.MethodPut, "/bff/admin/routing/global", `{"policy":"global","selector":"default-provider","target":"provider-b","status":"Active","revision":1}`, adminCookie(t, handler))
		if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), `"state":"failed"`) || strings.Contains(response.Body.String(), `"state":"verified"`) {
			t.Fatalf("failed readback response = %d %s", response.Code, response.Body.String())
		}
	})
}

func readManagementStateForTest(t *testing.T, path string) map[string]any {
	t.Helper()
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(encoded, &state); err != nil {
		t.Fatal(err)
	}
	return map[string]any{"providers": state["providers"], "routing": state["routing"], "tenants": state["tenants"], "apiKeys": state["apiKeys"], "audit": state["audit"], "settings": state["settings"]}
}

func TestHealthEndpoints(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		Models:       []string{"model-a", "model-b"},
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "gateway health", path: "/health"},
		{name: "bff health", path: "/bff/health"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if body["status"] != "ok" {
				t.Fatalf("expected status ok, got %v", body["status"])
			}
		})
	}
}

func TestAdminSummaryUsesProviderConfig(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		Models:       []string{"model-a", "model-b", "model-c"},
	})

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/summary", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		DefaultProvider string `json:"defaultProvider"`
		ModelCount      int    `json:"modelCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body.DefaultProvider != "sans-primary" {
		t.Fatalf("expected default provider sans-primary, got %q", body.DefaultProvider)
	}
	if body.ModelCount != 3 {
		t.Fatalf("expected 3 models, got %d", body.ModelCount)
	}
}

func TestAdminProvidersReturnsConfiguredModelList(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		BaseURL:      "https://example.test/v1",
		Models:       []string{"model-a", "model-b"},
	})

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/providers", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body struct {
		Providers []struct {
			Name    string   `json:"name"`
			BaseURL string   `json:"baseUrl"`
			Models  []string `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if len(body.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(body.Providers))
	}
	if body.Providers[0].Name != "sans-primary" {
		t.Fatalf("expected provider sans-primary, got %q", body.Providers[0].Name)
	}
	if len(body.Providers[0].Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(body.Providers[0].Models))
	}
}

func TestCreateProviderPersistsForSubsequentReads(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	body := strings.NewReader(`{
		"name": "backup-provider",
		"baseUrl": "https://backup.example/v1",
		"defaultModel": "backup/model",
		"models": ["backup/model", "backup/fast"]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/bff/admin/providers", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/bff/admin/providers", nil)
	req.AddCookie(adminCookie(t, handler))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var response struct {
		Providers []struct {
			Name   string   `json:"name"`
			Models []string `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if len(response.Providers) != 2 {
		t.Fatalf("expected 2 providers after create, got %d", len(response.Providers))
	}
	if response.Providers[1].Name != "backup-provider" {
		t.Fatalf("expected created provider, got %q", response.Providers[1].Name)
	}
	if len(response.Providers[1].Models) != 2 {
		t.Fatalf("expected created models to persist, got %d", len(response.Providers[1].Models))
	}
}

func TestStatePersistsAcrossServerRestart(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "admin-state.json")
	first := newTestServer(Config{
		ProviderName: "sans-primary",
		StatePath:    statePath,
	})

	postJSON(t, first, "/bff/admin/providers", `{
		"name": "durable-provider",
		"baseUrl": "https://durable.example/v1",
		"defaultModel": "durable/model",
		"models": ["durable/model"]
	}`)

	second := newTestServer(Config{
		ProviderName: "sans-primary",
		StatePath:    statePath,
	})

	assertGETContains(t, second, "/bff/admin/providers", "durable-provider")
}

func TestOperationalEndpointsReturnRequestLogsAndBenchmarks(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		DemoMode:     true,
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			Source: "empty",
		}},
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: []benchmarkRequestDTO{}, Source: "empty", Redis: storageStatusDTO{Status: "connected"},
		}},
		BenchmarkStore: fakeBenchmarkStore{
			snapshot: benchmarkSnapshot{
				Benchmarks: fallbackBenchmarks(),
				Source:     "fallback",
			},
		},
	})

	assertGETContains(t, handler, "/bff/admin/request-logs", "inputTokens")
	assertGETContains(t, handler, "/bff/admin/request-logs", "req_10291")
	assertGETContains(t, handler, "/bff/admin/benchmarks", "Local Baseline")
	assertGETContains(t, handler, "/bff/admin/benchmarks", "improvementPct")
}

func TestAdminBenchmarksReturnsStorageStatusFromConfiguredStore(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		BenchmarkStore: fakeBenchmarkStore{
			snapshot: benchmarkSnapshot{
				Benchmarks: []benchmarkDTO{
					{
						RunID:                 "run-123",
						Method:                "Our Gateway Method",
						Dataset:               "mmlu_20",
						RequestCount:          20,
						Concurrency:           1,
						TimeoutSettingSeconds: 120,
						Provider:              "openai-compatible",
						TargetModel:           "model-a",
						GatewayVersion:        "VeloxMesh",
						AvgLatencyMs:          float64Pointer(500),
						P50LatencyMs:          float64Pointer(450),
						P95LatencyMs:          float64Pointer(731),
						P99LatencyMs:          float64Pointer(800),
						TTFTMs:                float64Pointer(180),
						ThroughputRPS:         float64Pointer(2.4),
						SuccessRatePct:        95,
						ErrorRatePct:          5,
						TimeoutRatePct:        0,
						TestDate:              "2026-07-16T00:00:00Z",
						Source:                "gateway-runner",
						RawFilePath:           "reports/run-123",
						ExportID:              "run-123",
						Status:                "passed",
					},
				},
				Source:      "redis",
				GeneratedAt: "2026-07-16T00:05:00Z",
				Redis: storageStatusDTO{
					Status: "connected",
					Detail: "snapshot loaded",
				},
				Qdrant: storageStatusDTO{
					Status: "connected",
					Detail: "healthz ok",
				},
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/benchmarks", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response struct {
		Benchmarks []benchmarkDTO `json:"benchmarks"`
		Source     string         `json:"source"`
		Storage    struct {
			Redis  storageStatusDTO `json:"redis"`
			Qdrant storageStatusDTO `json:"qdrant"`
		} `json:"storage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.Source != "redis" {
		t.Fatalf("expected redis source, got %q", response.Source)
	}
	if response.Storage.Redis.Status != "connected" || response.Storage.Qdrant.Status != "connected" {
		t.Fatalf("expected connected stores, got %+v", response.Storage)
	}
	if len(response.Benchmarks) != 1 || response.Benchmarks[0].RunID != "run-123" {
		t.Fatalf("expected store benchmark row, got %+v", response.Benchmarks)
	}
	if response.Benchmarks[0].RequestCount != 20 || *response.Benchmarks[0].P95LatencyMs != 731 {
		t.Fatalf("expected complete numeric benchmark fields, got %+v", response.Benchmarks[0])
	}
}

func TestAdminBenchmarksReturnsEmptyRowsOutsideDemoMode(t *testing.T) {
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		ProviderName:           "sans-primary",
		DemoMode:               false,
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Source: "empty",
			Redis:  storageStatusDTO{Status: "connected", Detail: "no benchmark snapshot key"},
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/benchmarks", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var response struct {
		Benchmarks []benchmarkDTO `json:"benchmarks"`
		Source     string         `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if len(response.Benchmarks) != 0 || response.Source != "empty" {
		t.Fatalf("expected explicit empty live response, got %+v", response)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}

func TestQdrantStatusReportsBenchmarkCollection(t *testing.T) {
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("healthz check passed"))
			return
		}
		if r.URL.Path == "/collections/veloxmesh_benchmarks" {
			writeJSON(w, http.StatusOK, map[string]any{
				"result": map[string]any{
					"points_count": 1,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer qdrant.Close()

	status := liveBenchmarkStore{
		qdrantURL:        qdrant.URL,
		qdrantCollection: "veloxmesh_benchmarks",
		httpClient:       qdrant.Client(),
	}.qdrantStatus(context.Background())

	if status.Status != "connected" {
		t.Fatalf("expected connected qdrant status, got %+v", status)
	}
	if !strings.Contains(status.Detail, "veloxmesh_benchmarks") || !strings.Contains(status.Detail, "1 point") {
		t.Fatalf("expected collection detail with point count, got %+v", status)
	}
}

func TestUpdateProviderAndDeleteResources(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	postJSON(t, handler, "/bff/admin/providers", `{
		"name": "editable-provider",
		"baseUrl": "https://old.example/v1",
		"defaultModel": "old/model",
		"models": ["old/model"]
	}`)
	putJSON(t, handler, "/bff/admin/providers/editable-provider", `{
		"baseUrl": "https://new.example/v1",
		"defaultModel": "new/model",
		"models": ["new/model", "new/fast"],
		"status": "degraded"
	}`)

	assertGETContains(t, handler, "/bff/admin/providers", "https://new.example/v1")
	assertGETContains(t, handler, "/bff/admin/providers", "new/fast")

	postJSON(t, handler, "/bff/admin/tenants", `{
		"tenant": "delete-team",
		"owner": "QA",
		"dailyQuota": "1,000",
		"status": "Healthy"
	}`)
	deleteResource(t, handler, "/bff/admin/tenants/delete-team")
	assertGETNotContains(t, handler, "/bff/admin/tenants", "delete-team")

	createdKey := authRequest(t, handler, http.MethodPost, "/bff/admin/api-keys", `{
		"tenant": "delete-key-team",
		"scope": "gateway:invoke"
	}`, adminCookie(t, handler))
	if createdKey.Code != http.StatusCreated {
		t.Fatalf("expected API key status 201, got %d: %s", createdKey.Code, createdKey.Body.String())
	}
	var createdKeyBody struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createdKey.Body.Bytes(), &createdKeyBody); err != nil || createdKeyBody.ID == "" {
		t.Fatalf("expected created API key ID, got %s", createdKey.Body.String())
	}
	deleteResource(t, handler, "/bff/admin/api-keys/"+createdKeyBody.ID)
	assertGETNotContains(t, handler, "/bff/admin/api-keys", "delete-key-team")

	assertGETContains(t, handler, "/bff/admin/audit", "Updated provider editable-provider")
	assertGETContains(t, handler, "/bff/admin/audit", "Deleted tenant delete-team")
	assertGETContains(t, handler, "/bff/admin/audit", "Revoked API key "+createdKeyBody.ID)
}

func TestUpdateAndDeleteMissingResourcesReturnNotFound(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	req := httptest.NewRequest(http.MethodPut, "/bff/admin/providers/missing-provider", strings.NewReader(`{
		"baseUrl": "https://missing.example/v1",
		"defaultModel": "missing/model",
		"models": ["missing/model"]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing provider update to return 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "provider not found") {
		t.Fatalf("expected stable not found error, got %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/bff/admin/tenants/missing-tenant", nil)
	req.AddCookie(adminCookie(t, handler))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing tenant delete to return 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "tenant not found") {
		t.Fatalf("expected stable not found error, got %s", rec.Body.String())
	}
}

func TestAdminSessionReturnsRoleAndScopes(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/session", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var response struct {
		User   string   `json:"user"`
		Role   string   `json:"role"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if response.User == "" || response.Role != "Admin" {
		t.Fatalf("unexpected session response: %+v", response)
	}
	if len(response.Scopes) == 0 || response.Scopes[0] != "admin:write" {
		t.Fatalf("expected admin:write scope, got %+v", response.Scopes)
	}
}

func TestAdminEndpointsRequireAuthenticatedAdmin(t *testing.T) {
	outboxPath := filepath.Join(t.TempDir(), "email-outbox.log")
	handler := newTestServer(Config{ProviderName: "sans-primary", EmailOutboxPath: outboxPath})

	unauthenticated := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated admin request to return 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}

	customerCookie := authenticatedCookie(t, handler, outboxPath, "customer@example.test", "customer_user", "Customer")
	customer := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", customerCookie)
	if customer.Code != http.StatusForbidden {
		t.Fatalf("expected customer admin request to return 403, got %d: %s", customer.Code, customer.Body.String())
	}

	adminCookie := authenticatedCookie(t, handler, outboxPath, "admin@example.test", "admin_user", "Admin")
	admin := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie)
	if admin.Code != http.StatusOK {
		t.Fatalf("expected admin request to return 200, got %d: %s", admin.Code, admin.Body.String())
	}
}

func TestRegisterPersistsUserWithoutCreatingSessionAndLoginRequiresVerification(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "admin-state.json")
	outboxPath := filepath.Join(t.TempDir(), "email-outbox.log")
	handler := newTestServer(Config{ProviderName: "sans-primary", StatePath: statePath, EmailOutboxPath: outboxPath})

	rec := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "owner@example.test",
		"username": "capstone_owner",
		"password": "correct-horse"
	}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if cookieNamed(rec, "veloxmesh_session") != nil {
		t.Fatalf("registration should not create a session cookie")
	}
	if !strings.Contains(rec.Body.String(), `"status":"registered"`) {
		t.Fatalf("expected registered response, got %s", rec.Body.String())
	}

	session := authRequest(t, handler, http.MethodGet, "/bff/session", "", nil)
	if session.Code != http.StatusUnauthorized {
		t.Fatalf("expected no session after register, got %d: %s", session.Code, session.Body.String())
	}

	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{
		"identifier": "owner@example.test",
		"password": "correct-horse"
	}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected login challenge status 200, got %d: %s", login.Code, login.Body.String())
	}
	if cookieNamed(login, "veloxmesh_session") != nil {
		t.Fatalf("password login should not create a session cookie before verification")
	}
	if !strings.Contains(login.Body.String(), `"verificationRequired":true`) || !strings.Contains(login.Body.String(), `"challengeId"`) {
		t.Fatalf("expected verification challenge response, got %s", login.Body.String())
	}

	var challenge struct {
		ChallengeID string `json:"challengeId"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("login response is not JSON: %v", err)
	}
	if strings.TrimSpace(challenge.ChallengeID) == "" {
		t.Fatalf("expected challenge id in login response")
	}
	code := verificationCodeFromOutbox(t, outboxPath)
	if len(code) != 6 {
		t.Fatalf("expected six-digit code, got %q", code)
	}

	wrong := authRequest(t, handler, http.MethodPost, "/bff/auth/verify-login", fmt.Sprintf(`{
		"challengeId": %q,
		"code": "000000"
	}`, challenge.ChallengeID), nil)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong verification status 401, got %d: %s", wrong.Code, wrong.Body.String())
	}

	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify-login", fmt.Sprintf(`{
		"challengeId": %q,
		"code": %q
	}`, challenge.ChallengeID, code), nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("expected verified status 200, got %d: %s", verified.Code, verified.Body.String())
	}
	cookie := sessionCookie(t, verified)

	authenticatedSession := authRequest(t, handler, http.MethodGet, "/bff/session", "", cookie)
	if authenticatedSession.Code != http.StatusOK {
		t.Fatalf("expected session status 200 after verification, got %d: %s", authenticatedSession.Code, authenticatedSession.Body.String())
	}
	if !strings.Contains(authenticatedSession.Body.String(), "capstone_owner") || !strings.Contains(authenticatedSession.Body.String(), "Admin") {
		t.Fatalf("expected registered admin session, got %s", authenticatedSession.Body.String())
	}

	restartedOutboxPath := filepath.Join(t.TempDir(), "restarted-email-outbox.log")
	restarted := newTestServer(Config{ProviderName: "sans-primary", StatePath: statePath, EmailOutboxPath: restartedOutboxPath})
	persistedLogin := authRequest(t, restarted, http.MethodPost, "/bff/auth/login", `{
		"identifier": "owner@example.test",
		"password": "correct-horse"
	}`, nil)
	if persistedLogin.Code != http.StatusOK {
		t.Fatalf("expected persisted user login challenge status 200, got %d: %s", persistedLogin.Code, persistedLogin.Body.String())
	}
	if !strings.Contains(persistedLogin.Body.String(), `"verificationRequired":true`) {
		t.Fatalf("expected persisted login to require verification, got %s", persistedLogin.Body.String())
	}
}

func TestRoleSpecificLoginEnforcesStoredRoleBeforeCreatingChallenge(t *testing.T) {
	tests := []struct {
		name          string
		storedRole    string
		wrongPortal   string
		correctPortal string
		wrongRole     string
		wantMessage   string
	}{
		{
			name:          "Admin account",
			storedRole:    "Admin",
			wrongPortal:   "/bff/auth/customer/login",
			correctPortal: "/bff/auth/admin/login",
			wrongRole:     "Customer",
			wantMessage:   "This account does not have access to the Customer portal",
		},
		{
			name:          "Customer account",
			storedRole:    "Customer",
			wrongPortal:   "/bff/auth/admin/login",
			correctPortal: "/bff/auth/customer/login",
			wrongRole:     "Admin",
			wantMessage:   "This account does not have access to the Admin portal",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outboxPath := filepath.Join(t.TempDir(), "email-outbox.log")
			sendLimit := 1
			if test.storedRole == "Customer" {
				sendLimit = 2
			}
			handler := newTestServer(Config{
				EmailOutboxPath:            outboxPath,
				VerificationSendEmailLimit: sendLimit,
			})
			username := strings.ToLower(test.storedRole) + "_portal_user"
			password := "correct-horse"
			registered := authRequest(t, handler, http.MethodPost, "/bff/auth/register", fmt.Sprintf(`{
				"email": %q,
				"username": %q,
				"password": %q,
				"role": %q
			}`, username+"@example.test", username, password, test.storedRole), nil)
			if registered.Code != http.StatusCreated {
				t.Fatalf("expected %s registration status 201, got %d: %s", test.storedRole, registered.Code, registered.Body.String())
			}
			outboxBefore, err := os.ReadFile(outboxPath)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("read verification outbox before wrong-portal login: %v", err)
			}

			wrongPortal := authRequest(t, handler, http.MethodPost, test.wrongPortal, fmt.Sprintf(`{
				"identifier": %q,
				"password": %q,
				"role": %q
			}`, username, password, test.wrongRole), nil)
			if wrongPortal.Code != http.StatusForbidden {
				t.Fatalf("expected wrong-portal status 403, got %d: %s", wrongPortal.Code, wrongPortal.Body.String())
			}
			if !strings.Contains(wrongPortal.Body.String(), test.wantMessage) {
				t.Fatalf("expected portal-specific error %q, got %s", test.wantMessage, wrongPortal.Body.String())
			}
			if strings.Contains(wrongPortal.Body.String(), `"challengeId"`) {
				t.Fatalf("wrong-portal response must not contain a challenge: %s", wrongPortal.Body.String())
			}
			if cookieNamed(wrongPortal, sessionCookieName) != nil {
				t.Fatalf("wrong-portal response must not create a session cookie")
			}
			outboxAfter, err := os.ReadFile(outboxPath)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("read verification outbox after wrong-portal login: %v", err)
			}
			if !bytes.Equal(outboxAfter, outboxBefore) {
				t.Fatalf("wrong-portal login must not send a verification challenge")
			}

			correctPortal := authRequest(t, handler, http.MethodPost, test.correctPortal, fmt.Sprintf(`{
				"identifier": %q,
				"password": %q,
				"role": %q
			}`, username, password, test.wrongRole), nil)
			if correctPortal.Code != http.StatusOK {
				t.Fatalf("expected correct-portal challenge status 200, got %d: %s", correctPortal.Code, correctPortal.Body.String())
			}
			if !strings.Contains(correctPortal.Body.String(), `"verificationRequired":true`) || !strings.Contains(correctPortal.Body.String(), `"challengeId"`) {
				t.Fatalf("expected correct-portal verification challenge, got %s", correctPortal.Body.String())
			}
			if cookieNamed(correctPortal, sessionCookieName) != nil {
				t.Fatalf("correct-portal password login must not create a session before verification")
			}
		})
	}
}

func TestRoleSpecificLoginKeepsGenericEndpointCompatible(t *testing.T) {
	handler := newTestServer(Config{})
	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "generic-login@example.test",
		"username": "generic_login",
		"password": "correct-horse",
		"role": "Customer"
	}`, nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("expected registration status 201, got %d: %s", registered.Code, registered.Body.String())
	}

	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{
		"identifier": "generic_login",
		"password": "correct-horse"
	}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected generic login challenge status 200, got %d: %s", login.Code, login.Body.String())
	}
	if !strings.Contains(login.Body.String(), `"verificationRequired":true`) || !strings.Contains(login.Body.String(), `"challengeId"`) {
		t.Fatalf("expected generic login verification challenge, got %s", login.Body.String())
	}
}

func TestRegisterAssignsAdminAndCustomerRoles(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	admin := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "admin@example.test",
		"username": "admin_user",
		"password": "1234",
		"role": "Admin"
	}`, nil)
	if admin.Code != http.StatusCreated {
		t.Fatalf("expected admin register status 201, got %d: %s", admin.Code, admin.Body.String())
	}
	if !strings.Contains(admin.Body.String(), `"role":"Admin"`) || !strings.Contains(admin.Body.String(), "admin:write") {
		t.Fatalf("expected admin role and scopes, got %s", admin.Body.String())
	}

	customer := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "customer@example.test",
		"username": "cust_user",
		"password": "1234",
		"role": "Customer"
	}`, nil)
	if customer.Code != http.StatusCreated {
		t.Fatalf("expected customer register status 201, got %d: %s", customer.Code, customer.Body.String())
	}
	if !strings.Contains(customer.Body.String(), `"role":"Customer"`) || !strings.Contains(customer.Body.String(), "gateway:invoke") {
		t.Fatalf("expected customer role and scope, got %s", customer.Body.String())
	}
	if strings.Contains(customer.Body.String(), "admin:write") {
		t.Fatalf("customer response should not include admin scope, got %s", customer.Body.String())
	}
}

func TestRegisterRejectsUnknownRole(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	rec := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "badrole@example.test",
		"username": "badrole_user",
		"password": "1234",
		"role": "Owner"
	}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unknown role status 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterAcceptsFourCharacterUsernameAndPassword(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	rec := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "four@example.test",
		"username": "four",
		"password": "1234"
	}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected four-character username and password to register, got %d: %s", rec.Code, rec.Body.String())
	}

	shortUsername := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "tiny@example.test",
		"username": "abc",
		"password": "1234"
	}`, nil)
	if shortUsername.Code != http.StatusBadRequest {
		t.Fatalf("expected short username status 400, got %d: %s", shortUsername.Code, shortUsername.Body.String())
	}

	shortPassword := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "shortpass@example.test",
		"username": "validname",
		"password": "123"
	}`, nil)
	if shortPassword.Code != http.StatusBadRequest {
		t.Fatalf("expected short password status 400, got %d: %s", shortPassword.Code, shortPassword.Body.String())
	}
}

func TestAuthRejectsDuplicateUsernameAndInvalidPassword(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	first := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "first@example.test",
		"username": "shared_name",
		"password": "one-password"
	}`, nil)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first register status 201, got %d: %s", first.Code, first.Body.String())
	}

	duplicate := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "second@example.test",
		"username": "shared_name",
		"password": "two-password"
	}`, nil)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("expected duplicate username status 409, got %d: %s", duplicate.Code, duplicate.Body.String())
	}

	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{
		"identifier": "shared_name",
		"password": "wrong-password"
	}`, nil)
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid login status 401, got %d: %s", login.Code, login.Body.String())
	}
}

func TestLogoutClearsAuthenticatedSession(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	register := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "logout@example.test",
		"username": "logout_owner",
		"password": "temporary-secret"
	}`, nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d: %s", register.Code, register.Body.String())
	}
	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{
		"identifier": "logout_owner",
		"password": "temporary-secret"
	}`, nil)
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("login response is not JSON: %v", err)
	}
	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify-login", fmt.Sprintf(`{
		"challengeId": %q,
		"code": %q
	}`, challenge.ChallengeID, challenge.DevCode), nil)
	cookie := sessionCookie(t, verified)

	logout := authRequest(t, handler, http.MethodPost, "/bff/auth/logout", "", cookie)
	if logout.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d: %s", logout.Code, logout.Body.String())
	}

	session := authRequest(t, handler, http.MethodGet, "/bff/session", "", cookie)
	if session.Code != http.StatusUnauthorized {
		t.Fatalf("expected logged out session status 401, got %d: %s", session.Code, session.Body.String())
	}
}

func TestCreateManagementResourcesAndExportAuditCSV(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})

	postJSON(t, handler, "/bff/admin/routing", `{
		"policy": "Cost cap",
		"selector": "cost-aware",
		"target": "backup-provider",
		"status": "Draft"
	}`)
	postJSON(t, handler, "/bff/admin/tenants", `{
		"tenant": "new-team",
		"owner": "Capstone",
		"dailyQuota": "2,500",
		"status": "Healthy"
	}`)
	postJSON(t, handler, "/bff/admin/api-keys", `{
		"tenant": "new-team",
		"scope": "gateway:invoke"
	}`)

	assertGETContains(t, handler, "/bff/admin/routing", "Cost cap")
	assertGETContains(t, handler, "/bff/admin/tenants", "new-team")
	assertGETContains(t, handler, "/bff/admin/api-keys", "gateway:invoke")

	req := httptest.NewRequest(http.MethodGet, "/bff/admin/audit.csv", nil)
	req.AddCookie(adminCookie(t, handler))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected CSV status 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/csv") {
		t.Fatalf("expected text/csv content type, got %q", got)
	}
	csv := rec.Body.String()
	for _, want := range []string{"time,actor,action,result", "Created routing rule", "Created tenant", "Issued API key"} {
		if !strings.Contains(csv, want) {
			t.Fatalf("expected CSV to contain %q, got %s", want, csv)
		}
	}
}

func TestAdminManagementResponsesDeclareDashboardStateAsPartial(t *testing.T) {
	handler := newTestServer(Config{ProviderName: "sans-primary"})
	for _, path := range []string{
		"/bff/admin/routing",
		"/bff/admin/tenants",
		"/bff/admin/api-keys",
		"/bff/admin/audit",
	} {
		response := authRequest(t, handler, http.MethodGet, path, "", adminCookie(t, handler))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d: %s", path, response.Code, response.Body.String())
		}
		body := response.Body.String()
		if !strings.Contains(body, `"source":"dashboard-state"`) || !strings.Contains(body, `"partialData":true`) {
			t.Fatalf("GET %s must identify local partial data, got %s", path, body)
		}
	}
}

func TestAdminSettingsAreSafeValidatedAndPersisted(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "admin-settings-state.json")
	config := Config{
		ProviderName: "sans-primary",
		DefaultModel: "model-a",
		StatePath:    statePath,
		DevAPIKey:    "dev-secret-value",
		QdrantAPIKey: "qdrant-secret-value",
		SMTPPassword: "smtp-secret-value",
		SMTPHost:     "smtp.example.test",
		SMTPUsername: "",
		SMTPFrom:     "",
	}
	handler := newTestServer(config)
	cookie := adminCookie(t, handler)

	initial := authRequest(t, handler, http.MethodGet, "/bff/admin/settings", "", cookie)
	if initial.Code != http.StatusOK {
		t.Fatalf("expected settings status 200, got %d: %s", initial.Code, initial.Body.String())
	}
	for _, secret := range []string{config.DevAPIKey, config.QdrantAPIKey, config.SMTPPassword} {
		if strings.Contains(initial.Body.String(), secret) {
			t.Fatalf("settings response leaked secret %q: %s", secret, initial.Body.String())
		}
	}
	for _, want := range []string{`"smtp":"Not configured"`, `"source":"dashboard-state"`, `"partialData":true`} {
		if !strings.Contains(initial.Body.String(), want) {
			t.Fatalf("settings response missing %s: %s", want, initial.Body.String())
		}
	}

	updated := authRequest(t, handler, http.MethodPut, "/bff/admin/settings", `{
		"defaultProvider":"backup-provider",
		"defaultModel":"model-b",
		"requestTimeoutSeconds":45,
		"dataRetentionDays":60
	}`, cookie)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"defaultProvider":"backup-provider"`) {
		t.Fatalf("expected settings update, got %d: %s", updated.Code, updated.Body.String())
	}

	invalid := authRequest(t, handler, http.MethodPut, "/bff/admin/settings", `{
		"defaultProvider":"backup-provider",
		"defaultModel":"model-b",
		"requestTimeoutSeconds":0,
		"dataRetentionDays":60
	}`, cookie)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected invalid settings status 422, got %d: %s", invalid.Code, invalid.Body.String())
	}

	restarted := newTestServer(Config{ProviderName: "sans-primary", DefaultModel: "model-a", StatePath: statePath})
	persisted := authRequest(t, restarted, http.MethodGet, "/bff/admin/settings", "", adminCookie(t, restarted))
	for _, want := range []string{`"defaultProvider":"backup-provider"`, `"defaultModel":"model-b"`, `"dataRetentionDays":60`} {
		if !strings.Contains(persisted.Body.String(), want) {
			t.Fatalf("persisted settings missing %s: %s", want, persisted.Body.String())
		}
	}
}

func TestAdminAPIKeySecretIsShownOnceAndStoredAsHash(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "admin-key-state.json")
	handler := newTestServer(Config{StatePath: statePath})
	cookie := adminCookie(t, handler)
	created := authRequest(t, handler, http.MethodPost, "/bff/admin/api-keys", `{
		"tenant":"new-team",
		"scope":"gateway:invoke"
	}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected API key status 201, got %d: %s", created.Code, created.Body.String())
	}
	var response struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		MaskedKey string `json:"maskedKey"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID == "" || response.Key == "" || response.MaskedKey == "" || response.Key == response.MaskedKey {
		t.Fatalf("expected one-time secret and masked value, got %+v", response)
	}

	listed := authRequest(t, handler, http.MethodGet, "/bff/admin/api-keys", "", cookie)
	if !strings.Contains(listed.Body.String(), response.MaskedKey) || strings.Contains(listed.Body.String(), response.Key) {
		t.Fatalf("API key list must contain only masked value, got %s", listed.Body.String())
	}
	state, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), response.Key) || !strings.Contains(string(state), hashAPIKeySecret(response.Key)) {
		t.Fatalf("state must store hash but not secret: %s", string(state))
	}
}

func TestAdminSettingsAndAPIKeyFailWithoutMutatingStateWhenPersistenceFails(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state-directory")
	if err := os.Mkdir(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(Config{ProviderName: "sans-primary", DefaultModel: "model-a", StatePath: statePath})
	cookie := adminCookie(t, handler)

	settings := authRequest(t, handler, http.MethodPut, "/bff/admin/settings", `{
		"defaultProvider":"must-not-stick",
		"defaultModel":"must-not-stick",
		"requestTimeoutSeconds":45,
		"dataRetentionDays":60
	}`, cookie)
	if settings.Code != http.StatusInternalServerError {
		t.Fatalf("expected settings persistence failure status 500, got %d: %s", settings.Code, settings.Body.String())
	}
	currentSettings := authRequest(t, handler, http.MethodGet, "/bff/admin/settings", "", cookie)
	if strings.Contains(currentSettings.Body.String(), "must-not-stick") {
		t.Fatalf("failed settings write must not mutate memory: %s", currentSettings.Body.String())
	}

	created := authRequest(t, handler, http.MethodPost, "/bff/admin/api-keys", `{
		"tenant":"must-not-stick",
		"scope":"gateway:invoke"
	}`, cookie)
	if created.Code != http.StatusInternalServerError || strings.Contains(created.Body.String(), "vx_admin_") {
		t.Fatalf("expected API key persistence failure without secret, got %d: %s", created.Code, created.Body.String())
	}
	listed := authRequest(t, handler, http.MethodGet, "/bff/admin/api-keys", "", cookie)
	if strings.Contains(listed.Body.String(), "must-not-stick") {
		t.Fatalf("failed API key write must not mutate memory: %s", listed.Body.String())
	}
}

func TestLegacyPlaintextAdminAPIKeysAreMigratedBeforeTheyAreListed(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "legacy-state.json")
	legacySecret := "vx-legacy-plaintext-secret"
	legacyState := fmt.Sprintf(`{
		"providers":[],
		"routing":[],
		"tenants":[],
		"apiKeys":[{"key":%q,"tenant":"legacy-team","scope":"admin:read","lastUsed":"never"}],
		"audit":[],
		"users":[]
	}`, legacySecret)
	if err := os.WriteFile(statePath, []byte(legacyState), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(Config{StatePath: statePath})
	listed := authRequest(t, handler, http.MethodGet, "/bff/admin/api-keys", "", adminCookie(t, handler))
	if listed.Code != http.StatusOK {
		t.Fatalf("expected API key list status 200, got %d: %s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), legacySecret) || !strings.Contains(listed.Body.String(), maskAPIKeySecret(legacySecret)) {
		t.Fatalf("legacy API key list must expose only the masked value: %s", listed.Body.String())
	}
	persisted, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), legacySecret) || !strings.Contains(string(persisted), hashAPIKeySecret(legacySecret)) {
		t.Fatalf("legacy state must replace plaintext with a hash: %s", string(persisted))
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	addAdminCookieIfNeeded(t, handler, req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST %s expected 201, got %d: %s", path, rec.Code, rec.Body.String())
	}
}

func putJSON(t *testing.T, handler http.Handler, path string, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	addAdminCookieIfNeeded(t, handler, req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT %s expected 200, got %d: %s", path, rec.Code, rec.Body.String())
	}
}

func deleteResource(t *testing.T, handler http.Handler, path string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	addAdminCookieIfNeeded(t, handler, req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE %s expected 200, got %d: %s", path, rec.Code, rec.Body.String())
	}
}

func assertGETContains(t *testing.T, handler http.Handler, path string, want string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	addAdminCookieIfNeeded(t, handler, req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
	}
	data, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed reading response: %v", err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("GET %s expected %q in %s", path, want, string(data))
	}
}

func assertGETNotContains(t *testing.T, handler http.Handler, path string, unwanted string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	addAdminCookieIfNeeded(t, handler, req)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
	}
	data, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("failed reading response: %v", err)
	}
	if strings.Contains(string(data), unwanted) {
		t.Fatalf("GET %s did not expect %q in %s", path, unwanted, string(data))
	}
}

func authRequest(t *testing.T, handler http.Handler, method string, path string, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	if cookie := cookieNamed(rec, "veloxmesh_session"); cookie != nil {
		return cookie
	}
	t.Fatalf("expected veloxmesh_session cookie, got headers %+v", rec.Result().Header)
	return nil
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func verificationCodeFromOutbox(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected verification outbox at %s: %v", path, err)
	}
	matches := regexp.MustCompile(`\b\d{6}\b`).FindAllString(string(data), -1)
	if len(matches) == 0 {
		t.Fatalf("expected six-digit verification code in outbox, got %s", string(data))
	}
	return matches[len(matches)-1]
}

func addAdminCookieIfNeeded(t *testing.T, handler http.Handler, req *http.Request) {
	t.Helper()
	if strings.HasPrefix(req.URL.Path, "/bff/admin/") {
		req.AddCookie(adminCookie(t, handler))
	}
}

func adminCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	suffix, err := randomHex(4)
	if err != nil {
		t.Fatalf("failed to create admin test suffix: %v", err)
	}
	username := "admin_" + suffix
	return authenticatedCookie(t, handler, "", username+"@example.test", username, "Admin")
}

func authenticatedCookie(t *testing.T, handler http.Handler, outboxPath string, email string, username string, role string) *http.Cookie {
	t.Helper()
	password := "correct-horse"
	register := authRequest(t, handler, http.MethodPost, "/bff/auth/register", fmt.Sprintf(`{
		"email": %q,
		"username": %q,
		"password": %q,
		"role": %q
	}`, email, username, password, role), nil)
	if register.Code != http.StatusCreated {
		t.Fatalf("expected register status 201, got %d: %s", register.Code, register.Body.String())
	}

	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", fmt.Sprintf(`{
		"identifier": %q,
		"password": %q
	}`, username, password), nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected login challenge status 200, got %d: %s", login.Code, login.Body.String())
	}
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("login response is not JSON: %v", err)
	}
	code := challenge.DevCode
	if code == "" {
		if outboxPath == "" {
			t.Fatalf("login response did not include devCode and no outbox path was provided")
		}
		code = verificationCodeFromOutbox(t, outboxPath)
	}
	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify-login", fmt.Sprintf(`{
		"challengeId": %q,
		"code": %q
	}`, challenge.ChallengeID, code), nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("expected verify status 200, got %d: %s", verified.Code, verified.Body.String())
	}
	return sessionCookie(t, verified)
}

type fakeBenchmarkStore struct {
	snapshot benchmarkSnapshot
}

func (store fakeBenchmarkStore) Snapshot(_ context.Context) benchmarkSnapshot {
	return store.snapshot
}

func TestAdminOperationalPagesUseLiveStoreSnapshots(t *testing.T) {
	handler := newTestServer(Config{
		ProviderName: "sans-primary",
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: []benchmarkRequestDTO{}, Source: "empty", Redis: storageStatusDTO{Status: "connected"},
		}},
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			ProviderHealth: []providerHealthDTO{{
				Provider:     "sans-primary",
				TargetModel:  "nvidia/z-ai/glm-5.2",
				Status:       "Healthy",
				AvgLatencyMs: 3552.54,
				ErrorRate:    0,
				TimeoutRate:  0,
				LastChecked:  "2026-07-16T18:12:00Z",
			}},
			RequestLogs: []requestLogDTO{{
				RequestID: "20260716T111033-mmlu_5:mmlu-0",
				Tenant:    "benchmark",
				Provider:  "sans-primary",
				Model:     "nvidia/z-ai/glm-5.2",
				Method:    "Our Gateway Method",
				LatencyMs: 4761.94,
				TTFTMs:    4761.69,
				Status:    "Success",
				Timestamp: "2026-07-16T18:10:33Z",
			}},
			Source:      "redis",
			GeneratedAt: "2026-07-16T18:12:00Z",
		}},
	})

	provider := authRequest(t, handler, http.MethodGet, "/bff/admin/provider-health", "", adminCookie(t, handler))
	if provider.Code != http.StatusOK || !strings.Contains(provider.Body.String(), "3552.54") || !strings.Contains(provider.Body.String(), `"source":"redis"`) {
		t.Fatalf("expected live provider health snapshot, got %d: %s", provider.Code, provider.Body.String())
	}

	logs := authRequest(t, handler, http.MethodGet, "/bff/admin/request-logs", "", adminCookie(t, handler))
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), "20260716T111033-mmlu_5:mmlu-0") || !strings.Contains(logs.Body.String(), "4761.94") {
		t.Fatalf("expected live request log snapshot, got %d: %s", logs.Code, logs.Body.String())
	}
}

func TestAdminRequestLogsMergeLatestBenchmarkEvidence(t *testing.T) {
	handler := newTestServer(Config{
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			RequestLogs: []requestLogDTO{
				{RequestID: "duplicate-request", Tenant: "tenant-a", Provider: "old-provider", Status: "Success", Timestamp: "2026-07-18T10:00:00Z"},
				{RequestID: "operational-only", Tenant: "tenant-b", Provider: "provider-b", Status: "Success", Timestamp: "2026-07-18T09:00:00Z"},
			},
			Source: "redis", GeneratedAt: "2026-07-18T10:01:00Z", Redis: storageStatusDTO{Status: "connected", Detail: "loaded operational logs"},
		}},
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: []benchmarkRequestDTO{
				{RunID: "step9-mmlu", RequestID: "duplicate-request", Dataset: "mmlu_20", Method: "Our Gateway Method", Provider: "openai-primary", Model: "nvidia/z-ai/glm-5.2", StartedAt: "2026-07-18T11:00:00Z", LatencyMs: 100, TTFTMs: benchmarkFloat(80), InputTokens: 10, OutputTokens: 5, Status: "success", HTTPStatus: 200},
				{RunID: "step9-lmsys", RequestID: "benchmark-latest", Dataset: "lmsys_20", Method: "Our Gateway Method", Provider: "openai-primary", Model: "nvidia/z-ai/glm-5.2", StartedAt: "2026-07-18T12:00:00Z", LatencyMs: 55, Status: "error", HTTPStatus: 502, ErrorType: "provider_rate_limit"},
			},
			Source: "redis", GeneratedAt: "2026-07-18T12:01:00Z", Redis: storageStatusDTO{Status: "connected", Detail: "loaded benchmark requests"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/request-logs", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Logs   []requestLogDTO `json:"logs"`
		Source string          `json:"source"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode request logs: %v", err)
	}
	if len(body.Logs) != 3 {
		t.Fatalf("logs = %+v, want three merged rows", body.Logs)
	}
	if body.Logs[0].RequestID != "benchmark-latest" || body.Logs[1].RequestID != "duplicate-request" || body.Logs[2].RequestID != "operational-only" {
		t.Fatalf("unexpected merge order: %+v", body.Logs)
	}
	if body.Logs[1].Provider != "openai-primary" {
		t.Fatalf("duplicate did not prefer benchmark evidence: %+v", body.Logs[1])
	}
	if body.Logs[0].Tenant != "benchmark/lmsys_20" || body.Logs[0].ErrorMessage != "provider_rate_limit (HTTP 502)" {
		t.Fatalf("benchmark mapping mismatch: %+v", body.Logs[0])
	}
	if body.Source != "operational+benchmark" {
		t.Fatalf("source = %q, want operational+benchmark", body.Source)
	}
}

func TestAdminRequestLogsCapsNewestRowsAndReportsTruncation(t *testing.T) {
	requests := make([]benchmarkRequestDTO, 0, 1002)
	start := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 1002; index++ {
		requests = append(requests, benchmarkRequestDTO{
			RunID: "large-run", RequestID: fmt.Sprintf("request-%04d", index), Dataset: "lmsys_full", Method: "Our Gateway Method",
			StartedAt: start.Add(time.Duration(index) * time.Second).Format(time.RFC3339), Status: "success", HTTPStatus: 200,
		})
	}
	handler := newTestServer(Config{
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{Source: "empty", Redis: storageStatusDTO{Status: "connected"}}},
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: requests, Source: "redis", Redis: storageStatusDTO{Status: "connected"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/request-logs", "", adminCookie(t, handler))
	var body struct {
		Logs         []requestLogDTO `json:"logs"`
		TotalRows    int             `json:"totalRows"`
		ReturnedRows int             `json:"returnedRows"`
		Truncated    bool            `json:"truncated"`
		PartialData  bool            `json:"partialData"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode request logs: %v", err)
	}
	if len(body.Logs) != 1000 || body.TotalRows != 1002 || body.ReturnedRows != 1000 || !body.Truncated || !body.PartialData {
		t.Fatalf("unexpected truncation metadata: logs=%d total=%d returned=%d truncated=%v partial=%v", len(body.Logs), body.TotalRows, body.ReturnedRows, body.Truncated, body.PartialData)
	}
	if body.Logs[0].RequestID != "request-1001" || body.Logs[999].RequestID != "request-0002" {
		t.Fatalf("cap did not preserve newest rows: first=%q last=%q", body.Logs[0].RequestID, body.Logs[999].RequestID)
	}
}

func TestAdminRequestLogsPreserveBenchmarkRowsWhenOperationalStoreUnavailable(t *testing.T) {
	handler := newTestServer(Config{
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			Source: "empty", Redis: storageStatusDTO{Status: "unreachable", Detail: "connection refused"},
		}},
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: []benchmarkRequestDTO{{RequestID: "benchmark-only", Dataset: "mmlu_20", StartedAt: "2026-07-18T12:00:00Z", Status: "success", HTTPStatus: 200}},
			Source:   "redis", Redis: storageStatusDTO{Status: "connected"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/request-logs", "", adminCookie(t, handler))
	var body struct {
		Logs        []requestLogDTO `json:"logs"`
		Source      string          `json:"source"`
		Warnings    []string        `json:"warnings"`
		PartialData bool            `json:"partialData"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode request logs: %v", err)
	}
	if len(body.Logs) != 1 || body.Logs[0].RequestID != "benchmark-only" || body.Source != "benchmark" {
		t.Fatalf("benchmark evidence was not preserved: %+v", body)
	}
	if !body.PartialData || !containsSummaryWarning(body.Warnings, "operational") {
		t.Fatalf("missing partial-source warning: partial=%v warnings=%v", body.PartialData, body.Warnings)
	}
}

func TestAdminOperationalPagesAreEmptyWithoutLiveDataOutsideDemoMode(t *testing.T) {
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		ProviderName:           "sans-primary",
		DemoMode:               false,
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: []benchmarkRequestDTO{}, Source: "empty", Redis: storageStatusDTO{Status: "connected"},
		}},
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			Source: "empty",
		}},
	})

	provider := authRequest(t, handler, http.MethodGet, "/bff/admin/provider-health", "", adminCookie(t, handler))
	logs := authRequest(t, handler, http.MethodGet, "/bff/admin/request-logs", "", adminCookie(t, handler))
	if !strings.Contains(provider.Body.String(), `"providers":[]`) {
		t.Fatalf("expected empty provider health rows, got %s", provider.Body.String())
	}
	if !strings.Contains(logs.Body.String(), `"logs":[]`) {
		t.Fatalf("expected empty request log rows, got %s", logs.Body.String())
	}
}

type fakeOperationalStore struct {
	snapshot operationalSnapshot
}

func (store fakeOperationalStore) Snapshot(_ context.Context) operationalSnapshot {
	return store.snapshot
}

func TestCustomerRegistrationCreatesTenantAndReturnsVerificationChallenge(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "customer-state.json")
	handler := NewServer(Config{
		StatePath:       statePath,
		EmailOutboxPath: filepath.Join(t.TempDir(), "email-outbox.log"),
		TestMode:        true,
	})

	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{
		"email": "alice@example.test",
		"username": "alice_customer",
		"organization": "Alice Research",
		"password": "correct-horse-battery-staple",
		"confirmPassword": "correct-horse-battery-staple",
		"role": "Admin",
		"tenant_id": "other-tenant"
	}`, nil)

	if registered.Code != http.StatusCreated {
		t.Fatalf("expected customer register status 201, got %d: %s", registered.Code, registered.Body.String())
	}
	var response struct {
		Status       string `json:"status"`
		Role         string `json:"role"`
		TenantID     string `json:"tenantId"`
		ChallengeID  string `json:"challengeId"`
		Verification bool   `json:"verificationRequired"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &response); err != nil {
		t.Fatalf("customer register response is not JSON: %v", err)
	}
	if response.Role != "Customer" || response.TenantID == "" || response.TenantID == "other-tenant" {
		t.Fatalf("expected server-assigned Customer tenant identity, got %+v", response)
	}
	if response.ChallengeID == "" || !response.Verification {
		t.Fatalf("expected email verification challenge, got %+v", response)
	}
	if cookieNamed(registered, sessionCookieName) != nil {
		t.Fatalf("registration must not create a session before verification")
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("expected persisted registration state: %v", err)
	}
	stateJSON := string(data)
	for _, want := range []string{"alice_customer", "Alice Research", response.TenantID, `"role": "Customer"`} {
		if !strings.Contains(stateJSON, want) {
			t.Fatalf("persisted state missing %q: %s", want, stateJSON)
		}
	}
}

func TestPublicRegistrationCannotCreateAdmin(t *testing.T) {
	handler := NewServer(Config{})

	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email": "public-admin@example.test",
		"username": "public_admin",
		"password": "correct-horse-battery-staple",
		"role": "Admin"
	}`, nil)

	if registered.Code != http.StatusForbidden {
		t.Fatalf("expected public Admin registration status 403, got %d: %s", registered.Code, registered.Body.String())
	}
}

func TestVerifiedCustomerSessionBindsTenantRoleAndExpiresOnLogout(t *testing.T) {
	handler := NewServer(Config{EmailOutboxPath: filepath.Join(t.TempDir(), "email-outbox.log"), TestMode: true})
	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", `{
		"email": "session-customer@example.test",
		"username": "session_customer",
		"organization": "Session Customer",
		"password": "correct-horse-battery-staple",
		"confirmPassword": "correct-horse-battery-staple"
	}`, nil)
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
		TenantID    string `json:"tenantId"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("register response is not JSON: %v", err)
	}

	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{
		"challengeId": %q,
		"code": %q
	}`, challenge.ChallengeID, challenge.DevCode), nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("expected verification status 200, got %d: %s", verified.Code, verified.Body.String())
	}
	cookie := sessionCookie(t, verified)
	var identity struct {
		UserID   string `json:"userId"`
		TenantID string `json:"tenantId"`
		Role     string `json:"role"`
	}
	if err := json.Unmarshal(verified.Body.Bytes(), &identity); err != nil {
		t.Fatalf("verified response is not JSON: %v", err)
	}
	if identity.UserID == "" || identity.TenantID != challenge.TenantID || identity.Role != "Customer" {
		t.Fatalf("expected bound Customer session identity, got %+v", identity)
	}

	admin := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", cookie)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("expected Customer Admin API status 403, got %d: %s", admin.Code, admin.Body.String())
	}
	session := authRequest(t, handler, http.MethodGet, "/bff/auth/session", "", cookie)
	if session.Code != http.StatusOK || !strings.Contains(session.Body.String(), challenge.TenantID) {
		t.Fatalf("expected authenticated session identity, got %d: %s", session.Code, session.Body.String())
	}

	loggedOut := authRequest(t, handler, http.MethodPost, "/bff/auth/logout", `{}`, cookie)
	if loggedOut.Code != http.StatusOK {
		t.Fatalf("expected logout status 200, got %d", loggedOut.Code)
	}
	afterLogout := authRequest(t, handler, http.MethodGet, "/bff/auth/session", "", cookie)
	if afterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("expected logged-out session status 401, got %d: %s", afterLogout.Code, afterLogout.Body.String())
	}
}

func TestCustomerAPIsEnforceTenantIsolationAndMaskAPIKeys(t *testing.T) {
	store := &mutableOperationalStore{}
	handler := NewServer(Config{
		StatePath:        filepath.Join(t.TempDir(), "customer-api-state.json"),
		EmailOutboxPath:  filepath.Join(t.TempDir(), "email-outbox.log"),
		OperationalStore: store,
		TestMode:         true,
	})
	aliceCookie, aliceTenant := registeredCustomerCookie(t, handler, "alice_api", "Alice API")
	bobCookie, bobTenant := registeredCustomerCookie(t, handler, "bob_api", "Bob API")
	store.snapshot = operationalSnapshot{
		Source:      "redis",
		GeneratedAt: "2026-07-17T10:00:00Z",
		RequestLogs: []requestLogDTO{
			{RequestID: "alice-request", Tenant: aliceTenant, Provider: "provider-a", Model: "model-a", Method: "Our Gateway Method", InputTokens: 10, OutputTokens: 20, Status: "Success", LatencyMs: 100, TTFTMs: 40, Timestamp: "2026-07-17T09:00:00Z"},
			{RequestID: "bob-request", Tenant: bobTenant, Provider: "provider-b", Model: "model-b", Method: "Our Gateway Method", InputTokens: 30, OutputTokens: 40, Status: "Timeout", LatencyMs: 500, TTFTMs: 300, Timestamp: "2026-07-17T09:05:00Z"},
		},
	}

	summary := authRequest(t, handler, http.MethodGet, "/bff/customer/summary?tenant_id="+bobTenant, "", aliceCookie)
	if summary.Code != http.StatusOK || !strings.Contains(summary.Body.String(), `"requests":1`) || !strings.Contains(summary.Body.String(), aliceTenant) {
		t.Fatalf("expected Alice-only summary, got %d: %s", summary.Code, summary.Body.String())
	}
	requests := authRequest(t, handler, http.MethodGet, "/bff/customer/requests?tenant_id="+bobTenant, "", aliceCookie)
	if requests.Code != http.StatusOK || !strings.Contains(requests.Body.String(), "alice-request") || strings.Contains(requests.Body.String(), "bob-request") {
		t.Fatalf("expected Alice-only requests, got %d: %s", requests.Code, requests.Body.String())
	}

	created := authRequest(t, handler, http.MethodPost, "/bff/customer/api-keys", `{"scope":"gateway:invoke","tenant_id":"`+bobTenant+`"}`, aliceCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected API key create status 201, got %d: %s", created.Code, created.Body.String())
	}
	var key struct {
		ID        string `json:"id"`
		Key       string `json:"key"`
		MaskedKey string `json:"maskedKey"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &key); err != nil {
		t.Fatalf("API key response is not JSON: %v", err)
	}
	if key.ID == "" || key.Key == "" || key.MaskedKey == "" || key.Key == key.MaskedKey {
		t.Fatalf("expected one-time secret plus masked key, got %+v", key)
	}
	listed := authRequest(t, handler, http.MethodGet, "/bff/customer/api-keys", "", aliceCookie)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), key.MaskedKey) || strings.Contains(listed.Body.String(), key.Key) {
		t.Fatalf("expected masked API key list, got %d: %s", listed.Code, listed.Body.String())
	}
	bobDelete := authRequest(t, handler, http.MethodDelete, "/bff/customer/api-keys/"+key.ID, "", bobCookie)
	if bobDelete.Code != http.StatusNotFound {
		t.Fatalf("expected cross-tenant API key delete status 404, got %d: %s", bobDelete.Code, bobDelete.Body.String())
	}
	aliceDelete := authRequest(t, handler, http.MethodDelete, "/bff/customer/api-keys/"+key.ID, "", aliceCookie)
	if aliceDelete.Code != http.StatusOK {
		t.Fatalf("expected owner API key delete status 200, got %d: %s", aliceDelete.Code, aliceDelete.Body.String())
	}

	unauthenticated := authRequest(t, handler, http.MethodGet, "/bff/customer/summary", "", nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated Customer API status 401, got %d", unauthenticated.Code)
	}
}

func TestCustomerTenantAndAPIKeyPersistAcrossRestartWhileSessionRequiresLogin(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "customer-restart-state.json")
	outboxPath := filepath.Join(t.TempDir(), "customer-restart-outbox.log")
	handler := NewServer(Config{StatePath: statePath, EmailOutboxPath: outboxPath, TestMode: true})
	cookie, tenantID := registeredCustomerCookie(t, handler, "restart_customer", "Restart Research")
	created := authRequest(t, handler, http.MethodPost, "/bff/customer/api-keys", `{"scope":"gateway:invoke"}`, cookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected API key create status 201, got %d: %s", created.Code, created.Body.String())
	}
	var key struct {
		ID        string `json:"id"`
		MaskedKey string `json:"maskedKey"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}

	restartedOutbox := filepath.Join(t.TempDir(), "customer-restarted-outbox.log")
	restarted := NewServer(Config{StatePath: statePath, EmailOutboxPath: restartedOutbox, TestMode: true})
	oldSession := authRequest(t, restarted, http.MethodGet, "/bff/session", "", cookie)
	if oldSession.Code != http.StatusUnauthorized {
		t.Fatalf("expected pre-restart session to require login, got %d", oldSession.Code)
	}
	login := authRequest(t, restarted, http.MethodPost, "/bff/auth/login", `{"identifier":"restart_customer","password":"correct-horse-battery-staple"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("expected persisted Customer login status 200, got %d: %s", login.Code, login.Body.String())
	}
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	code := challenge.DevCode
	if code == "" {
		code = verificationCodeFromOutbox(t, restartedOutbox)
	}
	verified := authRequest(t, restarted, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{"challengeId":%q,"code":%q}`, challenge.ChallengeID, code), nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("expected persisted Customer verification status 200, got %d: %s", verified.Code, verified.Body.String())
	}
	newCookie := sessionCookie(t, verified)
	session := authRequest(t, restarted, http.MethodGet, "/bff/session", "", newCookie)
	if session.Code != http.StatusOK || !strings.Contains(session.Body.String(), tenantID) {
		t.Fatalf("expected persisted Tenant after login, got %d: %s", session.Code, session.Body.String())
	}
	keys := authRequest(t, restarted, http.MethodGet, "/bff/customer/api-keys", "", newCookie)
	if keys.Code != http.StatusOK || !strings.Contains(keys.Body.String(), key.ID) || !strings.Contains(keys.Body.String(), key.MaskedKey) {
		t.Fatalf("expected persisted masked API key after restart, got %d: %s", keys.Code, keys.Body.String())
	}
}

func TestCustomerPasswordUsesAdaptiveHash(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "password-state.json")
	handler := NewServer(Config{StatePath: statePath, EmailOutboxPath: filepath.Join(t.TempDir(), "email-outbox.log"), TestMode: true})
	registeredCustomerCookie(t, handler, "bcrypt_customer", "Bcrypt Customer")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read customer state: %v", err)
	}
	if !strings.Contains(string(data), `"passwordHash": "$2`) {
		t.Fatalf("expected adaptive bcrypt password hash, got %s", string(data))
	}
	if strings.Contains(string(data), "correct-horse-battery-staple") {
		t.Fatalf("state must not contain the plaintext password")
	}
}

func TestBootstrapAdminWorksWithoutPublicAdminRegistration(t *testing.T) {
	handler := NewServer(Config{
		BootstrapAdminEmail:    "bootstrap-admin@example.test",
		BootstrapAdminUsername: "bootstrap_admin",
		BootstrapAdminPassword: "bootstrap-password-1234",
		EmailOutboxPath:        filepath.Join(t.TempDir(), "email-outbox.log"),
		TestMode:               true,
	})
	public := authRequest(t, handler, http.MethodPost, "/bff/auth/register", `{
		"email":"other-admin@example.test","username":"other_admin","password":"password-1234","role":"Admin"
	}`, nil)
	if public.Code != http.StatusForbidden {
		t.Fatalf("expected public Admin registration status 403, got %d", public.Code)
	}
	login := authRequest(t, handler, http.MethodPost, "/bff/auth/login", `{
		"identifier":"bootstrap_admin","password":"bootstrap-password-1234"
	}`, nil)
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"verificationRequired":true`) {
		t.Fatalf("expected bootstrap Admin login challenge, got %d: %s", login.Code, login.Body.String())
	}
}

func registeredCustomerCookie(t *testing.T, handler http.Handler, username string, organization string) (*http.Cookie, string) {
	t.Helper()
	registered := authRequest(t, handler, http.MethodPost, "/bff/auth/customer/register", fmt.Sprintf(`{
		"email": %q,
		"username": %q,
		"organization": %q,
		"password": "correct-horse-battery-staple",
		"confirmPassword": "correct-horse-battery-staple"
	}`, username+"@example.test", username, organization), nil)
	if registered.Code != http.StatusCreated {
		t.Fatalf("expected customer registration status 201, got %d: %s", registered.Code, registered.Body.String())
	}
	var challenge struct {
		ChallengeID string `json:"challengeId"`
		DevCode     string `json:"devCode"`
		TenantID    string `json:"tenantId"`
	}
	if err := json.Unmarshal(registered.Body.Bytes(), &challenge); err != nil {
		t.Fatalf("register response is not JSON: %v", err)
	}
	verified := authRequest(t, handler, http.MethodPost, "/bff/auth/verify", fmt.Sprintf(`{
		"challengeId": %q,
		"code": %q
	}`, challenge.ChallengeID, challenge.DevCode), nil)
	if verified.Code != http.StatusOK {
		t.Fatalf("expected customer verification status 200, got %d: %s", verified.Code, verified.Body.String())
	}
	return sessionCookie(t, verified), challenge.TenantID
}

type mutableOperationalStore struct {
	snapshot operationalSnapshot
}

func (store *mutableOperationalStore) Snapshot(_ context.Context) operationalSnapshot {
	return store.snapshot
}
