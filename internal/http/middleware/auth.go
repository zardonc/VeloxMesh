package middleware

import (
	"encoding/json"
	"net/http"
	"strings"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
)

func Auth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				sendAuthError(w, "missing_authorization", "Missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				sendAuthError(w, "invalid_authorization", "Invalid Authorization header format")
				return
			}

			token := parts[1]
			if token != cfg.DevAPIKey {
				sendAuthError(w, "invalid_api_key", "Invalid API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func sendAuthError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errors.NewGatewayError(code, message, http.StatusUnauthorized))
}
