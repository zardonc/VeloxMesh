---
phase: 2.4
plan: gaps
type: gap_closure
wave: 1
depends_on:
  - phase-2.4
files_modified:
  - internal/providers/anthropic/adapter_test.go
  - internal/providers/gemini/adapter_test.go
  - internal/providers/openai/adapter_test.go
  - internal/errors/errors.go
  - internal/providers/openai/adapter.go
  - internal/providers/anthropic/adapter.go
  - internal/providers/gemini/adapter.go
  - internal/gateway/service.go
  - internal/health/store.go
  - internal/http/handlers/health.go
  - internal/observability/metrics.go
  - tests/integration/health_test.go
autonomous: true
requirements_addressed:
  - PHASE-2.4-UAT-GAP-CATEGORY-ASSERTIONS
  - PHASE-2.4-UAT-GAP-FORMATTING
---

# Phase 2.4 Gap Closure Plan

<objective>
Close the two Phase 2.4 UAT gaps: make native provider error-category tests assert the exact shared `provider_*` contract, and restore a clean `gofmt` gate.
</objective>

<context>
Use `.planning/phases/02-health-aware-routing/02-04-UAT.md` as the source of failed checks.

Current verification state:
- `go test ./...` passes.
- `go vet ./...` passes.
- `gofmt -l .` lists many files.
- Anthropic/Gemini adapter implementations use shared category constants, but their tests do not yet assert exact `GatewayError.Code` values for error paths.
</context>

<tasks>

## 1. Strengthen Native Provider Error Category Tests

type: testing

files:
- `internal/providers/anthropic/adapter_test.go`
- `internal/providers/gemini/adapter_test.go`

action:
- For existing Anthropic auth and rate-limit test cases, assert the returned error is `*errors.GatewayError` and that `Code` equals `provider_auth_error` or `provider_rate_limit`.
- Add or extend Anthropic cases for empty content/no text content to assert `provider_bad_response` where practical with SDK fake server behavior.
- For existing Gemini auth and rate-limit test cases, assert the returned error is `*errors.GatewayError` and that `Code` equals `provider_auth_error` or `provider_rate_limit`.
- Add or extend Gemini cases for empty candidates/no text content to assert `provider_bad_response` where practical.
- Keep tests deterministic and fake-server based.

verify:
- `go test ./internal/providers/anthropic ./internal/providers/gemini`

acceptance_criteria:
- Native provider adapter tests fail if category strings drift away from the shared taxonomy.

## 2. Restore Formatting Gate

type: maintenance

files:
- all Go files listed by `gofmt -l .`

action:
- Run `gofmt -w` on Go files.
- Avoid semantic edits while formatting.

verify:
- `gofmt -l .` returns no files.

acceptance_criteria:
- Formatting check is clean.

## 3. Re-run Phase 2.4 Verification

type: verification

files:
- `.planning/phases/02-health-aware-routing/02-04-UAT.md`

action:
- Run:
  - `go vet ./...`
  - `go test ./...`
  - `gofmt -l .`
- Update `02-04-UAT.md` after fixes so both gaps are marked resolved or replace it with a passing verification record if following the project's UAT convention.

verify:
- `go vet ./...` passes.
- `go test ./...` passes.
- `gofmt -l .` returns no files.

acceptance_criteria:
- Phase 2.4 can be marked verified with 0 issues.

</tasks>

<success_criteria>
- Anthropic/Gemini tests assert exact shared provider error categories.
- Bad native provider responses are covered by tests where practical.
- `gofmt -l .` is empty.
- `go vet ./...` and `go test ./...` still pass.
</success_criteria>

## PLANNING COMPLETE
