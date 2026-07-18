package bff

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeBenchmarkRequestStore struct {
	snapshot benchmarkRequestSnapshot
}

func (store fakeBenchmarkRequestStore) Snapshot(_ context.Context) benchmarkRequestSnapshot {
	return store.snapshot
}

func TestBenchmarkRawCSVHasOneDataRowPerAttempt(t *testing.T) {
	handler := benchmarkExportTestServer(t, benchmarkRequestRows())
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/benchmarks/raw.csv", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	rows, err := csv.NewReader(strings.NewReader(response.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("CSV rows=%d, want header + 3 request rows", len(rows))
	}
	if rows[0][0] != "run_id" || rows[0][1] != "request_id" || rows[0][4] != "method_id" {
		t.Fatalf("unexpected canonical CSV header: %v", rows[0])
	}
	if strings.Contains(response.Body.String(), "provider-secret-value") || strings.Contains(strings.ToLower(response.Body.String()), "authorization") {
		t.Fatalf("raw CSV leaked credential material: %s", response.Body.String())
	}
}

func TestBenchmarkZIPIsCompleteAndSummaryIsRecomputedFromRawRows(t *testing.T) {
	handler := benchmarkExportTestServer(t, benchmarkRequestRows())
	response := authRequest(t, handler, http.MethodGet, "/bff/admin/benchmarks/export.zip", "", adminCookie(t, handler))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	reader, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatalf("open ZIP: %v", err)
	}
	entries := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(handle)
		_ = handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(content)
	}
	for _, required := range []string{
		"report.html", "metadata.json", "summary.csv", "raw_requests.csv", "errors_and_timeouts.csv",
		"charts/latency.svg", "charts/tail-latency.svg", "charts/throughput.svg", "charts/error-timeout-rate.svg",
	} {
		if _, ok := entries[required]; !ok {
			t.Errorf("ZIP missing %s; entries=%v", required, zipEntryNames(reader.File))
		}
	}
	rawRows, err := csv.NewReader(strings.NewReader(entries["raw_requests.csv"])).ReadAll()
	if err != nil || len(rawRows) != 4 {
		t.Fatalf("raw request CSV does not match attempted requests: rows=%d err=%v", len(rawRows), err)
	}
	summaryRows, err := csv.NewReader(strings.NewReader(entries["summary.csv"])).ReadAll()
	if err != nil || len(summaryRows) != 2 {
		t.Fatalf("summary CSV rows=%d err=%v", len(summaryRows), err)
	}
	header := csvRowMap(summaryRows[0], summaryRows[1])
	if header["request_count"] != "3" || header["avg_latency_ms"] != "400" || header["p95_latency_ms"] != "900" {
		t.Fatalf("summary was not recomputed from raw rows: %v", header)
	}
	if header["success_rate_pct"] != "33.33" || header["error_rate_pct"] != "33.33" || header["timeout_rate_pct"] != "33.33" {
		t.Fatalf("outcome rates were not recomputed: %v", header)
	}
	if strings.Contains(entries["summary.csv"], "9999") {
		t.Fatalf("summary trusted the injected aggregate instead of raw rows: %s", entries["summary.csv"])
	}
	if !strings.Contains(entries["report.html"], "Appendix") || !strings.Contains(entries["report.html"], "raw_requests.csv") || strings.Count(entries["report.html"], "req-") > 4 {
		t.Fatalf("HTML appendix is missing references or embeds too many request rows")
	}
	for name, content := range entries {
		if strings.Contains(content, "provider-secret-value") || strings.Contains(strings.ToLower(content), "authorization: bearer") {
			t.Fatalf("ZIP entry %s leaked credential material", name)
		}
	}
}

