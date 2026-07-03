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
	Enabled       bool
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
				if cachedIdent, err := cache.GetCachedIdentity(r.Context(), tokenHash); err == nil && cachedIdent != nil {
					if cachedIdent.Enabled {
						identity = &AuthIdentity{
							ID:            cachedIdent.ID,
							Role:          cachedIdent.Role,
							CreditBalance: cachedIdent.CreditBalance,
							Enabled:       cachedIdent.Enabled,
						}
						// Fast path return via cache
						ctx := context.WithValue(r.Context(), AuthIdentityKey, identity)
						next.ServeHTTP(w, r.WithContext(ctx))
						return
					} else {
						sendAuthError(w, "invalid_api_key", "Invalid API key")
						return
					}
				}
			}

			allowed := false

			if cfg.DevAPIKey != "" && token == cfg.DevAPIKey {
				allowed = true
				identity = &AuthIdentity{
					ID:            "dev-key",
					Role:          "admin",
					CreditBalance: 999999, // Dev key has unlimited credits conceptually
					Enabled:       true,
				}
			} else if repo != nil && repo.APIKeys() != nil {
				if keyRecord, err := repo.APIKeys().GetByHash(r.Context(), tokenHash); err == nil && keyRecord != nil {
					if keyRecord.Enabled {
						allowed = true
						identity = &AuthIdentity{
							ID:            keyRecord.ID,
							Role:          keyRecord.Role,
							CreditBalance: keyRecord.CreditBalance,
							Enabled:       keyRecord.Enabled,
						}
					}
				}
			}

			if cache != nil && identity != nil {
				// Cache the identity envelope safely
				_ = cache.CacheIdentity(r.Context(), tokenHash, &hotstate.CachedIdentity{
					ID:            identity.ID,
					Role:          identity.Role,
					Enabled:       identity.Enabled,
					CreditBalance: identity.CreditBalance,
				}, ttl)
			} else if cache != nil && !allowed {
				// Cache the negative result safely
				_ = cache.CacheIdentity(r.Context(), tokenHash, &hotstate.CachedIdentity{
					Enabled: false,
				}, ttl)
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
