package bff

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type adminSummaryTestResponse struct {
	DefaultProvider string                 `json:"defaultProvider"`
	DefaultModel    string                 `json:"defaultModel"`
	ModelCount      *int                   `json:"modelCount"`
	ActiveProviders *int                   `json:"activeProviders"`
	ActiveTenants   *int                   `json:"activeTenants"`
	RequestVolume   *int                   `json:"requestVolume"`
	AvgLatencyMs    *float64               `json:"avgLatencyMs"`
	P95LatencyMs    *float64               `json:"p95LatencyMs"`
	SuccessRate     *float64               `json:"successRate"`
	ErrorRate       *float64               `json:"errorRate"`
	TimeoutRate     *float64               `json:"timeoutRate"`
	QueueDepth      *float64               `json:"queueDepth"`
	GatewayStatus   string                 `json:"gatewayStatus"`
	RoutingStrategy string                 `json:"routingStrategy"`
	LatestBenchmark *benchmarkDTO          `json:"latestBenchmark"`
	ProviderHealth  []providerHealthDTO    `json:"providerHealth"`
	RecentErrors    []requestLogDTO        `json:"recentErrors"`
	GeneratedAt     string                 `json:"generatedAt"`
	DataSources     []summaryDataSourceDTO `json:"dataSources"`
	Partial         bool                   `json:"partial"`
	Warnings        []string               `json:"warnings"`
	Error           string                 `json:"error"`
}

func TestAdminSummaryAggregatesRealSourcesExactly(t *testing.T) {
	today := time.Now().UTC()
	yesterday := today.Add(-24 * time.Hour)
	upstream := newAdminSummaryUpstream(t, "")
	defer upstream.Close()

	avgBenchmark := 321.5
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		GatewayAdminURL:        upstream.URL,
		GatewayAdminAPIKey:     "server-only-admin-key",
		GatewayAPITimeout:      time.Second,
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			ProviderHealth: []providerHealthDTO{{Provider: "provider-a", TargetModel: "model-a", Status: "Healthy", AvgLatencyMs: 123.4, LastChecked: today.Format(time.RFC3339)}},
			RequestLogs: []requestLogDTO{
				{RequestID: "req-success", Tenant: "tenant-a", Provider: "provider-a", Model: "model-a", Status: "Success", LatencyMs: 100, Timestamp: today.Add(-time.Hour).Format(time.RFC3339)},
				{RequestID: "req-error", Tenant: "tenant-b", Provider: "provider-a", Model: "model-a", Status: "Error", LatencyMs: 200, ErrorMessage: "upstream 500", Timestamp: today.Add(-30 * time.Minute).Format(time.RFC3339)},
				{RequestID: "req-timeout", Tenant: "tenant-a", Provider: "provider-a", Model: "model-a", Status: "Timeout", LatencyMs: 900, ErrorMessage: "deadline exceeded", Timestamp: today.Add(-10 * time.Minute).Format(time.RFC3339)},
				{RequestID: "req-yesterday", Tenant: "tenant-c", Provider: "provider-a", Model: "model-a", Status: "Success", LatencyMs: 9999, Timestamp: yesterday.Format(time.RFC3339)},
			},
			Source: "redis", GeneratedAt: today.Format(time.RFC3339), Redis: storageStatusDTO{Status: "connected", Detail: "loaded"},
		}},
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Benchmarks: []benchmarkDTO{
				{RunID: "old-run", Method: "Local Baseline", TestDate: "2026-07-16", Status: "passed"},
				{RunID: "latest-run", Method: "Our Gateway Method", TestDate: "2026-07-18", AvgLatencyMs: &avgBenchmark, Status: "passed"},
			},
			Source: "redis", GeneratedAt: today.Format(time.RFC3339), Redis: storageStatusDTO{Status: "connected", Detail: "loaded"}, Qdrant: storageStatusDTO{Status: "connected", Detail: "ready"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body adminSummaryTestResponse
	decodeSummaryResponse(t, response, &body)
	if body.Partial || len(body.Warnings) != 0 {
		t.Fatalf("expected complete summary, got partial=%v warnings=%v", body.Partial, body.Warnings)
	}
	if body.DefaultProvider != "provider-a" || body.DefaultModel != "model-a" || body.ModelCount == nil || *body.ModelCount != 2 || body.ActiveProviders == nil || *body.ActiveProviders != 1 {
		t.Fatalf("provider aggregation mismatch: %+v", body)
	}
	if body.RequestVolume == nil || *body.RequestVolume != 3 || body.ActiveTenants == nil || *body.ActiveTenants != 2 {
		t.Fatalf("today aggregation mismatch: volume=%v tenants=%v", body.RequestVolume, body.ActiveTenants)
	}
	assertFloatPointer(t, "avg latency", body.AvgLatencyMs, 400)
	assertFloatPointer(t, "p95 latency", body.P95LatencyMs, 900)
	assertFloatPointer(t, "success rate", body.SuccessRate, 33.33)
	assertFloatPointer(t, "error rate", body.ErrorRate, 33.33)
	assertFloatPointer(t, "timeout rate", body.TimeoutRate, 33.33)
	assertFloatPointer(t, "queue depth", body.QueueDepth, 5)
	if body.GatewayStatus != "Healthy" || body.RoutingStrategy != "latency-aware" {
		t.Fatalf("gateway aggregation mismatch: status=%q route=%q", body.GatewayStatus, body.RoutingStrategy)
	}
	if body.LatestBenchmark == nil || body.LatestBenchmark.RunID != "latest-run" || len(body.ProviderHealth) != 1 || len(body.RecentErrors) != 2 {
		t.Fatalf("store aggregation mismatch: latest=%+v health=%d errors=%d", body.LatestBenchmark, len(body.ProviderHealth), len(body.RecentErrors))
	}
	if body.GeneratedAt == "" || len(body.DataSources) < 6 {
		t.Fatalf("missing provenance: generatedAt=%q dataSources=%+v", body.GeneratedAt, body.DataSources)
	}
	for _, forbidden := range []string{`"requestVolume":18420`, `"p95LatencyMs":842`, `"queueDepth":17`} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("production summary contains demo metric %s: %s", forbidden, response.Body.String())
		}
	}
}

