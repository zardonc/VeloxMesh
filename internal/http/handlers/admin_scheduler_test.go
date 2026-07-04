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
	"veloxmesh/internal/http/middleware"
	"veloxmesh/internal/scheduler"
)

func TestAdminSchedulerRolloutRequiresAdminAuth(t *testing.T) {
	handler := testAdminSchedulerHandler()
	r := chi.NewRouter()
	r.With(middleware.AdminAuth(&config.Config{AdminAPIKey: "admin"})).Get("/admin/scheduler/rollout", handler.GetRollout)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/scheduler/rollout", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestAdminSchedulerRolloutPatchUpdatesPercent(t *testing.T) {
	handler := testAdminSchedulerHandler()
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
	handler := testAdminSchedulerHandler()
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
	audit := &capturingAuditRepo{}
	repo := &mockAdminRepo{auditRepo: audit, quality: &emptyQualityRepo{}}
	controller := scheduler.NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 10})
	service := scheduler.NewAdminSchedulerService(repo, controller)

	_, err := service.Update(context.Background(), &scheduler.RolloutPatchRequest{ONNXRolloutPercent: ptrInt(0)})
	if err != nil {
		t.Fatalf("update rollout: %v", err)
	}
	metadata := string(audit.event.Metadata)
	for _, forbidden := range []string{"tenant", "api_key", "prompt", "message", "authorization", "secret", "payload"} {
		if strings.Contains(metadata, forbidden) {
			t.Fatalf("audit metadata contains forbidden token %q: %s", forbidden, metadata)
		}
	}
}

func testAdminSchedulerHandler() *AdminSchedulerHandler {
	repo := &mockAdminRepo{auditRepo: &mockAuditRepo{}, quality: &emptyQualityRepo{}}
	controller := scheduler.NewSchedulerRolloutController(config.SchedulerConfig{Enabled: true, HeuristicEndpoint: "h", ONNXEndpoint: "o", ONNXRolloutPercent: 10})
	return NewAdminSchedulerHandler(scheduler.NewAdminSchedulerService(repo, controller))
}

type emptyQualityRepo struct{}

func (r *emptyQualityRepo) Upsert(context.Context, *controlstate.SchedulerQualityRollup) error {
	return nil
}

func (r *emptyQualityRepo) ListByWindow(context.Context, time.Time, time.Time, string, string, string, int) ([]*controlstate.SchedulerQualityRollup, error) {
	return []*controlstate.SchedulerQualityRollup{}, nil
}

type capturingAuditRepo struct {
	controlstate.AuditRepository
	event *controlstate.AuditEvent
}

func (r *capturingAuditRepo) Log(_ context.Context, event *controlstate.AuditEvent) error {
	r.event = event
	return nil
}

func ptrInt(value int) *int {
	return &value
}
