package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"

	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/controlstate/sqlite"
	"veloxmesh/internal/coordination"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/handlers"
	"veloxmesh/internal/pipeline"
)

func TestSemanticRulesRoutesUseRealSQLiteStore(t *testing.T) {
	dsn := localSemanticRulesTestDB(t)

	repo, router := semanticRulesTestRouter(t, dsn)

	cfg := pipeline.DefaultSemanticPipelineConfig()
	cfg.Rules[pipeline.RulePII] = pipeline.RuleConfig{Enabled: true}
	putJSON(t, router, "/admin/v1/semantic-rules", "admin", cfg, http.StatusNoContent)

	global := getRules(t, router, "/admin/v1/semantic-rules", "admin", http.StatusOK)
	if !global.Rules[pipeline.RulePII].Enabled {
		t.Fatalf("expected global PII rule enabled")
	}

	userCfg := pipeline.DefaultSemanticPipelineConfig()
	userCfg.Rules[pipeline.RuleHeadroom] = pipeline.RuleConfig{Enabled: true}
	putJSON(t, router, "/admin/v1/semantic-rules/users/user-a", "admin", userCfg, http.StatusNoContent)

	gotUser := getRules(t, router, "/admin/v1/semantic-rules/users/user-a", "admin", http.StatusOK)
	if !gotUser.Rules[pipeline.RuleHeadroom].Enabled {
		t.Fatalf("expected user Headroom rule enabled")
	}
	if gotUser.Rules[pipeline.RulePII].Enabled {
		t.Fatalf("expected user config not to inherit global PII row directly")
	}

	getRules(t, router, "/admin/v1/semantic-rules", "", http.StatusUnauthorized)
	if err := repo.Close(); err != nil {
		t.Fatalf("close sqlite before reopen: %v", err)
	}

	repo, router = semanticRulesTestRouter(t, dsn)
	defer repo.Close()
	reopened := getRules(t, router, "/admin/v1/semantic-rules", "admin", http.StatusOK)
	if !reopened.Rules[pipeline.RulePII].Enabled {
		t.Fatalf("expected global PII rule persisted in local sqlite file")
	}
}

func localSemanticRulesTestDB(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "veloxmesh-test.db")
}

func semanticRulesTestRouter(t *testing.T, dsn string) (*sqlite.Repository, http.Handler) {
	t.Helper()
	ctx := context.Background()
	repo, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := repo.Migrate(ctx); err != nil {
		repo.Close()
		t.Fatalf("migrate sqlite: %v", err)
	}

	hot := hotstate.NewLocalHotState()
	svc := controlstate.NewAdminSemanticRulesService(repo, hot)
	handler := handlers.NewAdminSemanticRulesHandler(svc)
	router := NewRouter(&config.Config{AdminAPIKey: "admin"}, nil, nil, nil, handler, hot, repo, coordination.NewNoopCoordinator(), nil)
	return repo, router
}

func putJSON(t *testing.T, router http.Handler, path, adminKey string, body any, want int) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(payload))
	if adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+adminKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("PUT %s: expected %d, got %d body=%s", path, want, rec.Code, rec.Body.String())
	}
}

func getRules(t *testing.T, router http.Handler, path, adminKey string, want int) *pipeline.SemanticPipelineConfig {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if adminKey != "" {
		req.Header.Set("Authorization", "Bearer "+adminKey)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("GET %s: expected %d, got %d body=%s", path, want, rec.Code, rec.Body.String())
	}
	if want != http.StatusOK {
		return nil
	}
	var cfg pipeline.SemanticPipelineConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return &cfg
}
