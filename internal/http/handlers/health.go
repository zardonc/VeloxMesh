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

		caps := svc.GetProviderCapabilities()
		capMap := make(map[string]map[string]interface{})
		for _, c := range caps {
			capMap[c.ID] = map[string]interface{}{
				"provider_type": string(c.Capabilities.ProviderType),
				"streaming":     c.Capabilities.Streaming,
				"tool_calling":  c.Capabilities.ToolCalling,
			}
		}

		var providerDetails []map[string]interface{}
		for _, snap := range snapshots {
			switch snap.Status {
			case health.StatusHealthy:
				healthyCount++
			case health.StatusDegraded:
				degradedCount++
			case health.StatusUnhealthy:
				unhealthyCount++
			}

			detail := map[string]interface{}{
				"id":     snap.ID,
				"status": string(snap.Status),
			}
			if c, ok := capMap[snap.ID]; ok {
				detail["capabilities"] = c
			}
			if !snap.LastProbeAt.IsZero() {
				detail["last_probe_at"] = snap.LastProbeAt
				detail["last_probe_success"] = snap.LastProbeSuccess
				if snap.LastProbeError != "" {
					detail["last_probe_error"] = snap.LastProbeError
				}
				detail["last_probe_duration_ms"] = snap.LastProbeDuration.Milliseconds()
			}
			providerDetails = append(providerDetails, detail)
		}

		overall := "ready"
		status := http.StatusOK
		if healthyCount == 0 && degradedCount == 0 {
			overall = "unavailable"
			status = http.StatusServiceUnavailable
		}

		probeEnabled := false
		if cfg.HealthCheck.Enabled != nil {
			probeEnabled = *cfg.HealthCheck.Enabled
		}

		response := map[string]interface{}{
			"status":               overall,
			"configured_providers": len(cfg.Providers),
			"healthy":              healthyCount,
			"degraded":             degradedCount,
			"unhealthy":            unhealthyCount,
			"routing_strategy":     cfg.RoutingStrategy,
			"probe_enabled":        probeEnabled,
			"providers":            providerDetails,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}
}
