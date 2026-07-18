package bff

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	verificationLifetime    = 5 * time.Minute
	verificationMaxAttempts = 5
	defaultRateLimitWindow  = 15 * time.Minute
)

type rateLimitBucket struct {
	count   int
	resetAt time.Time
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateLimitBucket
}

func newFixedWindowLimiter() *fixedWindowLimiter {
	return &fixedWindowLimiter{buckets: map[string]rateLimitBucket{}}
}

func (limiter *fixedWindowLimiter) allow(key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	if limit <= 0 {
		return true, 0
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if len(limiter.buckets) > 4096 {
		for bucketKey, bucket := range limiter.buckets {
			if !now.Before(bucket.resetAt) {
				delete(limiter.buckets, bucketKey)
			}
		}
	}
	bucket, ok := limiter.buckets[key]
	if !ok || !now.Before(bucket.resetAt) {
		limiter.buckets[key] = rateLimitBucket{count: 1, resetAt: now.Add(window)}
		return true, 0
	}
	if bucket.count >= limit {
		return false, bucket.resetAt.Sub(now)
	}
	bucket.count++
	limiter.buckets[key] = bucket
	return true, 0
}

type verificationResult int

const (
	verificationInvalid verificationResult = iota
	verificationAccepted
	verificationExpired
	verificationExhausted
	verificationConsumed
)

func newLoginChallenge(id string, username string, code string, pepper []byte, now time.Time) loginChallengeDTO {
	return loginChallengeDTO{
		ID:        id,
		Username:  username,
		CodeHash:  hashVerificationCode(id, code, pepper),
		ExpiresAt: now.Add(verificationLifetime),
	}
}

func (challenge *loginChallengeDTO) verify(code string, pepper []byte, now time.Time) verificationResult {
	if challenge.Consumed {
		return verificationConsumed
	}
	if challenge.FailedAttempts >= verificationMaxAttempts {
		return verificationExhausted
	}
	if now.After(challenge.ExpiresAt) {
		return verificationExpired
	}
	expected := hashVerificationCode(challenge.ID, code, pepper)
	if subtle.ConstantTimeCompare([]byte(challenge.CodeHash), []byte(expected)) != 1 {
		challenge.FailedAttempts++
		if challenge.FailedAttempts >= verificationMaxAttempts {
			return verificationExhausted
		}
		return verificationInvalid
	}
	challenge.Consumed = true
	return verificationAccepted
}

func hashVerificationCode(challengeID string, code string, pepper []byte) string {
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte(challengeID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (server *Server) allowVerificationSend(w http.ResponseWriter, r *http.Request, email string) bool {
	checks := []struct {
		key   string
		limit int
	}{
		{key: "send:email:" + normalizeRateKey(email), limit: server.config.VerificationSendEmailLimit},
		{key: "send:ip:" + clientAddress(r), limit: server.config.VerificationSendIPLimit},
	}
	return server.allowVerificationChecks(w, checks)
}

func (server *Server) allowVerificationAttempt(w http.ResponseWriter, r *http.Request, email string, challengeID string) bool {
	checks := []struct {
		key   string
		limit int
	}{
		{key: "verify:email:" + normalizeRateKey(email), limit: server.config.VerificationVerifyEmailLimit},
		{key: "verify:ip:" + clientAddress(r), limit: server.config.VerificationVerifyIPLimit},
		{key: "verify:challenge:" + normalizeRateKey(challengeID), limit: verificationMaxAttempts},
	}
	return server.allowVerificationChecks(w, checks)
}

func (server *Server) allowVerificationChecks(w http.ResponseWriter, checks []struct {
	key   string
	limit int
}) bool {
	for _, check := range checks {
		allowed, retryAfter := server.verificationLimiter.allow(check.key, check.limit, server.config.VerificationRateWindow, server.now())
		if allowed {
			continue
		}
		seconds := int(retryAfter.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error":   "verification_rate_limited",
			"message": "Too many verification requests. Try again later.",
		})
		return false
	}
	return true
}

func clientAddress(r *http.Request) string {
	remote := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return normalizeRateKey(host)
	}
	if remote == "" {
		return "unknown"
	}
	return normalizeRateKey(remote)
}

func normalizeRateKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
