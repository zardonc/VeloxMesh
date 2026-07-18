# Dashboard Authentication Security Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Dashboard verification, SMTP delivery, sessions, API keys, and Admin authorization safe for production deployment.

**Architecture:** Keep HTTP endpoint orchestration in the existing BFF and move security primitives into focused Go modules. Production mode is fail-closed; Demo/Test mode is the only path that exposes a development verification code or writes an outbox.

**Tech Stack:** Go `net/http`, `crypto/rand`, `crypto/sha256`, `crypto/tls`, SMTP protocol, React/Vitest, Playwright, Docker Compose.

---

## Chunk 1: Verification Challenges And Rate Limits

### Task 1: Hashed, bounded challenges

**Files:**
- Create: `dashboard/internal/bff/auth_security.go`
- Test: `dashboard/internal/bff/auth_security_test.go`
- Modify: `dashboard/internal/bff/server.go`

- [ ] Add failing tests for hash-only storage, five-minute expiry, one-time use, constant-time comparison, and lockout after five failures.
- [ ] Run `go test ./internal/bff -run 'TestVerification' -count=1` and confirm the new tests fail for missing behavior.
- [ ] Implement challenge hashing, attempt accounting, expiry, consumption, and unbiased six-digit generation.
- [ ] Re-run the focused tests and existing registration/login tests.

### Task 2: Email, IP, and challenge limits

**Files:**
- Modify: `dashboard/internal/bff/auth_security.go`
- Test: `dashboard/internal/bff/auth_security_test.go`
- Modify: `dashboard/internal/bff/server.go`

- [ ] Add failing tests for normalized-email send limits, trusted client-IP limits, challenge verify limits, `429`, and `Retry-After`.
- [ ] Implement a mutex-protected fixed-window limiter with bounded cleanup.
- [ ] Make login failures generic and avoid account-enumeration differences.
- [ ] Run focused auth tests and confirm green.

## Chunk 2: Production SMTP And Session Policy

### Task 3: Fail-closed delivery and TLS SMTP

**Files:**
- Create: `dashboard/internal/bff/smtp_sender.go`
- Test: `dashboard/internal/bff/smtp_sender_test.go`
- Modify: `dashboard/internal/bff/server.go`
- Modify: `dashboard/cmd/gateway/main.go`
- Test: `dashboard/cmd/gateway/main_test.go`

- [ ] Add failing tests proving production without SMTP returns `503`, never emits `devCode`, never writes outbox, and removes failed challenges.
- [ ] Add failing config tests for `SMTP_PASSWORD_FILE`, partial SMTP configuration, and production startup validation.
- [ ] Implement implicit TLS and required STARTTLS delivery with certificate verification.
- [ ] Restrict outbox and `devCode` to explicit Demo/Test mode.
- [ ] Run BFF and command-package tests.

### Task 4: Secure, expiring sessions

**Files:**
- Modify: `dashboard/internal/bff/auth_security.go`
- Modify: `dashboard/internal/bff/server.go`
- Test: `dashboard/internal/bff/auth_security_test.go`
- Test: `dashboard/web/admin-console/e2e/dashboard.spec.ts`

- [ ] Add failing tests for production `Secure`, `HttpOnly`, `SameSite`, configured lifetime, expiry, and logout deletion.
- [ ] Implement the policy and ensure local E2E uses explicit TestMode/non-Secure cookies.
- [ ] Run focused Go auth tests and permission E2E.

## Chunk 3: Secret Handling, Documentation, And Acceptance

### Task 5: API key and response secret contracts

**Files:**
- Test: `dashboard/internal/bff/auth_security_test.go`
- Modify: `dashboard/internal/bff/server.go` only if a failing test exposes plaintext persistence or response leakage.

- [ ] Add tests proving create-only plaintext, hash/prefix persistence, masked lists, and secret-free Audit/Settings/exports.
- [ ] Add Customer `/bff/admin/*` table tests and preserve `403` behavior.
- [ ] Run focused tests and the complete Dashboard Go suite.

### Task 6: Deployment configuration and documentation

**Files:**
- Modify: `dashboard/.env.example`
- Modify: `dashboard/docker-compose.yml`
- Modify: `dashboard/README.md`

- [ ] Document SMTP host/port/from/username/password-file, TLS mode, TestMode, cookie security, and rate-limit settings without real credentials.
- [ ] Add a Docker Secret example that does not require committing a password.
- [ ] Run sensitive-value scans, `git diff --check`, Go race tests, frontend tests/build, and the complete Playwright suite.
- [ ] Record external deployment work: real SMTP host, DNS/certificate behavior, and Docker Secret must be verified in the target server environment.

No commits or pushes are performed unless the user explicitly requests them.
