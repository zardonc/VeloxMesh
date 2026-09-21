---
phase: 27-stream-terminal-settlement-consistency
verified: 2026-09-21T18:15:44Z
status: gaps_found
score: 4/5 must-haves verified
covered_files:
  - .planning/REQUIREMENTS.md
  - .planning/ROADMAP.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-PHASE-BASE.txt
  - .planning/phases/27-stream-terminal-settlement-consistency/27-01-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-02-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-03-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-04-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-05-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-06-PLAN.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-01-SUMMARY.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-02-SUMMARY.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-03-SUMMARY.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-04-SUMMARY.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-05-SUMMARY.md
  - .planning/phases/27-stream-terminal-settlement-consistency/27-06-SUMMARY.md
  - internal/errors/errors.go
  - internal/errors/errors_test.go
  - internal/gateway/fusion.go
  - internal/gateway/fusion_terminal.go
  - internal/gateway/fusion_terminal_test.go
  - internal/gateway/service.go
  - internal/gateway/service_settlement.go
  - internal/gateway/service_settlement_test.go
  - internal/gateway/service_stream.go
  - internal/gateway/service_stream_terminal.go
  - internal/gateway/service_stream_test.go
  - internal/gateway/service_test.go
  - internal/gateway/stream_terminal.go
  - internal/gateway/stream_terminal_test.go
  - internal/http/handlers/chat.go
  - internal/http/handlers/chat_stream.go
  - internal/http/handlers/chat_stream_test.go
  - internal/http/handlers/chat_test.go
  - internal/providers/openai/adapter.go
  - internal/providers/openai/adapter_test.go
  - tests/integration/chat_stream_test.go
  - tests/integration/chat_test.go
covered_digest: "v1:sha256:2ba911a5dbf37520d6f2cb2dc7822a7dc00a8488fe88beaab01ed94256fe9c6e"
behavior_unverified: 1
overrides_applied: 0
gaps:
  - truth: "The Phase 27 full-suite validation gate completes successfully."
    status: failed
    reason: "go test -timeout 60s ./... cannot complete in the current environment. Existing tests require Redis (192.168.234.129:6379), PostgreSQL (192.168.234.129:5432), Qdrant (192.168.234.129:6334), and PyPI access for hatchling; all were unavailable. The Phase 27 plan explicitly requires recording this blocker instead of marking the full-suite gate passed."
    artifacts:
      - path: cmd/scheduler/main_test.go
        issue: "Python ONNX worker smoke test could not download hatchling from PyPI."
      - path: internal/app
        issue: "Existing tests could not connect to Qdrant or PostgreSQL."
      - path: tests/integration
        issue: "Existing Redis-backed tests could not connect to Redis."
    missing:
      - "Reachable Redis, PostgreSQL, and Qdrant test endpoints."
      - "Network access to PyPI, or a pre-provisioned hatchling dependency."
      - "A rerun of go test -timeout 60s ./... after the environment is provisioned."
behavior_unverified_items:
  - truth: "Focused benchmark or allocation-aware evidence proves that Phase 27 adds no material per-chunk throughput or latency regression."
    test: "Run a focused stream benchmark or allocation-aware test comparing the Phase 27 forwarding path with the recorded baseline under representative streaming load."
    expected: "No new per-chunk external I/O, storage read, lock-heavy work, or material allocation/throughput regression is observed."
    why_human: "Code inspection shows terminal callbacks are invoked only on finalization and the repeated-candidate tests pass, but no focused benchmark or allocation assertion was found to measure the runtime performance claim."
---

# Phase 27: Stream Terminal and Settlement Consistency Verification Report

**Phase Goal:** Every streaming request ends with one authoritative terminal outcome driving client output, provider health, circuit breaker, observability, admission release, and usage settlement consistently.
**Verified:** 2026-09-21T18:15:44Z
**Status:** `gaps_found` - partial / blocked, not phase-passed.
**Re-verification:** No - initial verification.

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Done/clean EOF succeeds; provider errors retain category; policy rejection and cancellation do not affect provider health. | VERIFIED | `classifyTerminal` maps terminal inputs in `stream_terminal.go`; `TestClassifyTerminal` passed for completed, provider-error, cancellation, policy-rejection, and abnormal cases. |
| 2 | Every terminal side effect runs at most once across ordinary, buffered, and Fusion streams. | VERIFIED | `terminalFinalizer.Submit` accepts only its first candidate and invokes lifecycle callbacks once; `TestTerminalFinalizerConcurrentSubmit`, `TestService_HandleChatCompletionStreamTerminalFirstCandidateWins`, and `TestFusionStreamTerminalUsesSharedLifecycleFinalizer` passed. |
| 3 | Only completed streams settle final usage; missing usage logs `missing_usage`; failure/cancellation do not debit. | VERIFIED | `settleTerminal` returns before persistence for non-completed outcomes and records `missing_usage` for completed nil usage; `TestService_SettleTerminal` selection passed. |
| 4 | SSE sends at most one terminal sequence; write failure stops forwarding and releases/cancels work. | VERIFIED | `chat_stream.go` checks every write, cancels its derived context on failure, and returns; handler cancellation/write-failure/one-DONE tests plus targeted integration compatibility tests passed. |
| 5 | Focused benchmark/allocation-aware evidence proves no material per-chunk regression. | PRESENT_BEHAVIOR_UNVERIFIED | Terminal work is finalizer-owned rather than chunk-owned, and repeated terminal-candidate tests pass, but no focused benchmark or allocation assertion exercises the runtime performance claim. |

