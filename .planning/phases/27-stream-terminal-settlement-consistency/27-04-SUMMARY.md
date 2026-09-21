---
phase: 27-stream-terminal-settlement-consistency
plan: 04
status: complete
---

# Plan 27-04 Summary

## Delivered

- Moved settlement code from `service.go` into `service_settlement.go`.
- Routed stream settlement through the accepted shared terminal outcome rather than a separate stream branch.
- Limited all settlement persistence to `completed` outcomes. Provider errors, policy rejection, client cancellation, and internal abnormal outcomes preserve only in-memory diagnostic usage.
- Recorded one `missing_usage` usage record for completed outcomes without final Usage and never called `Settle` for that case.
- Replaced ignored settlement, Usage-log, and cost-aggregation errors with sanitized `slog` warnings that contain only operation, terminal kind, provider, model, and a stable error class.
- Preserved cost aggregation sequencing: it executes only after successful settlement and only when credits are available.
- Added deterministic fakes and tests for debit/no-debit paths, one-shot terminal settlement, callback order, release after a persistence failure, and sanitized observability.

## Verification

- `go test -timeout 60s ./internal/gateway -run 'Test.*Settlement|Test.*Usage'`
- `go test -timeout 60s ./internal/gateway`
- `go test -timeout 60s ./internal/errors ./internal/gateway ./internal/http/handlers`
- `git diff --check`

All Go commands used the writable temporary `GOCACHE` directory.

## Scope

No billing formula, pricing record, retry behavior, persistence schema, or client response was changed. `27-05` will exercise the combined lifecycle and settlement matrix.
