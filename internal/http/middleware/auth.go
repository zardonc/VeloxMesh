package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/config"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/hotstate"
)

func Auth(cfg *config.Config, cache hotstate.Client) func(http.Handler) http.Handler {
	var ttl time.Duration
	if cfg.RedisAuthCacheTTL != "" {
		ttl, _ = time.ParseDuration(cfg.RedisAuthCacheTTL)
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

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

			hash := sha256.Sum256([]byte(token))
			tokenHash := hex.EncodeToString(hash[:])

			if cache != nil {
				if allowed, err := cache.GetCachedAuthResult(r.Context(), tokenHash); err == nil {
					if allowed {
						next.ServeHTTP(w, r)
						return
					}
					sendAuthError(w, "invalid_api_key", "Invalid API key")
					return
				}
			}

			allowed := (token == cfg.DevAPIKey)

			if cache != nil {
				_ = cache.CacheAuthResult(r.Context(), tokenHash, allowed, ttl)
			}

			if !allowed {
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
