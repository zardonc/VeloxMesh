package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	controlsqlite "veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/coordination"
	gatewayErrors "veloxmesh/internal/errors"
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/scheduler"
)

func TestAdminSchedulerRolloutRequiresAdminAuth(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.With(middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"})).Get("/admin/scheduler/rollout", handler.GetRollout)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/scheduler/rollout", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAdminSchedulerRolloutPatchUpdatesPercent(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.Patch("/admin/scheduler/rollout", handler.PatchRollout)

	req := httptest.NewRequest("PATCH", "/admin/scheduler/rollout", bytes.NewBufferString(`{"onnx_rollout_percent":0,"quality_sample_window":25}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp scheduler.RolloutResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status.ONNXRolloutPercent != 0 || resp.Status.QualitySampleWindow != 25 {
		t.Fatalf("expected rollout percent 0 and window 25, got %#v", resp.Status)
	}
}

func TestAdminSchedulerRolloutPatchUpdatesQualitySampleWindowOnly(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.Patch("/admin/scheduler/rollout", handler.PatchRollout)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("PATCH", "/admin/scheduler/rollout", bytes.NewBufferString(`{"quality_sample_window":33}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp scheduler.RolloutResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status.ONNXRolloutPercent != 10 || resp.Status.QualitySampleWindow != 33 {
		t.Fatalf("unexpected rollout response: %#v", resp.Status)
	}
}

func TestAdminSchedulerRolloutPatchValidatesPercentAndBody(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.Patch("/admin/scheduler/rollout", handler.PatchRollout)

	for _, body := range []string{`{}`, `{"onnx_rollout_percent":-1}`, `{"onnx_rollout_percent":101}`, `{"quality_sample_window":0}`, `{"quality_sample_window":10001}`, `{"onnx_rollout_percent":1,"extra":true}`} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest("PATCH", "/admin/scheduler/rollout", strings.NewReader(body)))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", body, rr.Code)
		}
	}
}

func TestAdminSchedulerAuditMetadataIsSanitized(t *testing.T) {
	ctx := context.Background()
	repo := testAdminSchedulerRepo(t)
	controller := scheduler.NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 10})
	service := scheduler.NewAdminSchedulerService(repo, controller, testSchedulerRunner(nil))

	_, err := service.Update(ctx, &scheduler.RolloutPatchRequest{ONNXRolloutPercent: ptrInt(0)})
	if err != nil {
		t.Fatalf("update rollout: %v", err)
	}
	events, err := repo.Audit().List(ctx, "scheduler-rollout")
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(events))
	}
	metadata := string(events[0].Metadata)
	for _, forbidden := range []string{"tenant", "api_key", "prompt", "message", "authorization", "secret", "payload"} {
		if strings.Contains(metadata, forbidden) {
			t.Fatalf("audit metadata contains forbidden token %q: %s", forbidden, metadata)
		}
	}
}

func TestAdminSchedulerRolloutReturnsUnavailableWhenComponentsMissing(t *testing.T) {
	service := scheduler.NewAdminSchedulerService(nil, nil, nil)
	if _, err := service.Status(context.Background()); gatewayErrorCode(err) != "scheduler_unavailable" {
		t.Fatalf("expected scheduler_unavailable from status, got %v", err)
	}
	if _, err := service.Update(context.Background(), &scheduler.RolloutPatchRequest{ONNXRolloutPercent: ptrInt(0)}); gatewayErrorCode(err) != "scheduler_unavailable" {
		t.Fatalf("expected scheduler_unavailable from update, got %v", err)
	}
}