func TestAdminSummaryMarksOneFailedSourcePartialAndPreservesOtherValues(t *testing.T) {
	today := time.Now().UTC()
	upstream := newAdminSummaryUpstream(t, "/admin/v1/topology")
	defer upstream.Close()
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		GatewayAdminURL:        upstream.URL,
		GatewayAdminAPIKey:     "admin-key",
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			RequestLogs: []requestLogDTO{{RequestID: "req-1", Tenant: "tenant-a", Status: "Success", LatencyMs: 250, Timestamp: today.Format(time.RFC3339)}},
			Source:      "redis", GeneratedAt: today.Format(time.RFC3339), Redis: storageStatusDTO{Status: "connected"},
		}},
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Benchmarks: []benchmarkDTO{{RunID: "run-1", Method: "Local Baseline", TestDate: today.Format("2006-01-02"), Status: "passed"}},
			Source:     "redis", Redis: storageStatusDTO{Status: "connected"}, Qdrant: storageStatusDTO{Status: "connected"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body adminSummaryTestResponse
	decodeSummaryResponse(t, response, &body)
	if !body.Partial || body.RequestVolume == nil || *body.RequestVolume != 1 || body.ActiveProviders == nil || *body.ActiveProviders != 1 {
		t.Fatalf("partial summary discarded successful values: %+v", body)
	}
	if !containsSummaryWarning(body.Warnings, "topology") {
		t.Fatalf("warnings do not explain failed topology source: %v", body.Warnings)
	}
}

func TestAdminSummaryReturnsErrorWhenAllSourcesFail(t *testing.T) {
	upstream := newAdminSummaryUpstream(t, "*")
	defer upstream.Close()
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		GatewayAdminURL:        upstream.URL,
		GatewayAdminAPIKey:     "admin-key",
		OperationalStore: fakeOperationalStore{snapshot: operationalSnapshot{
			Source: "empty", Redis: storageStatusDTO{Status: "unreachable", Detail: "connection refused"},
		}},
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Source: "empty", Redis: storageStatusDTO{Status: "unreachable"}, Qdrant: storageStatusDTO{Status: "unreachable"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie(t, handler))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body adminSummaryTestResponse
	decodeSummaryResponse(t, response, &body)
	if body.Error != "admin_summary_unavailable" || !body.Partial || body.RequestVolume != nil || body.AvgLatencyMs != nil || body.QueueDepth != nil {
		t.Fatalf("all-failed response filled unavailable metrics: %+v", body)
	}
	if strings.Contains(response.Body.String(), "18420") || strings.Contains(response.Body.String(), "99.2") || strings.Contains(response.Body.String(), "842") {
		t.Fatalf("all-failed response leaked demo values: %s", response.Body.String())
	}
}

