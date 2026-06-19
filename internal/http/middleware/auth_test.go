package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"veloxmesh/internal/config"
	"veloxmesh/internal/hotstate"
	"veloxmesh/internal/http/middleware"
)

func TestAuthMiddlewareCache(t *testing.T) {
	cfg := &config.Config{
		DevAPIKey:         "test-dev-key",
		RedisAuthCacheTTL: "1m",
	}

	cache := hotstate.NewLocalHotState()
	authMiddleware := middleware.Auth(cfg, cache)

	handler := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Initial request (Cache miss, allowed)
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("Authorization", "Bearer test-dev-key")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec1.Code)
	}

	// Verify cached
	// sha256 of "test-dev-key"
	// Let's just do a 2nd request to verify it uses cache
	// We'll temporarily change DevAPIKey to verify cache hit
	cfg.DevAPIKey = "different-key"
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer test-dev-key")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 from cache, got %d", rec2.Code)
	}

	// 3. Denied request caching
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Header.Set("Authorization", "Bearer bad-key")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec3.Code)
	}

	// Change DevAPIKey to bad-key
	cfg.DevAPIKey = "bad-key"
	req4 := httptest.NewRequest(http.MethodGet, "/", nil)
	req4.Header.Set("Authorization", "Bearer bad-key")
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 from negative cache, got %d", rec4.Code)
	}
}
