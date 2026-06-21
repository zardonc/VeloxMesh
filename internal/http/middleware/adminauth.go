package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
)

func AdminAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				sendAdminAuthError(w, "admin_missing_authorization", "Missing Authorization header for admin API")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				sendAdminAuthError(w, "admin_invalid_authorization", "Invalid Authorization header format for admin API")
				return
			}

			token := parts[1]
			if cfg.AdminAPIKey == "" {
				// If admin API key is not configured, deny all admin requests.
				sendAdminAuthError(w, "admin_invalid_api_key", "Admin API key is not configured")
				return
			}
			if token != cfg.AdminAPIKey {
				sendAdminAuthError(w, "admin_invalid_api_key", "Invalid Admin API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func sendAdminAuthError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errors.NewGatewayError(code, message, http.StatusUnauthorized))
}
