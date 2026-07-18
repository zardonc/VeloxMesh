package bff

import (
	"bufio"
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type summaryDataSourceDTO struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
	GeneratedAt string `json:"generatedAt,omitempty"`
}

type adminSummaryDTO struct {
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
	Topology        *GatewayTopology       `json:"topology"`
	LatestBenchmark *benchmarkDTO          `json:"latestBenchmark"`
	ProviderHealth  []providerHealthDTO    `json:"providerHealth"`
	RecentErrors    []requestLogDTO        `json:"recentErrors"`
	GeneratedAt     string                 `json:"generatedAt"`
	DataSources     []summaryDataSourceDTO `json:"dataSources"`
	Partial         bool                   `json:"partial"`
	PartialData     bool                   `json:"partialData"`
	Warnings        []string               `json:"warnings"`
	Error           string                 `json:"error,omitempty"`
}

type adminSummaryGatewayResults struct {
	health       GatewayHealth
	healthErr    error
	readiness    GatewayReadiness
	readinessErr error
	providers    []GatewayProvider
	providersErr error
	topology     GatewayTopology
	topologyErr  error
	metrics      string
	metricsErr   error
}

type requestSummaryStats struct {
	requestVolume int
	activeTenants int
	avgLatencyMs  float64
	p95LatencyMs  float64
	successRate   float64
	errorRate     float64
	timeoutRate   float64
}

