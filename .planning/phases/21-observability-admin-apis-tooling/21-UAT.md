---
status: testing
phase: 21-observability-admin-apis-tooling
source:
  - 21-01-SUMMARY.md
  - 21-02-SUMMARY.md
  - 21-03-SUMMARY.md
started: 2026-07-06T20:53:07Z
updated: 2026-07-06T20:53:07Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

number: 1
name: Scheduler Status Partial Runtime Visibility
expected: |
  An authenticated admin can call GET /admin/v1/scheduler/status and receive queue depth, executor slot usage, rollout status, circuit breaker state, quality rollups, and runtime warnings. If optional runtime data is unavailable, the endpoint still returns the available fields with explicit warnings instead of failing the whole request.
awaiting: user response

## Tests

### 1. Scheduler Status Partial Runtime Visibility
expected: An authenticated admin can call GET /admin/v1/scheduler/status and receive queue depth, executor slot usage, rollout status, circuit breaker state, quality rollups, and runtime warnings. If optional runtime data is unavailable, the endpoint still returns the available fields with explicit warnings instead of failing the whole request.
result: [pending]

### 2. SLA Rules Read and Successful Replacement
expected: An authenticated writable admin can GET /admin/v1/scheduler/sla-rules to see the active runtime rules, then PUT a valid full replacement set and immediately observe the new rules in subsequent reads. The successful replacement emits scheduler.sla_rules.replace audit evidence with safe metadata only.
result: [pending]

### 3. SLA Rules Invalid Replacement Rejection
expected: When an authenticated writable admin PUTs an invalid SLA rule set, the API returns a clear validation failure, the existing runtime rules remain unchanged, and no sensitive submitted payload is written to audit metadata.
result: [pending]

### 4. Scheduler Training Export Safe Formats
expected: GET /admin/v1/scheduler/training-samples/export returns safe training data as JSON by default and NDJSON when requested. Valid limit, time, and task filters narrow the export, while the response exposes only whitelisted features and labels and omits task IDs or raw payload-like fields.
result: [pending]

### 5. Scheduler Training Export Validation Failures
expected: Invalid export requests such as out-of-range limits, malformed time filters, or unsupported formats are rejected with clear errors and do not return partial raw training records.
result: [pending]

### 6. Exact Semantic Neighbor Hydration Across Vector Backends
expected: Semantic-neighbor lookup hydrates only the exact scheduler training sample IDs returned by vector search, preserves vector result order, and omits missing IDs. The same behavior is verified for both Qdrant and pgvector-backed vector results.
result: [pending]

### 7. Semantic Neighbor Embedding Model Configuration
expected: If no embedding model is configured, semantic neighbors use text-embedding-3-small. If scheduler.semantic_neighbors_embedding_model or SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL is set, the configured model is passed into SemanticNeighborService without changing the vector backend behavior.
result: [pending]

### 8. Narrow Heuristic Override File Handling
expected: The scheduler accepts a heuristic override file containing only base_latency and model_multipliers, applies those values at startup, and rejects unknown top-level fields with a clear error instead of silently accepting unsupported tuning knobs.
result: [pending]

### 9. Non-Empty SchedulerType Quality Attribution
expected: Quality evidence recorded from heuristic, FIFO, gRPC, weighted, and metadata fallback scheduling paths always includes a non-empty SchedulerType so observability rollups can attribute scheduler behavior correctly.
result: [pending]

## Summary

total: 9
passed: 0
issues: 0
pending: 9
skipped: 0
blocked: 0

## Gaps

[none yet]
