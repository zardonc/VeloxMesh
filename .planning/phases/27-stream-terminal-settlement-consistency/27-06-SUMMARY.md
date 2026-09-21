---
phase: 27-stream-terminal-settlement-consistency
plan: 06
status: complete
date: 2026-09-21
---

# Plan 27-06 Summary

## Delivered

- Replaced the prior Fusion terminal regression with a focused shared-contract test in the `gateway` package.
- Covered Fusion completed, provider-error, client-cancelled, policy-rejected, and internal-abnormal terminal outcomes.
- For every case, asserted one admission release and that only completed final Usage produces a single settlement record.
- Submitted a late cancellation after the primary candidate in each case to prove Fusion cannot re-run lifecycle or settlement side effects.

## Verification

All commands used the writable temporary `GOCACHE` and a 60-second timeout.

- `go test -timeout 60s ./internal/gateway -run 'TestFusion.*Terminal|TestFusion.*Settlement|TestFusion.*Duplicate'`
- JSON-selected gateway verification emitted passing events for `TestFusionTerminalUsesSharedOutcomeContract` and existing Fusion terminal first-winner coverage.
- `go test -timeout 60s ./internal/gateway ./internal/http/handlers ./internal/providers/openai`
- `git diff-index --check cd61c3afa9f9b20e62f54467fa449eaf75f6d242 --`

## Scope

This work package changes only `internal/gateway/fusion_terminal_test.go` and this evidence file. It does not change Fusion aggregation, route selection, member execution, provider routing, or Judge behavior. No real-provider Fusion soak test was run; that remains an optional environment-dependent follow-up.
