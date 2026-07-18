package bff

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPGatewayAdminClientListProvidersSendsBearerAndDecodesSnakeCase(t *testing.T) {
	const adminKey = "admin-secret-must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/admin/v1/providers" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+adminKey {
			t.Fatalf("unexpected authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"provider-1","name":"Primary","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","timeout":"30s","weight":10,"revision":4,"secret":{"configured":true},"created_at":"2026-07-17T10:00:00Z","updated_at":"2026-07-17T10:05:00Z"}]}`)
	}))
	defer server.Close()

	client, err := NewHTTPGatewayAdminClient(server.URL, adminKey, time.Second, server.Client())
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}

	providers, err := client.ListProviders(context.Background())
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(providers) != 1 || providers[0].BaseURL != "https://provider.example/v1" {
		t.Fatalf("unexpected providers: %#v", providers)
	}
	if providers[0].DefaultModel != "model-a" || !providers[0].Secret.Configured || providers[0].Revision != 4 {
		t.Fatalf("snake_case provider fields were not decoded: %#v", providers[0])
	}
	if formatted := fmt.Sprintf("%#v", client); strings.Contains(formatted, adminKey) {
		t.Fatalf("formatted client leaked admin key: %s", formatted)
	}
}

func TestHTTPGatewayAdminClientPutRoutingSendsJSONAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/admin/v1/routing" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type %q", got)
		}
		var body GatewayRoutingUpdateRequest
		decodeTestJSON(t, r, &body)
		if body.DefaultProvider != "provider-2" || body.MaxAttempts != 3 || body.Revision != 7 {
			t.Fatalf("unexpected routing request: %#v", body)
		}
		fmt.Fprint(w, `{"id":"global","strategy":"latency-aware","default_provider":"provider-2","fallback_enabled":true,"max_attempts":3,"revision":8,"created_at":"2026-07-17T10:00:00Z","updated_at":"2026-07-17T11:00:00Z"}`)
	}))
	defer server.Close()

	client := newTestGatewayAdminClient(t, server, "admin-key", time.Second)
	got, err := client.PutRouting(context.Background(), GatewayRoutingUpdateRequest{
		Strategy:        "latency-aware",
		DefaultProvider: "provider-2",
		FallbackEnabled: true,
		MaxAttempts:     3,
		Revision:        7,
	})
	if err != nil {
		t.Fatalf("put routing: %v", err)
	}
	if got.DefaultProvider != "provider-2" || got.Revision != 8 {
		t.Fatalf("unexpected routing response: %#v", got)
	}
}

