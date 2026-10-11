---
phase: 29-semantic-cache-latency-hardening
plan: "02"
subsystem: gateway-cache
tags: [go, bounded-cache, async-writes, qdrant, gemini]
status: complete
completed: 2026-10-01
requires:
  - phase: 29-01
    provides: trusted FAQ eligibility and versioned scope
provides:
  - Total lookup deadline and finite admission
  - Nonblocking immutable write queue with independent timeout and bounded close
  - Native Gemini embedding and valid Qdrant collection names
  - Correct durable-provider startup order and bounded reason metrics
affects: [29-03]
requirements-completed: []
key-decisions:
  - User authorized offline Wave 2 development without final live baseline approval.
  - All enabled cache bounds must be explicitly configured; production remains off.
  - Stable provider quota and final numeric approval remain 29-03 gates.
coverage:
  - id: D-06
    requirement: CACHE-F01
    description: Optional dependency, malformed-vector and queue failures bypass/drop with enumerated reasons.
    verification:
      - kind: unit
        ref: internal/cache/semantic_bounds_test.go
        status: pass
      - kind: unit
        ref: internal/cache/semantic_vector_fault_test.go
        status: pass
  - id: D-07
    requirement: CACHE-F01
    description: Reads are deadline/concurrency bounded and foreground HTTP responses do not wait for writes.
    verification:
      - kind: integration
        ref: tests/integration/semantic_cache_test.go#TestSemanticCache_CacheHeaders
        status: pass
      - kind: unit
        ref: internal/cache/semantic_bounds_test.go#TestSemanticCacheCombinedReadWriteCapacity
        status: pass
---

# Plan 29-02: Offline bounded cache implementation

The real application now initializes its cache after durable providers are ready, uses the native embedding interfaces and valid Qdrant names, bounds foreground reads, and queues optional writes without delaying the primary response. This closes the offline implementation; it does **not** approve a release configuration or finish CACHE-F01.

## Implementation commits

- `1d866f1` — native Gemini embedding, adapter capability/conformance coverage and real-model smoke runner.
- `7712471` — real startup/collection repairs; total read deadline, finite concurrency, immutable write queue, independent write timeout, finite close, reason metrics, config validation and measurement tools.

## Authorization and numeric checkpoint

The user's 2026-10-01 continuation explicitly permits offline Wave 2 while the second-model final acceptance or production publisher owner is missing. The earlier final-parameter checkpoint is deferred, not silently approved. Explicit isolation candidates are 100 ms read / four reads / two workers / queue 32 / 2 s write timeout / 1 s shutdown grace. See `29-EVIDENCE.md` for their limited basis, loss-of-hit tradeoff, drop policy and owner.

## Red-first and verification evidence

- Startup test reproduced `provider registry not ready` before provider activation was moved earlier.
- Native embedding and collection-name tests failed before implementation.
- Invalid embedding/vector errors and missing async queue were reproduced before bounded behavior was added; zero/malformed numeric config failed before validation was added.
- HTTP slow-write test failed with the old synchronous `Store` call, then passed with `Enqueue` while write embedding was deliberately held.
- Malformed vector result, provider identity sharing and mutable trusted-profile cases each reproduced their defect before fixes.
- Full `go test -timeout 60s ./...` passes with SSH-forwarded isolated Redis, Qdrant and PostgreSQL. The integration package completed in approximately 16 seconds.
- Focused race checks and `go vet -tags phase29preflight` pass. Existing protocol/stream bypass and zero-usage hit behavior remain covered.

## Measured outcome and unresolved work

Two configured SANS models returned real 1024/2048-dimensional vectors. A real native Gemini call returned 3072 dimensions. Repaired `App.New()` baseline runs retained 256 client attempts and separate operation timings, but both SANS models exhausted upstream `free-models-per-day` quota. Cache-on P95 ratios were approximately 1.407/1.406, with mismatched achieved load and extensive lookup/store failures; no stable capacity or final parameters are inferred.

`29-03` remains incomplete: restore sustainable external quota, run both actual model gateway lifecycles and semantic negatives, repeat a committed-build steady-state baseline, tune parameters, prove the 1.05 gate and obtain human review. No production cache, allowlist, publisher or account payment setting was changed.
