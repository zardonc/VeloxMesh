---
phase: "29"
slug: "semantic-cache-latency-hardening"
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-25"
---

# Phase 29 - Validation Strategy

## Test Infrastructure

| Property | Value |
| --- | --- |
| Framework | Go `testing`, existing gateway/cache/integration packages |
| Quick run | `go test -timeout 60s ./internal/cache ./internal/gateway ./internal/config` |
| Full backend suite | `go test -timeout 60s ./...` |
| Live run | Phase-end only, controlled test environment using both `.env.local` embedding configurations |

## Sampling Rate

- Write failure cases and a red test before production changes for each plan.
- Run focused offline tests after each implementation task; run the full backend suite after each plan wave.
- Run the expensive full gateway E2E, matched-load benchmark, and two-model live flow at the end only.
- Each backend test invocation has a hard 60-second timeout; a long E2E scenario is split into bounded invocations, not allowed to run unbounded.

## Per-Task Verification Map

| Plan | Requirement / decisions | Evidence |
| --- | --- | --- |
| 29-01 | CACHE-F01; D-01 to D-05 | Config/handler/gateway/cache integration tests for default-off, bypasses, version/scope/settings/model isolation, valid hit and zero Usage |
| 29-02 | CACHE-F01; D-06 to D-07 | Fault and concurrency integration tests, response-before-write timing, reason-only observability, bounded shutdown |
| 29-03 | CACHE-F01; D-08 to D-12 | Curated negative/positive set, matched-load P95 artifact, two real-model gateway-flow artifact |

## Wave 0 Requirements

- Existing `internal/cache/semantic_test.go`, `internal/gateway/service_tool_test.go`, `internal/config/config_test.go`, and `tests/integration/semantic_cache_test.go` provide the test harness. Add only behaviorally necessary cases, red-first.
- Record baseline method and fixtures before measuring any performance threshold; no arbitrary timeout or queue size is asserted in this planning document.

## Manual-Only Verifications

| Behavior | Why manual | Evidence |
| --- | --- | --- |
| Trusted allowlist/version publishing contract | Depends on product and control-plane ownership | Signed-off choices A-D in `29-OVERVIEW.md` before 29-01 code |
| Two real embedding models through gateway | External credentials and vector store are excluded from routine CI | Redacted repeatable run artifact, no `.env.local` values |
| P95 release gate | Requires controlled matched load and environment | Raw samples, ratio, workload and hit ratio |

## Validation Sign-Off

- [ ] A-D and E-F are resolved before the tasks they block.
- [ ] Offline tests pass with `-timeout 60s` and include boundary/error paths.
- [ ] Negative false hits = 0; positive hit rate reported.
- [ ] P95 ratio <= 1.05 under matched low-hit non-streaming load.
- [ ] Both real embedding configurations pass full gateway-flow validation.
- [ ] No secret/config values or raw request/answer payloads in tracked evidence.
