package bff

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const benchmarkRequestRedisKey = "veloxmesh:benchmark_requests"

type benchmarkRequestDTO struct {
	RunID        string   `json:"runId"`
	RequestID    string   `json:"requestId"`
	Dataset      string   `json:"dataset"`
	RowIndex     int      `json:"rowIndex"`
	MethodID     string   `json:"methodId"`
	Method       string   `json:"method"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	ModelVersion string   `json:"modelVersion"`
	Route        string   `json:"route"`
	StartedAt    string   `json:"startedAt"`
	EndedAt      string   `json:"endedAt"`
	LatencyMs    float64  `json:"latencyMs"`
	TTFTMs       *float64 `json:"ttftMs"`
	InputTokens  int      `json:"inputTokens"`
	OutputTokens int      `json:"outputTokens"`
	TotalTokens  int      `json:"totalTokens"`
	Status       string   `json:"status"`
	HTTPStatus   int      `json:"httpStatus"`
	ErrorType    string   `json:"errorType"`
	Timeout      bool     `json:"timeout"`
	RetryCount   int      `json:"retryCount"`
	CacheHit     bool     `json:"cacheHit"`
}

type benchmarkRequestSnapshot struct {
	Requests    []benchmarkRequestDTO
	Source      string
	GeneratedAt string
	Redis       storageStatusDTO
}

type benchmarkRequestStore interface {
	Snapshot(ctx context.Context) benchmarkRequestSnapshot
}

type liveBenchmarkRequestStore struct {
	redisAddr string
}

type benchmarkRecomputedSummary struct {
	RunID          string
	MethodID       string
	Method         string
	Dataset        string
	Provider       string
	Model          string
	ModelVersion   string
	RequestCount   int
	AvgLatencyMs   float64
	P50LatencyMs   float64
	P95LatencyMs   float64
	P99LatencyMs   float64
	TTFTMs         *float64
	ThroughputRPS  float64
	SuccessRatePct float64
	ErrorRatePct   float64
	TimeoutRatePct float64
	StartedAt      string
	EndedAt        string
}

var benchmarkRequestCSVHeader = []string{
	"run_id", "request_id", "dataset", "row_index", "method_id", "method", "provider", "model", "model_version", "route",
	"started_at", "ended_at", "latency_ms", "ttft_ms", "input_tokens", "output_tokens", "total_tokens", "status", "http_status",
	"error_type", "timeout", "retry_count", "cache_hit",
}

var benchmarkSummaryCSVHeader = []string{
	"run_id", "method_id", "method", "dataset", "provider", "model", "model_version", "request_count", "avg_latency_ms",
	"p50_latency_ms", "p95_latency_ms", "p99_latency_ms", "ttft_ms", "throughput_rps", "success_rate_pct", "error_rate_pct",
	"timeout_rate_pct", "started_at", "ended_at",
}

func (store liveBenchmarkRequestStore) Snapshot(ctx context.Context) benchmarkRequestSnapshot {
	var document struct {
		GeneratedAt string                `json:"generatedAt"`
		Requests    []benchmarkRequestDTO `json:"requests"`
	}
	err := redisJSONDocument(ctx, store.redisAddr, benchmarkRequestRedisKey, &document)
	status := storageStatusDTO{Status: "connected", Detail: "loaded " + benchmarkRequestRedisKey}
	source := "redis"
	if err != nil {
		status = storageStatusDTO{Status: "connected", Detail: "no request-level benchmark snapshot"}
		if isRedisConnectionError(err) {
			status = storageStatusDTO{Status: "unreachable", Detail: shortError(err)}
		}
		source = "empty"
	}
	if document.Requests == nil {
		document.Requests = []benchmarkRequestDTO{}
	}
	return benchmarkRequestSnapshot{Requests: document.Requests, Source: source, GeneratedAt: document.GeneratedAt, Redis: status}
}

func (server *Server) handleAdminBenchmarkRawCSV(w http.ResponseWriter, r *http.Request) {
	snapshot := server.benchmarkRequests.Snapshot(r.Context())
	if len(snapshot.Requests) == 0 {
		writeBenchmarkRequestsUnavailable(w, snapshot)
		return
	}
	content, err := writeBenchmarkRequestsCSV(snapshot.Requests)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "benchmark_csv_failed"})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="veloxmesh-benchmark-raw-requests.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (server *Server) handleAdminBenchmarkExportZIP(w http.ResponseWriter, r *http.Request) {
	requestSnapshot := server.benchmarkRequests.Snapshot(r.Context())
	if len(requestSnapshot.Requests) == 0 {
		writeBenchmarkRequestsUnavailable(w, requestSnapshot)
		return
	}
	benchmarkSnapshot := server.benchmarkStore.Snapshot(r.Context())
	content, err := buildBenchmarkExportZIP(requestSnapshot, benchmarkSnapshot)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "benchmark_export_failed"})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="veloxmesh-benchmark-report.zip"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func writeBenchmarkRequestsUnavailable(w http.ResponseWriter, snapshot benchmarkRequestSnapshot) {
	writeJSON(w, http.StatusNotFound, map[string]any{
		"error":       "benchmark_requests_unavailable",
		"message":     "Request-level benchmark data has not been published",
		"source":      firstNonEmpty([]string{snapshot.Source}, "empty"),
		"generatedAt": snapshot.GeneratedAt,
		"storage":     map[string]storageStatusDTO{"redis": snapshot.Redis},
	})
}

func buildBenchmarkExportZIP(requestSnapshot benchmarkRequestSnapshot, benchmarkSnapshot benchmarkSnapshot) ([]byte, error) {
	rawCSV, err := writeBenchmarkRequestsCSV(requestSnapshot.Requests)
	if err != nil {
		return nil, err
	}
	errorsCSV, err := writeBenchmarkRequestsCSV(filterBenchmarkErrors(requestSnapshot.Requests))
	if err != nil {
		return nil, err
	}
	summaries := recomputeBenchmarkSummaries(requestSnapshot.Requests)
	summaryCSV, err := writeBenchmarkSummaryCSV(summaries)
	if err != nil {
		return nil, err
	}
	metadata, err := benchmarkMetadataJSON(requestSnapshot, benchmarkSnapshot, summaries)
	if err != nil {
		return nil, err
	}
	entries := map[string][]byte{
		"report.html":                   []byte(benchmarkReportHTML(requestSnapshot, summaries)),
		"metadata.json":                 metadata,
		"summary.csv":                   summaryCSV,
		"raw_requests.csv":              rawCSV,
		"errors_and_timeouts.csv":       errorsCSV,
		"charts/latency.svg":            []byte(benchmarkBarChartSVG("Average latency", summaries, func(row benchmarkRecomputedSummary) float64 { return row.AvgLatencyMs }, "ms")),
		"charts/tail-latency.svg":       []byte(benchmarkTailLatencySVG(summaries)),
		"charts/throughput.svg":         []byte(benchmarkBarChartSVG("Throughput", summaries, func(row benchmarkRecomputedSummary) float64 { return row.ThroughputRPS }, "req/s")),
		"charts/error-timeout-rate.svg": []byte(benchmarkErrorRateSVG(summaries)),
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		file, err := writer.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(entries[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeBenchmarkRequestsCSV(rows []benchmarkRequestDTO) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(benchmarkRequestCSVHeader); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write(benchmarkRequestCSVRow(row)); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func benchmarkRequestCSVRow(row benchmarkRequestDTO) []string {
	return []string{
		row.RunID, row.RequestID, row.Dataset, strconv.Itoa(row.RowIndex), row.MethodID, row.Method, row.Provider, row.Model, row.ModelVersion, row.Route,
		row.StartedAt, row.EndedAt, formatBenchmarkNumber(row.LatencyMs), formatOptionalBenchmarkNumber(row.TTFTMs), strconv.Itoa(row.InputTokens),
		strconv.Itoa(row.OutputTokens), strconv.Itoa(row.TotalTokens), row.Status, strconv.Itoa(row.HTTPStatus), row.ErrorType,
		strconv.FormatBool(row.Timeout), strconv.Itoa(row.RetryCount), strconv.FormatBool(row.CacheHit),
	}
}

func writeBenchmarkSummaryCSV(rows []benchmarkRecomputedSummary) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(benchmarkSummaryCSVHeader); err != nil {
		return nil, err
	}
	for _, row := range rows {
		values := []string{
			row.RunID, row.MethodID, row.Method, row.Dataset, row.Provider, row.Model, row.ModelVersion, strconv.Itoa(row.RequestCount),
			formatBenchmarkNumber(row.AvgLatencyMs), formatBenchmarkNumber(row.P50LatencyMs), formatBenchmarkNumber(row.P95LatencyMs),
			formatBenchmarkNumber(row.P99LatencyMs), formatOptionalBenchmarkNumber(row.TTFTMs), formatBenchmarkNumber(row.ThroughputRPS),
			formatBenchmarkNumber(row.SuccessRatePct), formatBenchmarkNumber(row.ErrorRatePct), formatBenchmarkNumber(row.TimeoutRatePct), row.StartedAt, row.EndedAt,
		}
		if err := writer.Write(values); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func recomputeBenchmarkSummaries(rows []benchmarkRequestDTO) []benchmarkRecomputedSummary {
	groups := map[string][]benchmarkRequestDTO{}
	order := make([]string, 0)
	for _, row := range rows {
		if _, exists := groups[row.RunID]; !exists {
			order = append(order, row.RunID)
		}
		groups[row.RunID] = append(groups[row.RunID], row)
	}
	result := make([]benchmarkRecomputedSummary, 0, len(order))
	for _, runID := range order {
		group := groups[runID]
		if len(group) == 0 {
			continue
		}
		latencies := make([]float64, 0, len(group))
		ttfts := make([]float64, 0, len(group))
		successes, errors, timeouts := 0, 0, 0
		var earliest, latest time.Time
		for _, row := range group {
			if row.LatencyMs >= 0 {
				latencies = append(latencies, row.LatencyMs)
			}
			if row.TTFTMs != nil && *row.TTFTMs >= 0 {
				ttfts = append(ttfts, *row.TTFTMs)
			}
			if row.Timeout || strings.EqualFold(row.Status, "timeout") {
				timeouts++
			} else if strings.EqualFold(row.Status, "success") && row.HTTPStatus >= 200 && row.HTTPStatus < 300 {
				successes++
			} else {
				errors++
			}
			started := parseBenchmarkTimestamp(row.StartedAt)
			ended := parseBenchmarkTimestamp(row.EndedAt)
			if !started.IsZero() && (earliest.IsZero() || started.Before(earliest)) {
				earliest = started
			}
			if !ended.IsZero() && (latest.IsZero() || ended.After(latest)) {
				latest = ended
			}
		}
		first := group[0]
		count := len(group)
		duration := latest.Sub(earliest).Seconds()
		throughput := 0.0
		if duration > 0 {
			throughput = float64(successes) / duration
		}
		summary := benchmarkRecomputedSummary{
			RunID: first.RunID, MethodID: first.MethodID, Method: first.Method, Dataset: first.Dataset, Provider: first.Provider, Model: first.Model,
			ModelVersion: first.ModelVersion, RequestCount: count, AvgLatencyMs: averageBenchmarkValues(latencies), P50LatencyMs: benchmarkPercentile(latencies, .50),
			P95LatencyMs: benchmarkPercentile(latencies, .95), P99LatencyMs: benchmarkPercentile(latencies, .99), ThroughputRPS: roundMetric(throughput),
			SuccessRatePct: roundMetric(float64(successes) * 100 / float64(count)), ErrorRatePct: roundMetric(float64(errors) * 100 / float64(count)),
			TimeoutRatePct: roundMetric(float64(timeouts) * 100 / float64(count)), StartedAt: firstNonZeroTime(earliest), EndedAt: firstNonZeroTime(latest),
		}
		if len(ttfts) > 0 {
			value := averageBenchmarkValues(ttfts)
			summary.TTFTMs = &value
		}
		result = append(result, summary)
	}
	return result
}

func filterBenchmarkErrors(rows []benchmarkRequestDTO) []benchmarkRequestDTO {
	result := make([]benchmarkRequestDTO, 0)
	for _, row := range rows {
		if row.Timeout || !strings.EqualFold(row.Status, "success") || row.HTTPStatus < 200 || row.HTTPStatus >= 300 {
			result = append(result, row)
		}
	}
	return result
}

func benchmarkMetadataJSON(requestSnapshot benchmarkRequestSnapshot, benchmarkSnapshot benchmarkSnapshot, summaries []benchmarkRecomputedSummary) ([]byte, error) {
	runs := make([]map[string]any, 0, len(summaries))
	for _, summary := range summaries {
		runs = append(runs, map[string]any{
			"runId": summary.RunID, "methodId": summary.MethodID, "method": summary.Method, "dataset": summary.Dataset,
			"providerId": summary.Provider, "modelId": summary.Model, "modelVersion": summary.ModelVersion,
		})
	}
	value := map[string]any{
		"project": "VeloxMesh AI Gateway", "generatedAt": requestSnapshot.GeneratedAt, "requestCount": len(requestSnapshot.Requests),
		"requestSource": requestSnapshot.Source, "benchmarkSource": benchmarkSnapshot.Source, "runs": runs,
		"files":            []string{"report.html", "metadata.json", "summary.csv", "raw_requests.csv", "errors_and_timeouts.csv", "charts/"},
		"rawRequestFields": benchmarkRequestCSVHeader,
	}
	return json.MarshalIndent(value, "", "  ")
}

func benchmarkReportHTML(snapshot benchmarkRequestSnapshot, summaries []benchmarkRecomputedSummary) string {
	var summaryRows strings.Builder
	for _, row := range summaries {
		fmt.Fprintf(&summaryRows, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			html.EscapeString(row.RunID), html.EscapeString(row.Method), html.EscapeString(row.Dataset), row.RequestCount,
			formatBenchmarkNumber(row.AvgLatencyMs), formatBenchmarkNumber(row.P95LatencyMs), formatBenchmarkNumber(row.SuccessRatePct)+"%")
	}
	errors := filterBenchmarkErrors(snapshot.Requests)
	var errorRows strings.Builder
	for index, row := range errors {
		if index >= 20 {
			break
		}
		fmt.Fprintf(&errorRows, "<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>", html.EscapeString(row.RequestID), html.EscapeString(row.Status), row.HTTPStatus, html.EscapeString(row.ErrorType))
	}
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>VeloxMesh Benchmark Report</title><style>body{font:14px Arial,sans-serif;color:#172033;max-width:1100px;margin:32px auto;padding:0 20px}h1,h2{color:#0f172a}table{width:100%;border-collapse:collapse;margin:12px 0 24px}th,td{border:1px solid #dbe2ea;padding:8px;text-align:left}th{background:#eef3f8}code{background:#f4f6f8;padding:2px 4px}</style></head><body>` +
		`<h1>VeloxMesh AI Gateway Benchmark Report</h1><p>Generated: ` + html.EscapeString(snapshot.GeneratedAt) + `</p>` +
		`<h2>Result Summary</h2><table><thead><tr><th>Run ID</th><th>Method</th><th>Dataset</th><th>Requests</th><th>Avg latency</th><th>P95 latency</th><th>Success</th></tr></thead><tbody>` + summaryRows.String() + `</tbody></table>` +
		`<h2>Charts</h2><p>See <code>charts/latency.svg</code>, <code>charts/tail-latency.svg</code>, <code>charts/throughput.svg</code>, and <code>charts/error-timeout-rate.svg</code>.</p>` +
		`<h2>Errors and Timeouts</h2><table><thead><tr><th>Request ID</th><th>Status</th><th>HTTP</th><th>Error type</th></tr></thead><tbody>` + errorRows.String() + `</tbody></table>` +
		`<h2>Appendix</h2><p>The complete request evidence is stored in <code>raw_requests.csv</code>. Error rows are also available in <code>errors_and_timeouts.csv</code>. Field definitions are listed in <code>metadata.json</code>. Request prompts, response bodies, and credentials are intentionally excluded.</p>` +
		`</body></html>`
}