func TestHTTPGatewayAdminClientVerifiesModelsAndChatWithDataPlaneKey(t *testing.T) {
	const adminKey = "admin-secret"
	const dataKey = "data-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/v1/providers/provider-2":
			if r.Header.Get("Authorization") != "Bearer "+adminKey {
				t.Fatalf("provider readback used wrong credential: %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Admin-Actor") != "admin@example.com" || r.Header.Get("X-Request-ID") != "verify-123" {
				t.Fatalf("admin evidence headers actor=%q request=%q", r.Header.Get("X-Admin-Actor"), r.Header.Get("X-Request-ID"))
			}
			fmt.Fprint(w, `{"id":"provider-2","name":"Provider 2","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"models":["model-a"],"default_model":"model-a","revision":8,"secret":{"configured":true}}`)
		case "GET /v1/models":
			if r.Header.Get("Authorization") != "Bearer "+dataKey {
				t.Fatalf("models verification used wrong credential: %q", r.Header.Get("Authorization"))
			}
			fmt.Fprint(w, `{"data":[{"id":"model-a"}]}`)
		case "POST /v1/chat/completions":
			if r.Header.Get("Authorization") != "Bearer "+dataKey {
				t.Fatalf("chat verification used wrong credential: %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Request-ID") != "verify-123" {
				t.Fatalf("verification request id = %q", r.Header.Get("X-Request-ID"))
			}
			w.Header().Set("X-Provider", "provider-2")
			w.Header().Set("X-Routing-Strategy", "default-provider")
			w.Header().Set("X-Request-ID", "verify-123")
			fmt.Fprint(w, `{"id":"verify-123","choices":[{"message":{"role":"assistant","content":"OK"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPGatewayAdminClientWithCredentials(server.URL, server.URL, server.URL, adminKey, dataKey, time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithGatewayOperation(context.Background(), "admin@example.com", "verify-123")
	provider, err := client.GetProvider(ctx, "provider-2")
	if err != nil || provider.Revision != 8 {
		t.Fatalf("GetProvider() = %+v, %v", provider, err)
	}
	models, err := client.VerifyModels(ctx, "model-a")
	if err != nil || !models.Verified {
		t.Fatalf("VerifyModels() = %+v, %v", models, err)
	}
	chat, err := client.VerifyChat(ctx, "model-a")
	if err != nil || !chat.Verified || chat.ProviderID != "provider-2" || chat.Route != "default-provider" || chat.RequestID != "verify-123" {
		t.Fatalf("VerifyChat() = %+v, %v", chat, err)
	}
	formatted := fmt.Sprintf("%#v", client)
	if strings.Contains(formatted, adminKey) || strings.Contains(formatted, dataKey) {
		t.Fatalf("formatted client leaked credentials: %s", formatted)
	}
}

func TestHTTPGatewayAdminClientSupportsSystemManagementOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /admin/v1/routing":
			fmt.Fprint(w, `{"id":"global","strategy":"round-robin","default_provider":"provider-1","fallback_enabled":true,"max_attempts":2,"revision":1}`)
		case "GET /admin/v1/tenants":
			fmt.Fprint(w, `{"data":[{"id":"tenant-a","name":"Alice Lab","owner":"Alice","daily_quota":5000,"status":"active","revision":2}]}`)
		case "POST /admin/v1/tenants":
			var body GatewayTenantCreateRequest
			decodeTestJSON(t, r, &body)
			if body.ID != "tenant-b" || body.Name != "Bob Lab" || body.DailyQuota != 9000 {
				t.Fatalf("unexpected tenant create: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"tenant-b","name":"Bob Lab","owner":"Bob","daily_quota":9000,"status":"active","revision":1}`)
		case "PUT /admin/v1/tenants/tenant-b":
			var body GatewayTenantUpdateRequest
			decodeTestJSON(t, r, &body)
			if body.Name != "Bob Lab" || body.Owner != "Bob Updated" || body.Revision != 1 {
				t.Fatalf("unexpected tenant update: %#v", body)
			}
			fmt.Fprint(w, `{"id":"tenant-b","name":"Bob Lab","owner":"Bob Updated","daily_quota":9000,"status":"active","revision":2}`)
		case "DELETE /admin/v1/tenants/tenant-b":
			w.WriteHeader(http.StatusNoContent)
		case "GET /admin/v1/api-keys":
			fmt.Fprint(w, `{"data":[{"id":"key-1","prefix":"vx_live_abcd","name":"Benchmark","tenant_id":"tenant-a","role":"gateway:invoke","enabled":true,"credit_balance":1000}]}`)
		case "POST /admin/v1/api-keys":
			var body GatewayAPIKeyCreateRequest
			decodeTestJSON(t, r, &body)
			if body.TenantID != "tenant-a" || body.Role != "gateway:invoke" {
				t.Fatalf("unexpected API key create: %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"record":{"id":"key-2","prefix":"vx_live_efgh","name":"Load test","tenant_id":"tenant-a","role":"gateway:invoke","enabled":true,"credit_balance":2000},"secret":"vx_live_one_time_secret"}`)
		case "DELETE /admin/v1/api-keys/key-2":
			w.WriteHeader(http.StatusNoContent)
		case "GET /admin/v1/audit":
			if r.URL.Query().Get("actor") != "admin" || r.URL.Query().Get("limit") != "25" {
				t.Fatalf("unexpected audit query: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":[{"id":"audit-1","actor":"admin","action":"tenant.update","target_id":"tenant-b","outcome":"success","metadata":{"field":"status"},"timestamp":"2026-07-17T12:00:00Z"}]}`)
		case "GET /admin/v1/usage":
			if r.URL.Query().Get("tenant_id") != "tenant-a" || r.URL.Query().Get("provider_id") != "provider-1" {
				t.Fatalf("unexpected usage query: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"data":[{"id":"usage-1","tenant_id":"tenant-a","provider_id":"provider-1","model":"model-a","prompt_tokens":10,"response_tokens":20,"total_tokens":30,"duration_ms":250,"timestamp":"2026-07-17T12:01:00Z","status":"settled"}]}`)
		case "GET /admin/v1/settings":
			fmt.Fprint(w, `{"id":"global","default_provider":"provider-1","default_model":"model-a","request_timeout_seconds":30,"data_retention_days":90,"revision":3}`)
		case "PUT /admin/v1/settings":
			var body GatewaySettingsUpdateRequest
			decodeTestJSON(t, r, &body)
			if body.DataRetentionDays != 60 || body.Revision != 3 {
				t.Fatalf("unexpected settings update: %#v", body)
			}
			fmt.Fprint(w, `{"id":"global","default_provider":"provider-1","default_model":"model-a","request_timeout_seconds":45,"data_retention_days":60,"revision":4}`)
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestGatewayAdminClient(t, server, "admin-key", time.Second)
	ctx := context.Background()

	if routing, err := client.GetRouting(ctx); err != nil || routing.Strategy != "round-robin" {
		t.Fatalf("get routing: %#v, %v", routing, err)
	}
	if tenants, err := client.ListTenants(ctx); err != nil || len(tenants) != 1 || tenants[0].DailyQuota != 5000 {
		t.Fatalf("list tenants: %#v, %v", tenants, err)
	}
	createdTenant, err := client.CreateTenant(ctx, GatewayTenantCreateRequest{ID: "tenant-b", Name: "Bob Lab", Owner: "Bob", DailyQuota: 9000, Status: "active"})
	if err != nil || createdTenant.ID != "tenant-b" {
		t.Fatalf("create tenant: %#v, %v", createdTenant, err)
	}
	updatedTenant, err := client.UpdateTenant(ctx, "tenant-b", GatewayTenantUpdateRequest{Name: "Bob Lab", Owner: "Bob Updated", DailyQuota: 9000, Status: "active", Revision: 1})
	if err != nil || updatedTenant.Revision != 2 {
		t.Fatalf("update tenant: %#v, %v", updatedTenant, err)
	}
	if err := client.DeleteTenant(ctx, "tenant-b"); err != nil {
		t.Fatalf("delete tenant: %v", err)
	}

	keys, err := client.ListAPIKeys(ctx)
	if err != nil || len(keys) != 1 || keys[0].TenantID != "tenant-a" {
		t.Fatalf("list API keys: %#v, %v", keys, err)
	}
	createdKey, err := client.CreateAPIKey(ctx, GatewayAPIKeyCreateRequest{Name: "Load test", TenantID: "tenant-a", Role: "gateway:invoke", CreditBalance: 2000})
	if err != nil || createdKey.Record.ID != "key-2" || createdKey.Secret != "vx_live_one_time_secret" {
		t.Fatalf("create API key: %#v, %v", createdKey, err)
	}
	if err := client.RevokeAPIKey(ctx, "key-2"); err != nil {
		t.Fatalf("revoke API key: %v", err)
	}

	audit, err := client.ListAudit(ctx, GatewayAuditFilter{Actor: "admin", Limit: 25})
	if err != nil || len(audit) != 1 || audit[0].TargetID != "tenant-b" {
		t.Fatalf("list audit: %#v, %v", audit, err)
	}
	usage, err := client.ListUsage(ctx, GatewayUsageFilter{TenantID: "tenant-a", ProviderID: "provider-1", Limit: 50})
	if err != nil || len(usage) != 1 || usage[0].TotalTokens != 30 {
		t.Fatalf("list usage: %#v, %v", usage, err)
	}
	settings, err := client.GetSettings(ctx)
	if err != nil || settings.Revision != 3 {
		t.Fatalf("get settings: %#v, %v", settings, err)
	}
	updatedSettings, err := client.PutSettings(ctx, GatewaySettingsUpdateRequest{DefaultProvider: "provider-1", DefaultModel: "model-a", RequestTimeoutSeconds: 45, DataRetentionDays: 60, Revision: 3})
	if err != nil || updatedSettings.Revision != 4 {
		t.Fatalf("put settings: %#v, %v", updatedSettings, err)
	}
}

func TestHTTPGatewayAdminClientCreatesUpdatesAndDeletesProvider(t *testing.T) {
	requests := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var request GatewayProviderMutation
		decodeTestJSON(t, r, &request)
		if request.APIKey == nil || *request.APIKey != "provider-secret" {
			t.Fatalf("provider mutation did not carry the requested provider credential")
		}
		fmt.Fprintf(w, `{"id":"provider-a","name":"Provider A","type":"openai-compatible","base_url":"https://provider.example/v1","enabled":true,"revision":%d}`, request.Revision+1)
	}))
	defer server.Close()

	client := newTestGatewayAdminClient(t, server, "admin-key", time.Second)
	secret := "provider-secret"
	created, err := client.CreateProvider(context.Background(), GatewayProviderMutation{ID: "provider-a", Name: "Provider A", Type: "openai-compatible", BaseURL: "https://provider.example/v1", Enabled: true, APIKey: &secret})
	if err != nil || created.ID != "provider-a" {
		t.Fatalf("CreateProvider() = %+v, %v", created, err)
	}
	_, err = client.UpdateProvider(context.Background(), "provider-a", GatewayProviderMutation{Name: "Provider A", Type: "openai-compatible", BaseURL: "https://provider.example/v1", Enabled: true, APIKey: &secret, Revision: 1})
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if err := client.DeleteProvider(context.Background(), "provider-a"); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}

	want := []string{"POST /admin/v1/providers", "PUT /admin/v1/providers/provider-a", "DELETE /admin/v1/providers/provider-a"}
	if fmt.Sprint(requests) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestHTTPGatewayAdminClientReusesHealthReadinessTopologyAndMetrics(t *testing.T) {
	var topologyAuth, healthAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			healthAuth = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, "ok")
		case "/readyz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ready","configured_providers":2,"healthy":2,"routing_strategy":"latency"}`)
		case "/admin/v1/topology":
			topologyAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"node_id":"node-a","role":"leader","writable":true}`)
		case "/metrics":
			_, _ = io.WriteString(w, "veloxmesh_requests_total 42\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewHTTPGatewayAdminClientWithEndpoints(server.URL, server.URL, server.URL, "admin-key", time.Second, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	health, err := client.GetHealth(context.Background())
	if err != nil || health.Status != "ok" {
		t.Fatalf("GetHealth() = %+v, %v", health, err)
	}
	ready, err := client.GetReadiness(context.Background())
	if err != nil || ready.Status != "ready" || ready.Healthy != 2 {
		t.Fatalf("GetReadiness() = %+v, %v", ready, err)
	}
	topology, err := client.GetTopology(context.Background())
	if err != nil || !topology.Writable {
		t.Fatalf("GetTopology() = %+v, %v", topology, err)
	}
	metrics, err := client.GetMetrics(context.Background())
	if err != nil || !strings.Contains(metrics, "veloxmesh_requests_total 42") {
		t.Fatalf("GetMetrics() = %q, %v", metrics, err)
	}
	if healthAuth != "" {
		t.Fatalf("public health probe received admin auth %q", healthAuth)
	}
	if topologyAuth != "Bearer admin-key" {
		t.Fatalf("topology auth = %q", topologyAuth)
	}
}

