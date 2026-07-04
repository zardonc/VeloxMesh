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

	req := httptest.NewRequest("PATCH", "/admin/scheduler/rollout", bytes.NewBufferString(`{"onnx_rollout_percent":0}`))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp scheduler.RolloutResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status.ONNXRolloutPercent != 0 {
		t.Fatalf("expected rollout percent 0, got %#v", resp.Status)
	}
}

func TestAdminSchedulerRolloutPatchValidatesPercentAndBody(t *testing.T) {
	handler := testAdminSchedulerHandler(t)
	r := chi.NewRouter()
	r.Patch("/admin/scheduler/rollout", handler.PatchRollout)

	for _, body := range []string{`{"onnx_rollout_percent":-1}`, `{"onnx_rollout_percent":101}`, `{"onnx_rollout_percent":1,"extra":true}`} {
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
	service := scheduler.NewAdminSchedulerService(repo, controller)

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
	handler := NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, controller))
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

func testAdminSchedulerHandler(t *testing.T) *AdminSchedulerHandler {
	t.Helper()
	repo := testAdminSchedulerRepo(t)
	controller := scheduler.NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 10})
	return NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, controller))
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