**Score:** 4/5 truths verified (1 present, behavior-unverified).

The implemented terminal semantics are supported by focused tests. The phase is not marked passed because the required full-suite validation is externally blocked, and TERM-08 still lacks measured performance evidence.

## Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `internal/gateway/stream_terminal.go` | Immutable terminal outcome classifier and one-shot finalizer | VERIFIED | 176 substantive lines; classifier snapshots usage and `terminalFinalizer` serializes first-winner callback dispatch. |
| `internal/gateway/service_stream.go` | Ordinary and buffered stream lifecycle ownership | VERIFIED | 385 substantive lines; stream terminal state is used by normal and buffered completion paths. |
| `internal/gateway/fusion_terminal.go` | Thin Fusion lifecycle adapter | VERIFIED | 42 substantive lines; creates the shared `streamTerminalState` without modifying Fusion aggregation/routing semantics. |
| `internal/http/handlers/chat_stream.go` | SSE forwarding and disconnect handling | VERIFIED | 153 substantive lines; derived cancellable context and checked writes stop forwarding on disconnect. |
| `internal/gateway/service_settlement.go` | Outcome-gated usage settlement | VERIFIED | 106 substantive lines; persistence occurs only for `terminalCompleted` with final usage. |
| `internal/gateway/*_test.go`, `internal/http/handlers/chat_stream_test.go`, `tests/integration/chat_stream_test.go` | Terminal, settlement, handler, and compatibility regression coverage | VERIFIED | Named focused selections passed; no disabled Phase 27 tests or untracked TODO/FIXME/XXX markers found. |

## Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `internal/gateway/stream_terminal.go` | `internal/errors/errors.go` | terminal classification uses existing error category/health policy | WIRED | `verify.key-links` passed for Plan 27-01; code calls the shared error classification and health predicate. |
| `internal/gateway/service_stream.go` | `internal/gateway/stream_terminal.go` | shared terminal state completes through classifier/finalizer | WIRED | `verify.key-links` passed for Plan 27-02; `streamTerminalState.complete` submits the classified candidate. |
| `internal/http/handlers/chat.go` | `internal/gateway/service_stream.go` | handler invokes stream service with derived request context | WIRED | `verify.key-links` passed for Plan 27-03; `chat_stream.go` creates `context.WithCancel(r.Context())` and forwards it. |
| `internal/gateway/stream_terminal.go` | `internal/gateway/service_settlement.go` | terminal finalizer callback invokes settlement decision | WIRED | `verify.key-links` passed for Plan 27-04; `recordSettlement` calls `settleTerminal`. |
| Plans 27-05 and 27-06 | N/A | no plan-declared key links | N/A | `verify.key-links` reports no declared links; source and named Fusion/compatibility tests were inspected directly. |

## Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `service_stream_terminal.go` | final `Usage` snapshot | provider stream events written into stream terminal state | Yes | FLOWING - the completed terminal candidate carries the last observed usage value. |
| `service_settlement.go` | settlement record | classified `terminalOutcome` and final usage | Yes | FLOWING - calls `repo.Settle` only for a completed outcome with non-nil usage. |
| `chat_stream.go` | SSE terminal bytes | gateway stream events and terminal classification | Yes | FLOWING - handler emits provider error/DONE only on the supported terminal paths and exits on write failure. |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Terminal classification and concurrent first-winner finalization | `go test -timeout 60s ./internal/gateway -run '^(TestClassifyTerminal|TestTerminalFinalizerConcurrentSubmit)$'` | `ok veloxmesh/internal/gateway 0.231s` | PASS |
| Cancellation releases once without provider failure | `go test -timeout 60s ./internal/gateway -run '^TestService_HandleChatCompletionStreamTerminalCancellationReleasesOnce$'` | `ok veloxmesh/internal/gateway 0.234s` | PASS |
| Settlement only for completed final usage | `go test -timeout 60s ./internal/gateway -run '^TestService_SettleTerminal$'` | `ok veloxmesh/internal/gateway 0.249s` | PASS |
| Fusion uses the shared terminal finalizer | `go test -timeout 60s ./internal/gateway -run '^TestFusionStreamTerminalUsesSharedLifecycleFinalizer$'` | `ok veloxmesh/internal/gateway 0.252s` | PASS |
| SSE one-DONE, write failure, and request cancellation behavior | `go test -timeout 60s ./internal/http/handlers -run 'TestChatCompletionsStream(WritesOneDoneAfterCompletedStream|CancelsRequestAfterWriteFailure|StopsAfterTerminalWriteFailure|StopsAfterRequestCancellation)'` | `ok veloxmesh/internal/http/handlers 0.274s` | PASS |
| OpenAI-compatible stream/auth/cancellation compatibility | `go test -timeout 60s ./tests/integration -run 'Test(ChatCompletionsStream|ChatCompletions|OpenAI)'` | `ok veloxmesh/tests/integration 0.374s` | PASS |