func TestHTTPGatewayAdminClientMapsTimeoutToSentinel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer server.Close()

	client := newTestGatewayAdminClient(t, server, "admin-key", 10*time.Millisecond)
	_, err := client.ListProviders(context.Background())
	if !errors.Is(err, ErrGatewayAdminTimeout) {
		t.Fatalf("expected timeout sentinel, got %T: %v", err, err)
	}
}

func TestHTTPGatewayAdminClientMapsHTTPStatusWithoutLeakingSecrets(t *testing.T) {
	const adminKey = "admin-key-never-in-errors"
	const upstreamSecret = "upstream-body-secret"

	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprintf(w, `{"error":"%s %s"}`, adminKey, upstreamSecret)
			}))
			defer server.Close()

			client := newTestGatewayAdminClient(t, server, adminKey, time.Second)
			_, err := client.ListProviders(context.Background())
			var httpErr *GatewayAdminHTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != status {
				t.Fatalf("expected typed HTTP error for %d, got %T: %v", status, err, err)
			}
			for _, secret := range []string{adminKey, upstreamSecret} {
				if strings.Contains(fmt.Sprintf("%v %#v", err, err), secret) {
					t.Fatalf("error leaked secret %q: %v", secret, err)
				}
			}
		})
	}
}

func TestHTTPGatewayAdminClientDoesNotForwardAdminKeyAcrossRedirect(t *testing.T) {
	const adminKey = "redirect-sensitive-admin-key"
	receivedAuthorization := make(chan string, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthorization <- r.Header.Get("Authorization")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()

	client := newTestGatewayAdminClient(t, redirector, adminKey, time.Second)
	_, err := client.ListProviders(context.Background())
	var httpErr *GatewayAdminHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect to be returned as typed HTTP error, got %T: %v", err, err)
	}
	select {
	case authorization := <-receivedAuthorization:
		t.Fatalf("redirect target received authorization header %q", authorization)
	default:
	}
}

func TestHTTPGatewayAdminClientCapsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":"`+strings.Repeat("x", gatewayAdminMaxResponseBytes+1)+`"}`)
	}))
	defer server.Close()

	client := newTestGatewayAdminClient(t, server, "admin-key", time.Second)
	_, err := client.ListProviders(context.Background())
	if !errors.Is(err, ErrGatewayAdminResponseTooLarge) {
		t.Fatalf("expected response-too-large sentinel, got %T: %v", err, err)
	}
}

func TestHTTPGatewayAdminClientRejectsMissingRequiredResponseFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/admin/v1/api-keys" {
			_, _ = io.WriteString(w, `{"data":[{"id":"key-a","prefix":"vx_live_abc"}]}`)
			return
		}
		if r.URL.Path == "/admin/v1/providers" {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client := newTestGatewayAdminClient(t, server, "admin-key", time.Second)
	if _, err := client.ListProviders(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("ListProviders missing data error = %v", err)
	}
	if _, err := client.GetRouting(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("GetRouting missing fields error = %v", err)
	}
	if _, err := client.ListAPIKeys(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("ListAPIKeys missing tenant error = %v", err)
	}
	if _, err := client.GetReadiness(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("GetReadiness missing fields error = %v", err)
	}
	if _, err := client.GetTopology(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("GetTopology missing fields error = %v", err)
	}
	if _, err := client.GetHealth(context.Background()); !errors.Is(err, ErrGatewayAdminInvalidResponse) {
		t.Fatalf("GetHealth invalid status error = %v", err)
	}
}

func newTestGatewayAdminClient(t *testing.T, server *httptest.Server, adminKey string, timeout time.Duration) GatewayAdminClient {
	t.Helper()
	client, err := NewHTTPGatewayAdminClient(server.URL, adminKey, timeout, server.Client())
	if err != nil {
		t.Fatalf("construct gateway admin client: %v", err)
	}
	return client
}

func decodeTestJSON(t *testing.T, r *http.Request, destination any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(destination); err != nil {
		t.Fatalf("decode request JSON: %v", err)
	}
}
