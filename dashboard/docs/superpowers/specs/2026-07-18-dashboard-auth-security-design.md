# Dashboard Authentication Security Design

## Objective

Harden Dashboard authentication, verification delivery, sessions, API key handling, and authorization for production use without changing the existing login or Customer/Admin navigation contract.

## Security Boundaries

- Production mode never returns a verification code, writes a verification code to a local outbox, or proceeds without a complete SMTP configuration.
- Demo and test delivery are available only when `DASHBOARD_DEMO_MODE=true` or `DASHBOARD_TEST_MODE=true`.
- Verification codes are generated with rejection sampling, stored only as SHA-256 hashes, expire after five minutes, are single-use, and are invalidated after five failed attempts.
- Verification requests are rate limited by normalized email, client IP, and challenge. Responses do not reveal whether an account exists.
- SMTP credentials are read from runtime environment variables or `SMTP_PASSWORD_FILE`; local env files may contain non-secret SMTP connection metadata but not a production password.
- SMTP delivery requires authenticated TLS with certificate verification. Port 465 uses implicit TLS; other ports require STARTTLS before authentication.
- Session cookies are `HttpOnly`, `SameSite=Lax`, have a bounded lifetime, and are `Secure` in production. Logout deletes both server state and the browser cookie.
- API key plaintext is returned only by a successful create operation. Persisted state contains a hash and a safe prefix/mask, never the full key.
- Customer requests to `/bff/admin/*` remain forbidden, and unauthenticated requests remain unauthorized.

## Components

### Verification Security

`auth_security.go` owns challenge hashing, constant-time verification, attempt accounting, rate-limit buckets, client-IP normalization, and security response helpers. `server.go` keeps endpoint orchestration and persists only user/account state.

Rate limits are intentionally process-local for this single-instance Capstone deployment. The implementation exposes clear configuration values so a Redis-backed limiter can replace it for a multi-replica deployment.

### Secure Mail Delivery

`smtp_sender.go` owns TLS SMTP delivery behind a small interface so tests can use a recording sender without opening a network connection. Production startup validation rejects partial SMTP configuration and missing credentials. Demo/test delivery retains the current outbox behavior.

### Session Policy

Configuration includes production/test mode, cookie security, and session lifetime. Production defaults to a Secure cookie. Tests and local HTTP E2E explicitly enable test mode or disable Secure cookies.

### Secret Loading

The BFF entry point resolves `SMTP_PASSWORD_FILE` before `SMTP_PASSWORD`, trims trailing newlines, and never logs the resulting value. Docker Compose mounts an optional secret file only through deployment-specific overrides; the repository example documents the variable without containing a secret.

## Error Handling

- Missing production SMTP returns `503 verification_delivery_unavailable` before creating or exposing a usable challenge.
- Login uses a generic authentication response and a generic verification-delivery response.
- Rate-limited requests return `429` with a stable error code and `Retry-After`.
- Invalid, expired, consumed, and exhausted challenges use the same public verification failure message.
- SMTP failures return `503` and remove the newly created challenge.

## Verification

Go tests cover five-minute expiry, hash-only storage, replay, five-attempt lockout, email/IP/challenge limits, production SMTP rejection, Demo/Test-only codes, TLS configuration, Secure cookies, logout invalidation, API key persistence, and secret-free Audit/Settings responses. Playwright continues to prove Customer/Admin isolation and login/logout behavior. A repository scan checks generated responses and artifacts for verification codes and full API key values.

## Known Deployment Constraint

The process-local rate limiter is correct for the current single BFF instance. A future horizontally scaled deployment must move limiter and challenge state to Redis or another shared store.
