# Phase 10: Advanced Routing & Observability - Validation

**Created:** 2026-06-30
**Source:** `10-RESEARCH.md` Validation Architecture
**Status:** Ready for execution

## Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` with current module |
| Config file | none |
| Quick run command | `go test ./internal/routing ./internal/health ./internal/controlstate ./internal/observability` |
| Full suite command | `go test ./...` |
| Timeout policy | Keep backend unit test commands under 60 seconds. |

## Requirement Test Map

| Req ID | Behavior | Test Type | Automated Command | Owner Plan |
|--------|----------|-----------|-------------------|------------|
| ROUT-01 | `composite-score` selects through the existing router and leaves default behavior unchanged. | unit | `go test ./internal/routing -run TestComposite` | 10-01 |
| ROUT-02 | Latency, pending requests, error rate, health penalty, and cost tie-break are applied in priority order. | unit | `go test ./internal/routing -run TestCompositeScoreSignals` | 10-01 |
| ROUT-03 | Z-score guard falls back for tiny/flat pools and stale metrics become neutral. | unit | `go test ./internal/routing -run TestCompositeNormalization` | 10-01 |
| OBS-01 | Traces include safe lifecycle attributes and exclude prompts, identity, and secrets. | unit/integration | `go test ./internal/observability ./internal/gateway -run TestTrace` | 10-04 |
| OBS-02 | Prometheus metrics expose histograms/counters with only approved low-cardinality labels. | unit | `go test ./internal/observability -run TestPrometheus` | 10-03 |

## Wave 0 Test Gaps

- [ ] `internal/routing/composite_test.go` covers ROUT-01, ROUT-02, and ROUT-03.
- [ ] `internal/controlstate/routing_config_test.go` covers config validation, migration, persistence, and last-known-good activation.
- [ ] `internal/observability/prometheus_test.go` covers OBS-02 and label allowlist behavior.
- [ ] `internal/observability/tracing_test.go` or gateway lifecycle tests cover OBS-01 sanitization and timing attributes.

## Phase Gates

1. Each plan adds or updates the smallest behavior-focused tests for its touched seam.
2. Per-task verification runs the narrow package tests for changed packages.
3. Per-wave verification runs `go test ./...` when it can complete within the required 60-second backend timeout.
4. Final verification includes an opt-in composite routing smoke, low-score fallback coverage, Prometheus label allowlist checks, and trace sanitization checks.

## Sanitization Checks

Observability tests must fail if exported spans, metric labels, or log-adjacent telemetry fields include raw prompts, request bodies, provider payloads, `Authorization`, API keys, user identifiers, or API-key identifiers. Safe generated request IDs may appear in traces only when they are not derived from user or credential identity.
