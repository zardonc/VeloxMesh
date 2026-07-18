# Customer Tenant Authentication Design

## Goal

Allow customers to create an account, verify their email, sign in, and view only their own tenant's real gateway usage, requests, and API keys. Public registration must never create an Admin account.

## Existing Constraints

- Keep the existing React, TypeScript, Vite, and Go `net/http` application.
- Preserve all current Admin pages and `/bff/admin/*` contracts.
- Persist identity and API key metadata in the existing JSON state file.
- Read operational request data from the existing Redis-backed `operationalStore`.
- Do not substitute mock data when a live Customer endpoint is empty or unavailable.
- Existing state files without the new fields must continue to load.

## Identity Model

- `Tenant`: stable generated ID, organization name, owner username, quota, and status.
- `User`: generated user ID, email, username, role, tenant ID, verification state, password hash, and scopes.
- `Session`: opaque random token mapped server-side to user ID, tenant ID, role, and expiry.
- `APIKey`: generated ID, tenant ID, masked value, SHA-256 secret hash, scope, status, creation time, and last-used value.
- `RequestLog`: existing Redis row with `tenant`; Customer APIs filter this field against the authenticated Session tenant ID.

For backward compatibility, existing Admin users without a user ID receive a stable ID after loading. Existing Admin accounts may have no tenant ID. Public Customer registration always creates a new tenant and assigns it to the new user.

## Registration And Verification

`POST /bff/auth/customer/register` accepts `email`, `username`, `organization`, `password`, and `confirmPassword`. The server ignores role and tenant fields and always creates a Customer. Under one state lock it validates uniqueness, prepares Tenant/User records, persists one new state snapshot, and then publishes the in-memory state. A persistence failure leaves neither record visible.

The response contains a verification challenge, not a Session. Development mode may return `devCode`; SMTP mode never does. `POST /bff/auth/verify` consumes the challenge, marks the user verified, creates a Session, and returns the authenticated identity. Existing `/bff/auth/login` and `/bff/auth/verify-login` remain as compatible aliases. Public `/bff/auth/register` accepts Customer only and rejects Admin; Admin users are created by the existing state file or deployment bootstrap, not by a public HTTP endpoint.

Passwords use bcrypt. Legacy SHA-256 password hashes remain verifiable and are upgraded to bcrypt on a successful login.

## Session And Authorization

The browser receives only an opaque HttpOnly, SameSite=Lax cookie. The server-side Session contains `user_id`, `tenant_id`, `role`, and `expires_at`. Session identity is the only source of role and tenant authorization. URL, query, header, and JSON `tenant_id` values cannot grant access.

- Missing or expired Session: `401`.
- Authenticated wrong role: `403`.
- Duplicate email or username: `409`.
- Validation error: `422`.
- Rate limit: `429`.

Sessions remain in memory and are intentionally invalidated by a BFF restart. User, tenant, and API key records persist in the JSON state file.

## Customer API

- `GET /bff/customer/summary`
- `GET /bff/customer/usage`
- `GET /bff/customer/requests`
- `GET /bff/customer/api-keys`
- `POST /bff/customer/api-keys`
- `DELETE /bff/customer/api-keys/{id}`

Each route requires a Customer Session. Summary and Usage are computed from real filtered operational logs. Requests support bounded pagination plus optional status/model/time filters. Empty tenants receive zero metrics and empty arrays. API key lists return only masked values. Creation returns the complete key exactly once; persistence stores only its hash and mask. Deletion checks tenant ownership and uses a non-revealing `404` for another tenant's key.

## Frontend Flow

The existing login surface exposes three clear actions: Customer Sign In, Create Customer Account, and Admin Sign In. Admin has no registration UI. Customer registration leads directly to the six-digit verification step. App startup calls the BFF Session endpoint and trusts only the returned role and tenant.

Customer Home, Usage, My Requests, and My API Keys call only `/bff/customer/*`. They implement Loading, Empty, Error, No Permission, and Partial Data states. An API key secret is shown once after creation and is not written to storage or the console.

## Testing

Go tests prove registration uniqueness, Admin registration rejection, verification, expiry, logout, 401/403 behavior, Session tenant immutability, Customer A/B data isolation, pagination, empty data, API key one-time display, and cross-tenant revocation denial. Frontend tests prove endpoint mapping, no Admin calls from Customer pages, registration flow, real-data empty/error behavior, and masking. Final verification runs Go tests, Vitest, TypeScript/Vite build, Playwright at 1440x900, 1024x768, and 390x844, plus HTTP security probes.