func TestAdminSchedulerRolloutUsesRealSQLiteRepository(t *testing.T) {
	ctx := context.Background()
	repo := testAdminSchedulerRepo(t)
	start := time.Now().UTC().Truncate(time.Minute)
	if err := repo.SchedulerQualityRollups().Upsert(ctx, &controlstate.SchedulerQualityRollup{
		BucketStart: start, BucketEnd: start.Add(5 * time.Minute),
		SchedulerType: "onnx", SchedulerVersion: "v1", TaskType: "code_gen",
		ModelClass: "standard", SampleCount: 1, MAPESum: 25, WaitMSSum: 10,
		SchedulerCallLatencyMSSum: 3, ConfidenceSum: 0.8,
		SafeSampleIDs: []string{"sample-1"},
	}); err != nil {
		t.Fatalf("seed quality rollup: %v", err)
	}

	controller := scheduler.NewSchedulerRolloutController(config.SchedulerConfig{
		Enabled: true, HeuristicEndpoint: "heuristic:9000", ONNXEndpoint: "onnx:9000", ONNXRolloutPercent: 10,
	})
	handler := NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, controller, testSchedulerRunner(nil)))
	r := chi.NewRouter()
	r.With(middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"})).Patch("/admin/scheduler/rollout", handler.PatchRollout)
	r.With(middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"})).Get("/admin/scheduler/rollout", handler.GetRollout)

	patch := httptest.NewRequest("PATCH", "/admin/scheduler/rollout", bytes.NewBufferString(`{"onnx_rollout_percent":0}`))
	patch.Header.Set("Authorization", "Bearer admin")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from real sqlite rollout patch, got %d body=%s", rr.Code, rr.Body.String())
	}
	var patched scheduler.RolloutResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &patched); err != nil {
		t.Fatalf("decode patch response: %v", err)
	}
	if patched.Status.ONNXRolloutPercent != 0 || len(patched.Rollups) != 1 {
		t.Fatalf("unexpected patch response: %#v", patched)
	}

	events, err := repo.Audit().List(ctx, "scheduler-rollout")
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 || !strings.Contains(string(events[0].Metadata), "old_percent") {
		t.Fatalf("expected sanitized persisted audit event, got %#v", events)
	}
}

func gatewayErrorCode(err error) string {
	gwErr, ok := err.(*gatewayErrors.GatewayError)
	if !ok {
		return ""
	}
	return gwErr.Code
}

func TestAdminSchedulerStatusRequiresAdminAuth(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.With(middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"})).Get("/admin/v1/scheduler/status", handler.GetStatus)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/v1/scheduler/status", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAdminSchedulerStatusUsesLimitAndDefaultWithRealSQLite(t *testing.T) {
	ctx := context.Background()
	repo := testAdminSchedulerRepo(t)
	for i := 0; i < 101; i++ {
		start := time.Now().UTC().Add(-30*time.Minute + time.Duration(i)*time.Second)
		if err := repo.SchedulerQualityRollups().Upsert(ctx, testRollup(start, i)); err != nil {
			t.Fatalf("seed rollup %d: %v", i, err)
		}
	}
	handler := NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, testRolloutController(), testSchedulerRunner(nil)))
	r := chi.NewRouter()
	r.Get("/admin/v1/scheduler/status", handler.GetStatus)

	limited := httptest.NewRecorder()
	r.ServeHTTP(limited, httptest.NewRequest("GET", "/admin/v1/scheduler/status?limit=5", nil))
	if got := decodeRuntimeStatus(t, limited).QualityRollups; len(got) != 5 {
		t.Fatalf("expected 5 rollups, got %d", len(got))
	}

	defaulted := httptest.NewRecorder()
	r.ServeHTTP(defaulted, httptest.NewRequest("GET", "/admin/v1/scheduler/status", nil))
	if got := decodeRuntimeStatus(t, defaulted).QualityRollups; len(got) != 100 {
		t.Fatalf("expected default 100 rollups, got %d", len(got))
	}
}

func TestAdminSchedulerStatusWarnsWhenRollupsUnavailable(t *testing.T) {
	service := scheduler.NewAdminSchedulerService(nil, testRolloutController(), testSchedulerRunner(nil))
	resp := service.RuntimeStatus(context.Background(), 0)

	if resp.QueueDepth == nil || resp.SlotsUsed == nil || resp.SlotsTotal == nil {
		t.Fatalf("expected available queue and slot fields: %#v", resp)
	}
	if !contains(resp.Warnings, "quality_rollups_unavailable") {
		t.Fatalf("expected rollup warning, got %#v", resp.Warnings)
	}
}

func TestAdminSchedulerStatusWarnsWhenRuntimeComponentsUnavailable(t *testing.T) {
	service := scheduler.NewAdminSchedulerService(nil, testRolloutController(), nil)
	resp := service.RuntimeStatus(context.Background(), 0)

	for _, warning := range []string{"queue_unavailable", "executor_slots_unavailable", "circuit_breaker_unavailable"} {
		if !contains(resp.Warnings, warning) {
			t.Fatalf("expected warning %q, got %#v", warning, resp.Warnings)
		}
	}
}

