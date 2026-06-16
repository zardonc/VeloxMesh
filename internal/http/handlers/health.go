package handlers

import (
	"encoding/json"
	"net/http"
	"veloxmesh/internal/config"
	"veloxmesh/internal/gateway"
	"veloxmesh/internal/health"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Readyz evaluates whether the gateway can route traffic.
// Decision (Phase 2.4): We do not invoke adapter.HealthCheck() here because doing so
// accurately for upstream LLM providers could require an expensive model generation call.
// Instead, readiness is based strictly on the in-memory health snapshots that are updated
// by real traffic via circuit breaker semantics.
func Readyz(cfg *config.Config, svc *gateway.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshots := svc.HealthStore().Snapshots()

		healthyCount := 0
		degradedCount := 0
		unhealthyCount := 0

		for _, snap := range snapshots {
			switch snap.Status {
			case health.StatusHealthy:
				healthyCount++
			case health.StatusDegraded:
				degradedCount++
			case health.StatusUnhealthy:
				unhealthyCount++
			}
		}

		overall := "ready"
		status := http.StatusOK
		if healthyCount == 0 && degradedCount == 0 {
			overall = "unavailable"
			status = http.StatusServiceUnavailable
		}

		response := map[string]interface{}{
			"status":               overall,
			"configured_providers": len(cfg.Providers),
			"healthy":              healthyCount,
			"degraded":             degradedCount,
			"unhealthy":            unhealthyCount,
			"routing_strategy":     cfg.RoutingStrategy,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}
}
