package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"veloxmesh/internal/config"
	"veloxmesh/internal/controlstate"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/hotstate"
)

type AuthIdentity struct {
	ID            string
	Role          string
	CreditBalance int64
}

type authContextKey string

const AuthIdentityKey authContextKey = "auth_identity"

func GetAuthIdentity(ctx context.Context) *AuthIdentity {
	if identity, ok := ctx.Value(AuthIdentityKey).(*AuthIdentity); ok {
		return identity
	}
	return nil
}

func Auth(cfg *config.Config, cache hotstate.Client, repo controlstate.Repository) func(http.Handler) http.Handler {
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

			var identity *AuthIdentity

			if cache != nil {
				if allowed, err := cache.GetCachedAuthResult(r.Context(), tokenHash); err == nil {
					if allowed {
						// Note: Cache doesn't hold the full identity in Phase 1's simplistic form.
						// To properly cache identity, we would need to store the identity object in cache.
						// For now, if we have repo, we MUST query it to get credits.
						if repo == nil {
							next.ServeHTTP(w, r)
							return
						}
						// If repo exists, we skip cache return here because we need CreditBalance
						// A full cache implementation would cache the APIKeyRecord itself.
					} else {
						sendAuthError(w, "invalid_api_key", "Invalid API key")
						return
					}
				}
			}

			allowed := false

			if repo != nil && repo.APIKeys() != nil {
				if keyRecord, err := repo.APIKeys().GetByHash(r.Context(), tokenHash); err == nil && keyRecord != nil {
					if keyRecord.Enabled {
						allowed = true
						identity = &AuthIdentity{
							ID:            keyRecord.ID,
							Role:          keyRecord.Role,
							CreditBalance: keyRecord.CreditBalance,
						}
					}
				}
			} else {
				// Disabled mode / dev fallback
				if token == cfg.DevAPIKey {
					allowed = true
					identity = &AuthIdentity{
						ID:            "dev-key",
						Role:          "admin",
						CreditBalance: 999999, // Dev key has unlimited credits conceptually, or check handles it
					}
				}
			}

			if cache != nil && repo == nil {
				// Only cache the boolean result in disabled mode for now
				_ = cache.CacheAuthResult(r.Context(), tokenHash, allowed, ttl)
			}

			if !allowed {
				sendAuthError(w, "invalid_api_key", "Invalid API key")
				return
			}

			ctx := context.WithValue(r.Context(), AuthIdentityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sendAuthError(w http.ResponseWriter, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errors.NewGatewayError(code, message, http.StatusUnauthorized))
}
