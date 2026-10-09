# Phase 29 comprehensive verification

Scope: verify the current Redis ordered-publication fix and its application callers.
Keep original Formal07 and followup evidence immutable. No production change.

## Coverage and sequence

1. Verify current source/binary identity and stopped isolated services. Run the full
   backend suite with real Redis/PostgreSQL/Qdrant and a hard 60-second process-tree
   deadline. Report skips separately. Run affected race and static checks.
2. Add only missing real-Redis publication coverage: default 50 ms timeout,
   concurrent same-key publication, recovery, provider/model/probe stale ordering.
   Reuse existing read-failure, routing, isolation and cache lifecycle tests.
3. Run real local-model cache integration checks before performance: embedding reuse,
   memo boundaries/recovery, cache replay/expiry/restart, exact isolation/safety,
   health routing recovery and settlement. Stop each batch on a failure and diagnose.
4. After prerequisites, run three matched blocks with 100 requests/window, 12
   windows/block (3,600 measured requests), 125 ms arrivals and concurrency ceiling 4.
   Cover semantic pure miss, fixed 4% hits, exact miss, semantic/exact same-question
   hits and bypass, each with the appropriate before/after off baselines. Freeze
   order and assert profiles/hit counts before interpreting timing. Retain every
   measured window; no opportunistic replacement. This is expanded regression
   verification, not the old 12-block/43,200-request formal acceptance.
5. Audit failed operations, source identity, resource observations and cleanup;
   report limitations and any newly confirmed application defect.

## Decision rules

- Semantic miss/low-hit: P95 <=1.25 times both off endpoints and delta <=40 ms.
- Exact miss: P95 delta <=10 ms; hit: P95 <=60 ms and <=0.5 times both endpoints.
- Application residual: P95 <=10 ms, P99 <=15 ms. Report original 1.05 separately.
- Compare each block and pooled scenarios; report drift and small-sample limits.
- A failed candidate metric does not imply a hardware physical limit or justify
  changing the gate. Review raw stages first. Functional failures block expansion.
- No external paid-provider load, global priority changes, production enablement,
  container/volume deletion or historical evidence overwrite.

Risks checked: counter loss, stale overwrite, fail-open health reads, hidden
replication errors, cross-provider blocking, duplicate embedding, failed-call
amplification, cache leakage across scope/version, billing, bypass and cleanup.