func (server *Server) handleAdminSummary(w http.ResponseWriter, r *http.Request) {
	if server.config.DemoMode {
		writeJSON(w, http.StatusOK, server.demoAdminSummary(r.Context()))
		return
	}

	summary, available := server.buildAdminSummary(r.Context())
	if available == 0 {
		summary.Error = "admin_summary_unavailable"
		writeJSON(w, http.StatusServiceUnavailable, summary)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (server *Server) buildAdminSummary(ctx context.Context) (adminSummaryDTO, int) {
	generatedAt := server.now().UTC()
	summary := adminSummaryDTO{
		GatewayStatus:  "Error",
		ProviderHealth: []providerHealthDTO{},
		RecentErrors:   []requestLogDTO{},
		GeneratedAt:    generatedAt.Format(time.RFC3339),
		DataSources:    []summaryDataSourceDTO{},
		Warnings:       []string{},
	}

	var gateway adminSummaryGatewayResults
	var operational operationalSnapshot
	var benchmarks benchmarkSnapshot
	var wait sync.WaitGroup
	wait.Add(7)
	if server.gatewayAdmin == nil {
		detail := "VeloxMesh Admin API is not configured"
		if server.gatewayAdminErr != nil {
			detail = "VeloxMesh Admin API configuration is invalid"
		}
		gateway.healthErr = summarySourceError(detail)
		gateway.readinessErr = summarySourceError(detail)
		gateway.providersErr = summarySourceError(detail)
		gateway.topologyErr = summarySourceError(detail)
		gateway.metricsErr = summarySourceError(detail)
		wait.Add(-5)
	} else {
		go func() { defer wait.Done(); gateway.health, gateway.healthErr = server.gatewayAdmin.GetHealth(ctx) }()
		go func() {
			defer wait.Done()
			gateway.readiness, gateway.readinessErr = server.gatewayAdmin.GetReadiness(ctx)
		}()
		go func() {
			defer wait.Done()
			gateway.providers, gateway.providersErr = server.gatewayAdmin.ListProviders(ctx)
		}()
		go func() {
			defer wait.Done()
			gateway.topology, gateway.topologyErr = server.gatewayAdmin.GetTopology(ctx)
		}()
		go func() { defer wait.Done(); gateway.metrics, gateway.metricsErr = server.gatewayAdmin.GetMetrics(ctx) }()
	}
	go func() { defer wait.Done(); operational = server.operationalStore.Snapshot(ctx) }()
	go func() { defer wait.Done(); benchmarks = server.benchmarkStore.Snapshot(ctx) }()
	wait.Wait()

	available := 0
	addSource := func(name, source string, err error, detail, sourceGeneratedAt string, empty bool) {
		status := "ok"
		if err != nil {
			status = "error"
			detail = shortError(err)
		} else if empty {
			status = "empty"
		}
		if status == "ok" {
			available++
		} else {
			warning := name + " source is " + status
			if detail != "" {
				warning += ": " + detail
			}
			summary.Warnings = append(summary.Warnings, warning)
		}
		summary.DataSources = append(summary.DataSources, summaryDataSourceDTO{Name: name, Source: source, Status: status, Detail: detail, GeneratedAt: sourceGeneratedAt})
	}

	addSource("Gateway health", "/healthz", gateway.healthErr, gateway.health.Status, "", false)
	addSource("Gateway readiness", "/readyz", gateway.readinessErr, gateway.readiness.Status, "", false)
	addSource("Active providers", "/admin/v1/providers", gateway.providersErr, "VeloxMesh Admin API", "", false)
	addSource("Topology", "/admin/v1/topology", gateway.topologyErr, gateway.topology.Role, "", false)
	queueDepth, hasQueueDepth := parseGatewayQueueDepth(gateway.metrics)
	addSource("Queue depth", "/metrics", gateway.metricsErr, "gateway_queue_depth", "", gateway.metricsErr == nil && !hasQueueDepth)
	operationalAvailable := operational.Source != "" && operational.Source != "empty"
	addSource("Operational data", operationalSource(operational), operationalSourceError(operational), operational.Redis.Detail, operational.GeneratedAt, !operationalAvailable)
	benchmarkAvailable := benchmarks.Source != "" && benchmarks.Source != "empty"
	addSource("Benchmark data", benchmarkSource(benchmarks), benchmarkSourceError(benchmarks), benchmarks.Redis.Detail, benchmarks.GeneratedAt, !benchmarkAvailable)

	if gateway.healthErr == nil && gateway.readinessErr == nil && strings.EqualFold(gateway.health.Status, "ok") && readinessHealthy(gateway.readiness.Status) {
		summary.GatewayStatus = "Healthy"
	} else if gateway.healthErr == nil || gateway.readinessErr == nil {
		summary.GatewayStatus = "Partial"
	}
	if gateway.readinessErr == nil {
		summary.RoutingStrategy = gateway.readiness.RoutingStrategy
	}
	if gateway.providersErr == nil {
		applyProviderSummary(&summary, gateway.providers)
	}
	if gateway.topologyErr == nil {
		topology := gateway.topology
		summary.Topology = &topology
	}
	if gateway.metricsErr == nil && hasQueueDepth {
		summary.QueueDepth = floatPointer(roundMetric(queueDepth))
	}
	if operationalAvailable {
		stats := calculateRequestSummaryStats(operational.RequestLogs, generatedAt)
		summary.RequestVolume = intPointer(stats.requestVolume)
		summary.ActiveTenants = intPointer(stats.activeTenants)
		summary.AvgLatencyMs = floatPointer(stats.avgLatencyMs)
		summary.P95LatencyMs = floatPointer(stats.p95LatencyMs)
		summary.SuccessRate = floatPointer(stats.successRate)
		summary.ErrorRate = floatPointer(stats.errorRate)
		summary.TimeoutRate = floatPointer(stats.timeoutRate)
		summary.ProviderHealth = append([]providerHealthDTO(nil), operational.ProviderHealth...)
		summary.RecentErrors = recentOperationalErrors(operational.RequestLogs)
	}
	if benchmarkAvailable {
		summary.LatestBenchmark = latestBenchmark(benchmarks.Benchmarks)
	}

	summary.Partial = available < len(summary.DataSources)
	summary.PartialData = summary.Partial
	if available == 0 {
		summary.Partial = true
		summary.PartialData = true
	}
	return summary, available
}

func (server *Server) demoAdminSummary(ctx context.Context) adminSummaryDTO {
	server.state.mu.Lock()
	providers := append([]providerDTO(nil), server.state.providers...)
	server.state.mu.Unlock()
	modelCount := len(server.config.Models)
	activeProviders := len(providers)
	logs := server.requestLogs()
	stats := calculateRequestSummaryStats(logs, server.now().UTC())
	summary := adminSummaryDTO{
		DefaultProvider: server.config.ProviderName,
		DefaultModel:    server.config.DefaultModel,
		ModelCount:      intPointer(modelCount),
		ActiveProviders: intPointer(activeProviders),
		ActiveTenants:   intPointer(stats.activeTenants),
		RequestVolume:   intPointer(stats.requestVolume),
		AvgLatencyMs:    floatPointer(stats.avgLatencyMs),
		P95LatencyMs:    floatPointer(stats.p95LatencyMs),
		SuccessRate:     floatPointer(stats.successRate),
		ErrorRate:       floatPointer(stats.errorRate),
		TimeoutRate:     floatPointer(stats.timeoutRate),
		GatewayStatus:   "Healthy",
		ProviderHealth:  server.providerHealth(),
		RecentErrors:    recentOperationalErrors(logs),
		GeneratedAt:     server.now().UTC().Format(time.RFC3339),
		DataSources:     []summaryDataSourceDTO{{Name: "Demo data", Source: "DASHBOARD_DEMO_MODE", Status: "ok", Detail: "explicit demo mode"}},
		Warnings:        []string{},
	}
	benchmarks := server.benchmarkStore.Snapshot(ctx)
	benchmarkStatus := "ok"
	benchmarkDetail := benchmarks.Redis.Detail
	if err := benchmarkSourceError(benchmarks); err != nil {
		benchmarkStatus = "error"
		benchmarkDetail = shortError(err)
	} else if len(benchmarks.Benchmarks) == 0 {
		benchmarkStatus = "empty"
	}
	if len(benchmarks.Benchmarks) > 0 {
		summary.LatestBenchmark = latestBenchmark(benchmarks.Benchmarks)
	}
	summary.DataSources = append(summary.DataSources, summaryDataSourceDTO{
		Name: "Benchmark data", Source: benchmarkSource(benchmarks), Status: benchmarkStatus, Detail: benchmarkDetail, GeneratedAt: benchmarks.GeneratedAt,
	})
	if benchmarkStatus != "ok" {
		summary.Partial = true
		summary.PartialData = true
		warning := "Benchmark data source is " + benchmarkStatus
		if benchmarkDetail != "" {
			warning += ": " + benchmarkDetail
		}
		summary.Warnings = append(summary.Warnings, warning)
	}
	return summary
}

func applyProviderSummary(summary *adminSummaryDTO, providers []GatewayProvider) {
	active := 0
	models := map[string]struct{}{}
	for _, provider := range providers {
		if !provider.Enabled {
			continue
		}
		active++
		if summary.DefaultProvider == "" {
			summary.DefaultProvider = provider.ID
			summary.DefaultModel = provider.DefaultModel
		}
		for _, model := range provider.Models {
			models[model] = struct{}{}
		}
	}
	summary.ActiveProviders = intPointer(active)
	summary.ModelCount = intPointer(len(models))
}

func calculateRequestSummaryStats(logs []requestLogDTO, now time.Time) requestSummaryStats {
	latencies := make([]float64, 0, len(logs))
	tenants := map[string]struct{}{}
	successes, errors, timeouts := 0, 0, 0
	for _, row := range logs {
		timestamp, err := time.Parse(time.RFC3339, strings.TrimSpace(row.Timestamp))
		if err != nil || !sameUTCDate(timestamp, now) {
			continue
		}
		latencies = append(latencies, row.LatencyMs)
		if row.Tenant != "" {
			tenants[row.Tenant] = struct{}{}
		}
		switch requestOutcome(row.Status) {
		case "success":
			successes++
		case "timeout":
			timeouts++
		default:
			errors++
		}
	}
	stats := requestSummaryStats{requestVolume: len(latencies), activeTenants: len(tenants)}
	if len(latencies) == 0 {
		return stats
	}
	sorted := append([]float64(nil), latencies...)
	sort.Float64s(sorted)
	var total float64
	for _, latency := range latencies {
		total += latency
	}
	index := int(math.Ceil(float64(len(sorted))*0.95)) - 1
	stats.avgLatencyMs = roundMetric(total / float64(len(latencies)))
	stats.p95LatencyMs = roundMetric(sorted[index])
	denominator := float64(len(latencies))
	stats.successRate = roundMetric(float64(successes) * 100 / denominator)
	stats.errorRate = roundMetric(float64(errors) * 100 / denominator)
	stats.timeoutRate = roundMetric(float64(timeouts) * 100 / denominator)
	return stats
}

func recentOperationalErrors(logs []requestLogDTO) []requestLogDTO {
	result := make([]requestLogDTO, 0)
	for _, row := range logs {
		if requestOutcome(row.Status) != "success" {
			result = append(result, row)
		}
	}
	sort.SliceStable(result, func(left, right int) bool {
		return parseQueryTime(result[left].Timestamp).After(parseQueryTime(result[right].Timestamp))
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func latestBenchmark(rows []benchmarkDTO) *benchmarkDTO {
	if len(rows) == 0 {
		return nil
	}
	latest := rows[0]
	latestTime := benchmarkTime(latest.TestDate)
	for _, row := range rows[1:] {
		if candidate := benchmarkTime(row.TestDate); candidate.After(latestTime) {
			latest = row
			latestTime = candidate
		}
	}
	return &latest
}

func benchmarkTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func parseGatewayQueueDepth(metrics string) (float64, bool) {
	var total float64
	found := false
	scanner := bufio.NewScanner(strings.NewReader(metrics))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "gateway_queue_depth{") && !strings.HasPrefix(line, "gateway_queue_depth ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		total += value
		found = true
	}
	return total, found
}

func operationalSource(snapshot operationalSnapshot) string {
	if snapshot.Source == "" || snapshot.Source == "empty" {
		return "Operational Store"
	}
	return snapshot.Source
}

func benchmarkSource(snapshot benchmarkSnapshot) string {
	if snapshot.Source == "" || snapshot.Source == "empty" {
		return "Benchmark Store"
	}
	return snapshot.Source
}

func operationalSourceError(snapshot operationalSnapshot) error {
	if strings.EqualFold(snapshot.Redis.Status, "unreachable") || strings.EqualFold(snapshot.Redis.Status, "error") {
		return summarySourceError("Operational Store: " + snapshot.Redis.Detail)
	}
	return nil
}

func benchmarkSourceError(snapshot benchmarkSnapshot) error {
	if strings.EqualFold(snapshot.Redis.Status, "unreachable") && strings.EqualFold(snapshot.Qdrant.Status, "unreachable") {
		return summarySourceError("Benchmark Store is unreachable")
	}
	return nil
}

type summarySourceError string

func (err summarySourceError) Error() string { return string(err) }

func readinessHealthy(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	return normalized == "ready" || normalized == "ok" || normalized == "healthy"
}

func requestOutcome(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if strings.Contains(normalized, "timeout") || strings.Contains(normalized, "timed out") || strings.Contains(normalized, "deadline") {
		return "timeout"
	}
	if normalized == "success" || normalized == "passed" || normalized == "settled" || normalized == "ok" || strings.HasPrefix(normalized, "2") {
		return "success"
	}
	return "error"
}

func sameUTCDate(left, right time.Time) bool {
	left = left.UTC()
	right = right.UTC()
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}

func intPointer(value int) *int { return &value }

func floatPointer(value float64) *float64 { return &value }