func TestBenchmarkExportsReturnNotFoundWithoutRawRequests(t *testing.T) {
	handler := benchmarkExportTestServer(t, nil)
	for _, path := range []string{"/bff/admin/benchmarks/raw.csv", "/bff/admin/benchmarks/export.zip"} {
		response := authRequest(t, handler, http.MethodGet, path, "", adminCookie(t, handler))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "benchmark_requests_unavailable") {
			t.Fatalf("GET %s = %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestCustomerCannotDownloadAdminBenchmarkArtifacts(t *testing.T) {
	handler := benchmarkExportTestServer(t, benchmarkRequestRows())
	cookie, _ := registeredCustomerCookie(t, handler, "benchmark_customer", "Benchmark Customer")
	for _, path := range []string{"/bff/admin/benchmarks/raw.csv", "/bff/admin/benchmarks/export.zip"} {
		response := authRequest(t, handler, http.MethodGet, path, "", cookie)
		if response.Code != http.StatusForbidden {
			t.Fatalf("Customer GET %s = %d, want 403", path, response.Code)
		}
	}
}

func benchmarkExportTestServer(t *testing.T, requests []benchmarkRequestDTO) http.Handler {
	t.Helper()
	misleading := 9999.0
	return NewServer(Config{
		AllowAdminRegistration: true,
		TestMode:               true,
		DemoMode:               false,
		BenchmarkStore: fakeBenchmarkStore{snapshot: benchmarkSnapshot{
			Benchmarks: []benchmarkDTO{{
				RunID: "run-1", MethodID: "gateway_improved_model", Method: "Our Gateway + Improved Model", Dataset: "mmlu", RequestCount: 999,
				Provider: "improved-provider", TargetModel: "improved-model", ModelVersion: "v2.1", AvgLatencyMs: &misleading, TestDate: "2026-07-18T10:00:00Z",
			}},
			Source: "redis",
		}},
		BenchmarkRequestStore: fakeBenchmarkRequestStore{snapshot: benchmarkRequestSnapshot{
			Requests: requests, Source: "redis", GeneratedAt: "2026-07-18T10:01:00Z", Redis: storageStatusDTO{Status: "connected"},
		}},
	})
}

func benchmarkRequestRows() []benchmarkRequestDTO {
	return []benchmarkRequestDTO{
		{RunID: "run-1", RequestID: "req-success", Dataset: "mmlu", RowIndex: 0, MethodID: "gateway_improved_model", Method: "Our Gateway + Improved Model", Provider: "improved-provider", Model: "improved-model", ModelVersion: "v2.1", Route: "default-provider", StartedAt: "2026-07-18T10:00:00.000Z", EndedAt: "2026-07-18T10:00:00.100Z", LatencyMs: 100, TTFTMs: benchmarkFloat(40), InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Status: "success", HTTPStatus: 200, ErrorType: "", Timeout: false, RetryCount: 0, CacheHit: false},
		{RunID: "run-1", RequestID: "req-error", Dataset: "mmlu", RowIndex: 1, MethodID: "gateway_improved_model", Method: "Our Gateway + Improved Model", Provider: "improved-provider", Model: "improved-model", ModelVersion: "v2.1", Route: "default-provider", StartedAt: "2026-07-18T10:00:00.200Z", EndedAt: "2026-07-18T10:00:00.400Z", LatencyMs: 200, TTFTMs: nil, Status: "error", HTTPStatus: 502, ErrorType: "provider_error", Timeout: false, RetryCount: 1, CacheHit: false},
		{RunID: "run-1", RequestID: "req-timeout", Dataset: "mmlu", RowIndex: 2, MethodID: "gateway_improved_model", Method: "Our Gateway + Improved Model", Provider: "improved-provider", Model: "improved-model", ModelVersion: "v2.1", Route: "fallback", StartedAt: "2026-07-18T10:00:00.500Z", EndedAt: "2026-07-18T10:00:01.400Z", LatencyMs: 900, TTFTMs: nil, Status: "timeout", HTTPStatus: 504, ErrorType: "timeout", Timeout: true, RetryCount: 2, CacheHit: false},
	}
}

func csvRowMap(header, row []string) map[string]string {
	result := map[string]string{}
	for index, key := range header {
		if index < len(row) {
			result[key] = row[index]
		}
	}
	return result
}

func zipEntryNames(files []*zip.File) []string {
	result := make([]string, 0, len(files))
	for _, file := range files {
		result = append(result, file.Name)
	}
	return result
}
