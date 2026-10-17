# Requirements: VeloxMesh Gateway

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

## Active Phase

- [ ] **CACHE-F01**: Remove fixed embedding assumptions and bound semantic-cache read/write latency, with default-off eligibility, version isolation, fault bypass, and full two-model gateway validation as specified in Phase 29 context.

Integration amendment, 2026-10-08: the user accepts the candidate budget in
[D-08](phases/29-semantic-cache-latency-hardening/29-CONTEXT.md) for merging the
default-off implementation and all branch commits. Latest 411-file source match,
613 backend PASS / 2 SKIP, 83 race PASS, 7 real-Redis PASS, 24 live cache/gateway
PASS and 36 windows / 3,600 successful requests support this scope. Two distinct
local embedding model lifecycles were recorded on 2026-10-03; they do not qualify
every online model or independent business answer quality. CACHE-F01 remains open
for the original 1.05 target, semantic quality/seed provenance, collection
reclamation, capacity and production activation. No production enablement is approved.

## Phase 30 Requirements — Request Budget and Safe Failover

**Updated:** 2026-10-17. Owner selected this direction and conservative failover. Existing per-attempt stage protection and limited fallback are implementation foundations, not missing features to rebuild. Phase 29 production remains partial after PR #31 merged into local main.

- [ ] **TIMEOUT-F01**: Unify request-wide and connection-stage deadlines with existing per-attempt protection and explicit safe-failover eligibility (Phase 30 umbrella).
- [ ] **BUDGET-01**: An optional ingress deadline covers cache, routing, admission, all attempts and response/stream completion, including bounded synchronous settlement/logging/cost I/O; earlier client deadlines win and exhausted requests start no new attempt. Phase 27 first-terminal ownership remains: cancellation/error never debit, completed-first settles once, and known Usage remains available for diagnosis.
- [ ] **CONNECT-01**: Connection and existing stage limits consume the remaining request budget; audit locked SDK/transport retries and prove actual upstream invocation bounds without upgrading dependencies.
- [ ] **FAILOVER-01**: Existing cross-provider fallback is bounded by remaining time and attempt count; ambiguous execution, client cancellation and started streams are never replayed, including tool traffic.
- [ ] **BUDGET-OBS-01**: Low-cardinality reasons and redacted replay evidence distinguish request exhaustion, attempt deadlines, permitted failover and replay refusal without payload or credential logging.
- [ ] **BUDGET-UAT-01**: Test-first failure cases and phase-end real local HTTP/SSE acceptance verify normal, tool, buffered/Fusion, cancellation, once-only terminal/settlement and permit cleanup under a hard 60-second backend test limit.

## Phase 31 Requirements — Operator Diagnostics Foundation

**Updated:** 2026-10-17. Owner selected Console prerequisites, with UI later; background probes are non-generating and generation checks require an explicit administrator action. Phase 30 remains unimplemented and is an execution prerequisite.

- [ ] **DIAG-F01**: Deliver trustworthy probe, timing and request-correlation data for a later Console without introducing a UI, new gateway protocols or a diagnostics platform.
- [ ] **PROBE-01**: Background probes perform only supported, bounded non-generating checks; distinguish configuration, reachability/authentication, unsupported/unknown and generation observations. Non-generating success cannot erase real-traffic failures or assert model availability.
- [ ] **PROBE-ISO-01**: Probe work has separate finite concurrency/rate/time bounds and cannot block user forwarding. Explicit manual generation checks use a registered provider/model, bounded output and controlled wire attempts, report potential provider cost/Usage separately, and never debit an unrelated tenant or enter its cache/scheduler path.
- [ ] **TIMING-01**: Define gateway-observed ingress-to-first and ingress-to-last successfully flushed semantic output, distinct from E2E and provider-attempt timing. Cover text/tools, ordinary/buffered/Fusion, write/flush failure, cancellation, empty responses and non-streaming nullable values without changing Phase 27 terminal/settlement ownership.
- [ ] **CORRELATION-01**: Generate one trusted internal request ID at ingress, correlate all attempts/trace/diagnostic records and return it to the caller without reusing external IDs as settlement keys. Validate and bound external IDs, retain only their safe correlation representation, and prevent duplicate external IDs from overwriting unrelated requests.
- [ ] **DIAG-QUERY-01**: Provide optional default-off capture into bounded node-local memory with TTL, entry/byte limits, nonblocking saturation handling and redacted allowlisted fields only. Admin-only, rate-limited bounded lookup/filtering must expose disabled/not-found/expiry/node-scope limits honestly; never store bodies, tool arguments, credentials or arbitrary upstream errors, and never use request IDs as metric labels.
- [ ] **DIAG-UAT-01**: Write failure cases before implementation; phase-end real local HTTP/SSE acceptance checks probes, timing, IDs, authorization/rate limits, redaction, saturation/TTL and isolation under an outer hard 60-second backend test ceiling plus Go's internal timeout, requiring exact named PASS and repeatable redacted artifacts.

## Future Requirements

- **CONSOLE-F01**: Add a lightweight control-plane Console for configuration and latency diagnosis.

## v7.9 Out of Scope

The historical table below describes the shipped Phase 27/28 protocol slice. Phase 30/31 additions are governed by their explicit requirements above; their Admin diagnostics and safe-failover work are not retroactive claims about that slice.

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
| CACHE-F01 | Phase 29 | Integration accepted under candidate budget; production partial |
| TIMEOUT-F01 | Phase 30 | Planned scope; per-attempt foundation already implemented |
| BUDGET-01 | Phase 30 | Pending |
| CONNECT-01 | Phase 30 | Pending |
| FAILOVER-01 | Phase 30 | Pending |
| BUDGET-OBS-01 | Phase 30 | Pending |
| BUDGET-UAT-01 | Phase 30 | Pending |
| DIAG-F01 | Phase 31 | Planned; blocked on Phase 30 implementation/verification |
| PROBE-01 | Phase 31 | Planned; pending implementation |
| PROBE-ISO-01 | Phase 31 | Planned; pending implementation |
| TIMING-01 | Phase 31 | Planned; pending implementation |
| CORRELATION-01 | Phase 31 | Planned; pending implementation |
| DIAG-QUERY-01 | Phase 31 | Planned; pending implementation |
| DIAG-UAT-01 | Phase 31 | Planned; pending implementation |

**Coverage:**

- v7.9 requirements: 9 total
- Mapped to phases: 9
- Unmapped: 0
- Complete: 9
- Separate active Phase 29 requirement: 1 mapped, 0 fully production-complete.
- Phase 30 requirements: 6 mapped, pending implementation and acceptance.
- Phase 31 requirements: 7 mapped, planning passed independent review; implementation waits for Phase 30.

---
*Requirements refreshed: 2026-10-17; historical v7.9 verification retained, Phase 29 production gates open, Phase 30/31 planned in dependency order.*
