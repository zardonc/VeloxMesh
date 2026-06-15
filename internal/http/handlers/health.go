package handlers

import (
	"net/http"
	"veloxmesh/internal/config"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func Readyz(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// For Phase 1, readiness checks config and provider registry availability.
		if cfg.DefaultProvider == "" || cfg.PrimaryBaseURL == "" || cfg.PrimaryAPIKey == "" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("service unavailable: incomplete configuration"))
			return
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	}
}
