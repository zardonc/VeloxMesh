# Requirements: VeloxMesh v7.9 Gateway Protocol Correctness

**Defined:** 2026-09-20
**Core Value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## v7.9 Requirements

### Stream Terminal Semantics

- [x] **TERM-01**: Gateway classifies stream completion as exactly one of completed, provider error, client cancelled, response-policy rejected, or abnormal internal termination, with the first terminal outcome winning.
- [x] **TERM-02**: Ordinary streaming, buffered streaming, and Fusion streaming run provider health, circuit-breaker, metrics, trace, admission release, and usage finalization at most once for every request.
- [x] **TERM-03**: Client cancellation and downstream write disconnection are recorded internally as `499/client_cancelled`, release request resources promptly, and do not count as provider-health failures.

### Usage and Settlement

- [x] **TERM-04**: A completed stream settles the final observed Usage exactly once; completed streams without Usage record `missing_usage`; provider errors, policy rejection, and client cancellation never debit the client.
- [x] **TERM-05**: Usage received before a failed or cancelled terminal outcome remains diagnostic and unsettled, without being promoted to a successful settlement.

### Protocol and Regression Coverage

- [x] **TERM-06**: SSE output emits no duplicate terminal error or `[DONE]` marker for Done, EOF, provider error, cancellation, or downstream write failure.
- [x] **TERM-07**: Contract tests cover ordinary, buffered, and Fusion paths across normal, error, boundary, authorization, compatibility, and cancellation scenarios without changing the existing OpenAI-compatible endpoint contract.
- [x] **TERM-08**: The finalization design adds no external I/O or storage lookup to the per-chunk hot path and shows no material regression in focused stream throughput/latency verification.

### Tool Calling Protocol

- [x] **TOOL-F01**: Complete internal and provider mappings for `tools`, `tool_choice`, tool-call fragments, and `tool_call_id`.

## Future Requirements

- **CACHE-F01**: Remove fixed embedding assumptions and bound semantic-cache read/write latency.
- **TIMEOUT-F01**: Unify connection, first-byte, stream-idle, and total-duration timeouts plus retry eligibility.
- **CONSOLE-F01**: Add a lightweight control-plane Console for configuration and latency diagnosis.

## Out of Scope

| Feature | Reason |
|---------|--------|
| New public API or request schema | v7.9 completes existing protocol semantics without introducing a new endpoint. |
| New provider or adapter | Provider expansion follows contract correctness. |
| Retry-policy redesign | Retry eligibility requires a separate timeout/idempotency phase. |
| Database migration or new settlement status | Existing usage repository contracts are sufficient for this hardening slice. |
| Console or control-plane UI | Control-plane work is deferred until protocol correctness is stable. |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| TERM-01 | Phase 27 | Verified |
| TERM-02 | Phase 27 | Verified |
| TERM-03 | Phase 27 | Verified |
| TERM-04 | Phase 27 | Verified |
| TERM-05 | Phase 27 | Verified |
| TERM-06 | Phase 27 | Verified |
| TERM-07 | Phase 27 | Verified |
| TERM-08 | Phase 27 | Verified |
| TOOL-F01 | Phase 28 | Verified |

**Coverage:**

- v7.9 requirements: 9 total
- Mapped to phases: 9
- Unmapped: 0
- Complete: 9

---
*Requirements verified: 2026-09-25*