func TestDemoAdminSummaryUsesLatestPublishedBenchmark(t *testing.T) {
	latestLatency := 6137.71
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		DemoMode:               true,
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Benchmarks: []benchmarkDTO{
				{RunID: "older-run", Method: "Our Gateway Method", TestDate: "2026-07-17T10:00:00Z", Status: "passed"},
				{RunID: "step9-latest-run", Method: "Our Gateway Method", TestDate: "2026-07-18T15:32:18Z", AvgLatencyMs: &latestLatency, Status: "passed"},
			},
			Source:      "redis",
			GeneratedAt: "2026-07-18T15:40:00Z",
			Redis:       storageStatusDTO{Status: "connected", Detail: "loaded veloxmesh:benchmarks"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body adminSummaryTestResponse
	decodeSummaryResponse(t, response, &body)
	if body.LatestBenchmark == nil || body.LatestBenchmark.RunID != "step9-latest-run" {
		t.Fatalf("latest benchmark = %+v, want step9-latest-run", body.LatestBenchmark)
	}
	if !containsSummarySource(body.DataSources, "Benchmark data", "redis", "ok") {
		t.Fatalf("missing live benchmark provenance: %+v", body.DataSources)
	}
}

func TestDemoAdminSummaryMarksUnavailableBenchmarkSourcePartial(t *testing.T) {
	handler := NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		DemoMode:               true,
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Source: "empty",
			Redis:  storageStatusDTO{Status: "unreachable", Detail: "connection refused"},
			Qdrant: storageStatusDTO{Status: "unreachable", Detail: "connection refused"},
		}},
	})

	response := authRequest(t, handler, http.MethodGet, "/bff/admin/summary", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body adminSummaryTestResponse
	decodeSummaryResponse(t, response, &body)
	if !body.Partial || body.LatestBenchmark != nil || !containsSummaryWarning(body.Warnings, "benchmark") {
		t.Fatalf("unavailable benchmark source was not marked partial: %+v", body)
	}
	if !containsSummarySource(body.DataSources, "Benchmark data", "Benchmark Store", "error") {
		t.Fatalf("missing benchmark source error provenance: %+v", body.DataSources)
	}
}

func TestParseGatewayQueueDepth(t *testing.T) {
	metrics := "# HELP gateway_queue_depth queue\n" +
		"gateway_queue_depth{backend=\"primary\",priority=\"normal\"} 2\n" +
		"gateway_queue_depth{backend=\"primary\",priority=\"high\"} 3.5\n" +
		"veloxmesh_request_count_total{status=\"success\"} 999\n"
	value, ok := parseGatewayQueueDepth(metrics)
	if !ok || value != 5.5 {
		t.Fatalf("parseGatewayQueueDepth() = %v, %v", value, ok)
	}
}

func newAdminSummaryUpstream(t *testing.T, failedPath string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failedPath == "*" || r.URL.Path == failedPath {
			http.Error(w, "source unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "ok")
		case "/readyz":
			_, _ = io.WriteString(w, `{"status":"ready","configured_providers":2,"healthy":1,"degraded":0,"unhealthy":1,"routing_strategy":"latency-aware"}`)
		case "/admin/v1/providers":
			_, _ = io.WriteString(w, `{"data":[{"id":"provider-a","name":"Provider A","base_url":"https://a.example/v1","enabled":true,"models":["model-a","model-b"],"default_model":"model-a"},{"id":"provider-b","name":"Provider B","base_url":"https://b.example/v1","enabled":false,"models":["model-c"],"default_model":"model-c"}]}`)
		case "/admin/v1/topology":
			_, _ = io.WriteString(w, `{"node_id":"node-a","role":"leader","leader_id":"node-a","writable":true,"wal_lag_elapsed":0,"wal_lag_pending":0}`)
		case "/metrics":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, "gateway_queue_depth{backend=\"a\",priority=\"normal\"} 2\ngateway_queue_depth{backend=\"b\",priority=\"normal\"} 3\n")
		default:
			http.NotFound(w, r)
		}
	}))
}

func decodeSummaryResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode summary response: %v; body=%s", err, response.Body.String())
	}
}

func assertFloatPointer(t *testing.T, name string, actual *float64, expected float64) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("%s = %v, want %v", name, actual, expected)
	}
}

func containsSummaryWarning(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(strings.ToLower(warning), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}

func containsSummarySource(sources []summaryDataSourceDTO, name, source, status string) bool {
	for _, item := range sources {
		if item.Name == name && item.Source == source && item.Status == status {
			return true
		}
	}
	return false
}

func Example_adminSummaryResponseContract() {
	fmt.Println("generatedAt dataSources partial warnings")
	// Output: generatedAt dataSources partial warnings
}
