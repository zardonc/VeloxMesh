---
phase: "29"
slug: "semantic-cache-latency-hardening"
status: partial
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-25"
updated: "2026-10-08"
integration_status: accepted_candidate_budget
---

# Phase 29 - Validation Strategy

Current integration uses the owner-accepted candidate D-08 budget, with production
cache off. Latest evidence and limitations are in [29-VERIFICATION.md](29-VERIFICATION.md).
Original release targets below remain separate from the accepted integration gate.

## Test Infrastructure

| Property | Value |
| --- | --- |
| Framework | Go `testing`, existing gateway/cache/integration packages |
| Quick run | `go test -timeout 60s ./internal/cache ./internal/gateway ./internal/config` |
| Full backend suite | `go test -timeout 60s ./...` |
| Live run | Phase-end only, controlled test environment using both `.env.local` embedding configurations |

## Sampling Rate

- Write failure cases and a red test before production changes for each plan.
- Start with focused offline tests after implementation changes; broaden only when results or changed scope warrant it. Reuse retained results when source hashes still match.
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

- [ ] Production A-D and E-F choices are approved before activation; isolated test choices already allowed implementation and validation.
- [x] Latest backend: 613 PASS, 2 explicit SKIP, 0 FAIL; relevant race 83 PASS. All backend invocations use internal and outer hard 60-second limits; source manifest matches 411 current files (2026-10-08).
- [ ] Negative false hits = 0; positive hit rate reported.
- [x] Local-model functional acceptance rerun on 2026-10-01: one paraphrase hit, two different-answer negatives missed, same-key version isolation, embedding fault forwarding and queue burst passed; see `29-VERIFICATION.md`.
- [ ] P95 ratio <= 1.05 under matched low-hit non-streaming load.
- [x] Owner-accepted integration budget: nine scenario/interval checks pass in 36 windows / 3,600 successful requests; semantic miss/low-hit ≤1.25×each off AND delta≤40ms, exact miss delta≤10ms, hits ≤60ms AND ≤0.5×each off, residual P95≤10/P99≤15ms (2026-10-08).
- [x] Two distinct real local embedding model lifecycles recorded on 2026-10-03; this does not qualify every online model or independent semantic business quality.
- [x] No credentials or raw runtime request/answer payloads in tracked evidence; model IDs/dimensions and explicit review candidates are retained as requested.

2026-10-01: `29-EVIDENCE.md` and raw JSONL record repaired real application-path measurements, but SANS upstream daily quota stopped successful repeats. Offline oracle negatives pass; real-model quality, sustained capacity, final parameters and the 1.05 gate are not signed off. Production remains disabled.

Local-model continuation: `29-LOCAL-MODEL-MEASUREMENT.md` records 1044 timed client attempts and a red-to-green real negative case. The user-selected 768-dimensional model passes one paraphrase, two different-answer negatives, same-key `faq-v1` -> `faq-v2` isolation, embedding fault fallback and a real queue burst. Both available corrected same-schedule P95 ratios fail (1.143 / 1.511), with primary timeouts and a missing warmup run; sustained capacity and final numeric approval remain open. This is one real model, so the two-model checkbox remains unchecked. Full backend and focused race checks pass after the shared embedding-input repair. No phase completion or production approval is implied.

2026-10-01 acceptance-scope amendment: the user explicitly accepts local-model functional verification for this round because online embedding quota prevents load testing, and requests report-only handling for nonfunctional/environment issues. The new `29-UAT.md` session is complete within that scope: 12 functional checks pass, 3 release/performance items are deferred, no new functional failure. Full backend rerun passes 592 top-level tests (two unrelated opt-in tests skipped), focused race passes, and all three selected real local-model runners pass. Original performance/two-model release checkboxes remain unchecked; deferral is not a passing measurement or permission to enable production caching.