func TestAdminSchedulerStatusIncludesCircuitBreakerState(t *testing.T) {
	scorer, err := scheduler.NewGRPCScorer(context.Background(), config.SchedulerConfig{
		Enabled: true, Endpoint: "127.0.0.1:1", Timeout: "1ms",
	})
	if err != nil {
		t.Fatalf("new grpc scorer: %v", err)
	}
	t.Cleanup(func() { _ = scorer.Close() })

	service := scheduler.NewAdminSchedulerService(nil, testRolloutController(), testSchedulerRunnerWithScorer(nil, scorer))
	resp := service.RuntimeStatus(context.Background(), 0)

	if resp.CircuitBreakerState != "closed" {
		t.Fatalf("expected closed circuit breaker, got %#v", resp)
	}
	if contains(resp.Warnings, "circuit_breaker_unavailable") {
		t.Fatalf("expected breaker state without unavailable warning: %#v", resp.Warnings)
	}
}

func TestAdminSchedulerSLARulesPutRequiresWritableAdminRoute(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	cluster := coordination.NewFakeCluster()
	follower := coordination.NewFakeCoordinator(cluster, "node-a")
	r := chi.NewRouter()
	r.With(
		middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"}),
		middleware.RequireWritable(follower),
	).Put("/admin/v1/scheduler/sla-rules", handler.PutSLARules)

	req := httptest.NewRequest("PUT", "/admin/v1/scheduler/sla-rules", strings.NewReader(`{"rules":[]}`))
	req.Header.Set("Authorization", "Bearer admin")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from writable middleware, got %d", rr.Code)
	}
}

func TestAdminSchedulerInvalidSLARulesLeaveOldRules(t *testing.T) {
	oldRule := validSLARule("old")
	service := scheduler.NewAdminSchedulerService(testAdminSchedulerRepo(t), testRolloutController(), testSchedulerRunner([]config.SLAPromotionRule{oldRule}))

	_, err := service.ReplaceSLARules(context.Background(), &scheduler.SLARulesReplaceRequest{
		Rules: []config.SLAPromotionRule{{PolicyID: "bad", TenantClass: "gold", ModelClass: "standard", RequestKind: "urgent", WaitThreshold: "1s"}},
	})
	if err == nil {
		t.Fatal("expected invalid rule error")
	}
	got := service.SLARules().Rules
	if len(got) != 1 || got[0].PolicyID != oldRule.PolicyID {
		t.Fatalf("expected old rule to remain, got %#v", got)
	}
}

func TestAdminSchedulerSLARulesReplaceAuditsSafeMetadata(t *testing.T) {
	ctx := context.Background()
	repo := testAdminSchedulerRepo(t)
	handler := NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, testRolloutController(), testSchedulerRunner([]config.SLAPromotionRule{validSLARule("old")})))
	r := chi.NewRouter()
	r.Put("/admin/v1/scheduler/sla-rules", handler.PutSLARules)

	body := `{"rules":[{"policy_id":"new","tenant_id":"tenant-123","tenant_class":"gold","model_class":"standard","request_kind":"code_gen","wait_threshold":"2s"}]}`
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("PUT", "/admin/v1/scheduler/sla-rules", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	events, err := repo.Audit().List(ctx, "scheduler-sla-rules")
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one audit event, got %d", len(events))
	}
	metadata := string(events[0].Metadata)
	for _, want := range []string{"old_count", "new_count", "policy_id", "tenant_class", "model_class", "request_kind"} {
		if !strings.Contains(metadata, want) {
			t.Fatalf("expected audit metadata to contain %q: %s", want, metadata)
		}
	}
	for _, forbidden := range []string{"tenant-123", "tenant_id", "prompt", "payload", "embedding", "authorization", "api_key", "secret"} {
		if strings.Contains(metadata, forbidden) {
			t.Fatalf("audit metadata contains forbidden token %q: %s", forbidden, metadata)
		}
	}
}