func benchmarkBarChartSVG(title string, rows []benchmarkRecomputedSummary, metric func(benchmarkRecomputedSummary) float64, unit string) string {
	width, rowHeight := 900, 48
	height := 70 + maxInt(1, len(rows))*rowHeight
	maxValue := 0.0
	for _, row := range rows {
		maxValue = math.Max(maxValue, metric(row))
	}
	var bars strings.Builder
	for index, row := range rows {
		value := metric(row)
		barWidth := 0.0
		if maxValue > 0 {
			barWidth = value / maxValue * 500
		}
		y := 52 + index*rowHeight
		fmt.Fprintf(&bars, `<text x="16" y="%d" font-size="13">%s</text><rect x="260" y="%d" width="%.2f" height="18" fill="#2563eb"/><text x="770" y="%d" font-size="13">%s %s</text>`, y+14, html.EscapeString(row.Method), y, barWidth, y+14, formatBenchmarkNumber(value), html.EscapeString(unit))
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d"><rect width="100%%" height="100%%" fill="white"/><text x="16" y="28" font-family="Arial" font-size="20" font-weight="bold">%s</text><g font-family="Arial">%s</g></svg>`, width, height, width, height, html.EscapeString(title), bars.String())
}

func benchmarkTailLatencySVG(rows []benchmarkRecomputedSummary) string {
	return benchmarkBarChartSVG("P99 tail latency", rows, func(row benchmarkRecomputedSummary) float64 { return row.P99LatencyMs }, "ms")
}

func benchmarkErrorRateSVG(rows []benchmarkRecomputedSummary) string {
	return benchmarkBarChartSVG("Error and timeout rate", rows, func(row benchmarkRecomputedSummary) float64 { return row.ErrorRatePct + row.TimeoutRatePct }, "%")
}

func benchmarkPercentile(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(float64(len(sorted))*percentile)) - 1
	if index < 0 {
		index = 0
	}
	return roundMetric(sorted[index])
}

func averageBenchmarkValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return roundMetric(total / float64(len(values)))
}

func parseBenchmarkTimestamp(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return parsed
}

func firstNonZeroTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatBenchmarkNumber(value float64) string {
	return strconv.FormatFloat(roundMetric(value), 'f', -1, 64)
}

func formatOptionalBenchmarkNumber(value *float64) string {
	if value == nil {
		return ""
	}
	return formatBenchmarkNumber(*value)
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
