# Email Verification Login Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change dashboard auth so registration returns to login and login requires a six-digit email verification code before creating a session.

**Architecture:** Keep auth in the existing Go BFF and React auth form. Registration persists users without creating a session. Login validates password, creates a six-digit code, writes it to SMTP when configured or `tmp/email-outbox.log` in local dev, and a new verify endpoint creates the session cookie.

**Tech Stack:** Go `net/http`, React/TypeScript, Vitest, local file outbox fallback.

---

## Chunk 1: Backend Auth Flow

- [ ] Write failing Go tests for registration without session cookie, login challenge creation, wrong code rejection, and correct code session creation.
- [ ] Implement verification challenge state and six-digit code generation.
- [ ] Implement local email outbox fallback.
- [ ] Add `POST /bff/auth/verify-login`.
- [ ] Run `go test ./...`.

## Chunk 2: Frontend Auth Flow

- [ ] Update API types/tests for `loginAccount` challenge response and `verifyLoginCode`.
- [ ] Update `AuthView` so register switches back to login with a message.
- [ ] Update login UI to show a six-digit code step after password is accepted.
- [ ] Run `pnpm test` and `pnpm build`.

## Chunk 3: Verification

- [ ] Start local stack if needed.
- [ ] Register a new account and confirm the page returns to Login.
- [ ] Read the six-digit code from `dashboard/tmp/email-outbox.log`.
- [ ] Verify login and confirm dashboard opens only after the code.