func TestAdminSchedulerTrainingExportJSONAndNDJSONAreSafe(t *testing.T) {
	ctx := context.Background()
	repo := testAdminSchedulerRepo(t)
	now := time.Now().UTC()
	for _, sample := range []*controlstate.SchedulerTrainingSample{
		testTrainingSample("sample-a", "code_gen", now),
		testTrainingSample("sample-b", "simple_qa", now.Add(time.Second)),
	} {
		if err := repo.SchedulerTrainingSamples().Insert(ctx, sample); err != nil {
			t.Fatalf("insert sample: %v", err)
		}
	}
	handler := NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, testRolloutController(), testSchedulerRunner(nil)))
	r := chi.NewRouter()
	r.Get("/admin/v1/scheduler/training-samples/export", handler.ExportTrainingSamples)

	jsonRR := httptest.NewRecorder()
	r.ServeHTTP(jsonRR, httptest.NewRequest("GET", "/admin/v1/scheduler/training-samples/export?task_type=code_gen", nil))
	if jsonRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", jsonRR.Code, jsonRR.Body.String())
	}
	if !strings.Contains(jsonRR.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON content type, got %q", jsonRR.Header().Get("Content-Type"))
	}
	assertSafeExportBody(t, jsonRR.Body.String())
	var resp scheduler.TrainingExportResponse
	if err := json.Unmarshal(jsonRR.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(resp.Samples) != 1 || resp.Samples[0].Features.RequestKind != "code_gen" {
		t.Fatalf("expected filtered code_gen sample, got %#v", resp.Samples)
	}

	ndjsonRR := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/v1/scheduler/training-samples/export?format=ndjson", nil)
	r.ServeHTTP(ndjsonRR, req)
	if ndjsonRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", ndjsonRR.Code, ndjsonRR.Body.String())
	}
	if !strings.Contains(ndjsonRR.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("expected NDJSON content type, got %q", ndjsonRR.Header().Get("Content-Type"))
	}
	lines := strings.Split(strings.TrimSpace(ndjsonRR.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two NDJSON lines, got %d: %q", len(lines), ndjsonRR.Body.String())
	}
	assertSafeExportBody(t, ndjsonRR.Body.String())
}

func testAdminSchedulerHandler(t *testing.T) *AdminSchedulerHandler {
	t.Helper()
	repo := testAdminSchedulerRepo(t)
	return NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, testRolloutController(), testSchedulerRunner(nil)))
}

func testAdminSchedulerRepo(t *testing.T) *controlsqlite.Repository {
	t.Helper()
	repo, err := controlsqlite.Open("file:" + strings.ReplaceAll(t.Name(), "/", "-") + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return repo
}

func ptrInt(value int) *int {
	return &value
}

func testRolloutController() *scheduler.SchedulerRolloutController {
	return scheduler.NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 10})
}

func testSchedulerRunner(rules []config.SLAPromotionRule) *scheduler.SynchronousRunner {
	return testSchedulerRunnerWithScorer(rules, nil)
}

func testSchedulerRunnerWithScorer(rules []config.SLAPromotionRule, scorer scheduler.Scorer) *scheduler.SynchronousRunner {
	queue := scheduler.NewMemoryQueue()
	registry := scheduler.NewResultRegistry()
	executor := &scheduler.Executor{
		Queue:    queue,
		Registry: registry,
		Promoter: &scheduler.SLAPromoter{Enabled: true, Rules: rules},
	}
	return scheduler.NewSynchronousRunnerWithConcurrency(&scheduler.TaskIntake{Scorer: scorer}, executor, registry, 3)
}

func validSLARule(policyID string) config.SLAPromotionRule {
	return config.SLAPromotionRule{
		PolicyID:      policyID,
		TenantClass:   "gold",
		ModelClass:    "standard",
		RequestKind:   "code_gen",
		WaitThreshold: "1s",
	}
}

func testRollup(start time.Time, i int) *controlstate.SchedulerQualityRollup {
	return &controlstate.SchedulerQualityRollup{
		BucketStart: start, BucketEnd: start.Add(time.Second),
		SchedulerType: "onnx", SchedulerVersion: "v1", TaskType: "code_gen",
		ModelClass: "standard", CoverageLevel: "tenant", SampleCount: int64(i + 1),
		MAPESum: 25, WaitMSSum: 10, SchedulerCallLatencyMSSum: 3, ConfidenceSum: 0.8,
		SafeSampleIDs: []string{"sample"},
	}
}

func testTrainingSample(id string, requestKind string, completed time.Time) *controlstate.SchedulerTrainingSample {
	return &controlstate.SchedulerTrainingSample{
		ID: id, TaskID: "task-" + id, ModelClass: "standard", RequestKind: requestKind,
		Priority: "normal", Stream: true, CoverageLevel: "tenant", CoverageRatio: 0.8,
		SchedulerVersion: "heuristic-v1", Outcome: "success", ActualLatencyMs: 42,
		InputTokens: 12, OutputTokens: 80, ProviderClass: "openai-compatible",
		CompletedAt: completed,
	}
}

func assertSafeExportBody(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{"task-", "tenant_id", "user_id", "prompt", "embedding", "payload", "authorization", "api_key", "secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("export body contains forbidden token %q: %s", forbidden, body)
		}
	}
}

func decodeRuntimeStatus(t *testing.T, rr *httptest.ResponseRecorder) scheduler.SchedulerRuntimeStatus {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp scheduler.SchedulerRuntimeStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return resp
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