An additional package-scoped command across gateway, handlers, OpenAI provider, and integration tests passed for the first three packages but failed only at the unrelated Redis-backed integration setup. The named Phase 27 integration selection above isolates and passes the relevant contract tests.

## Full-Suite Gate

| Command | Result | Status |
| --- | --- | --- |
| `go test -timeout 60s ./...` | Failed after Phase 27 packages passed: Redis at `192.168.234.129:6379`, PostgreSQL at `192.168.234.129:5432`, Qdrant at `192.168.234.129:6334`, and PyPI access for `hatchling` were unavailable. | BLOCKED - not passed |

This is an environment blocker, not evidence of a Phase 27 terminal-semantics regression. Per Plan 27-05, it remains a failed gate until the environment is provisioned and the exact full command passes.

## Probe Execution

No Phase 27 probe script was declared by the plans or found under the conventional `scripts/*/tests/probe-*.sh` location.

## Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| TERM-01 | 27-01, 27-02 | Classify terminal cause and preserve first-winner outcome. | SATISFIED | Classifier/finalizer source inspection and passing named gateway tests. |
| TERM-02 | 27-01, 27-02 | Exactly-once lifecycle effects across stream paths. | SATISFIED | Shared finalizer and passing concurrent/ordinary/Fusion tests. |
| TERM-03 | 27-01, 27-02 | Cancellation/write disconnect maps internally to client-cancelled and releases resources. | SATISFIED | Handler and service cancellation tests passed. |
| TERM-04 | Declared: 27-03; implemented: 27-04 | Completed final usage settles once; missing usage is diagnostic. | SATISFIED | Outcome-gated settlement source and test selection passed. |
| TERM-05 | Declared: 27-03; implemented: 27-04 | Failure/cancellation usage remains diagnostic and unsettled. | SATISFIED | `settleTerminal` early return and non-completed settlement cases passed. |
| TERM-06 | 27-02, 27-03 | No duplicate SSE terminal sequence or DONE. | SATISFIED | Handler one-DONE/write-failure tests passed. |
| TERM-07 | 27-04, 27-05 | Contract boundaries remain compatible. | SATISFIED | Targeted integration compatibility/auth/stream tests passed. |
| TERM-08 | 27-01, 27-02, 27-04, 27-05 | No new hot-path I/O or material stream performance regression. | PARTIAL | Source inspection shows terminal work is not per-chunk; measured benchmark/allocation evidence is absent. |

No requirement mapped to Phase 27 is orphaned from the Phase 27 plans.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| None | N/A | No unreferenced `TBD`, `FIXME`, or `XXX`; no Phase 27 production stub or hollow data-flow found. | Info | No source-level blocker found. |

## Human Verification Required

### Performance Evidence

**Test:** Run the focused stream benchmark or allocation-aware test described in `behavior_unverified_items` against the recorded baseline.

**Expected:** Terminal settlement/lifecycle work remains finalization-only, with no material per-chunk regression.

**Why human:** The available focused tests prove terminal correctness but do not measure allocation, throughput, or latency.

## Gaps Summary

Phase 27's terminal classification, one-shot lifecycle, cancellation/write-failure handling, usage settlement, SSE behavior, Fusion reuse, and targeted compatibility behavior are implemented and pass focused evidence. The phase cannot be marked passed: the prescribed full-suite command is blocked by existing unavailable Redis, PostgreSQL, Qdrant, and PyPI dependencies, and the performance criterion lacks a measured benchmark/allocation result. No production or test-code change is requested by this report.

---

_Verified: 2026-09-21T18:15:44Z_
_Verifier: the agent (gsd-verifier)_
