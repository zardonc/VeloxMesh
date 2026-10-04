---
status: awaiting-evidence
trigger: "尝试修复 Phase 29 遗留问题并验证；修复前验证本地 VM 组件性能"
created: 2026-10-03
updated: 2026-10-03
---

## Symptoms

Expected: cache writes become readable only after indexing; dependency faults have bounded latency and visible errors; scenario-specific performance and correctness evidence precede release.
Actual: partial writes leave enabled SQL rows; Redis health writes hold a global lock and ignore errors; low-hit full-response P95 ratio remains 1.220; semantic recall needs independent evaluation.
Reproduction: existing real Phase 29 test runner, VM Redis/Qdrant/PostgreSQL, local SQLite and actual model APIs. No mock services or replies.

## Current Focus

hypothesis: remaining low-hit costs include real embedding round trips; recall depends on representation/threshold calibration. Nemotron long waits occur after upstream headers, outside gateway processing.
test: completed six balanced direct/off/on blocks, actual C4/C8 scenarios, Redis delayed/stale publications, real cache lifecycle, pgvector/PostgreSQL component checks, two embedding lifecycles and raw input-prefix comparisons.
expecting: scoped code defects have passing real regressions; retain failed release gates and upstream resource evidence.
next_action: obtain Provider body-generation/queue evidence after HTTP 429 clears, and model-service effective-input/model-version metadata plus an independent semantic gold set and scenario SLOs before another release decision. Do not enable production cache or merge main on current evidence.

## Failure cases recorded before implementation

- SQL commit followed by Qdrant failure exposes an entry that was never indexed.
- Qdrant succeeds but repository activation fails; restart leaves stale points/rows.
- A timed-out remote write can finish later; compensation must preserve retry evidence and never produce a readable partial entry.
- Global health mutex amplifies one delayed Redis command into unrelated provider stalls.
- Moving network writes outside the lock can reorder snapshots and lose newer counters.
- Redis context deadlines are ineffective if socket deadlines do not follow context; errors must be visible.
- Reading remote snapshots can overwrite newer local state during concurrent requests.
- Embedding input adaptation can mix old/new representations unless policy participates in scope identity.

## Evidence

- timestamp: 2026-10-03
  source: live read of referenced chat and 29-REMEDIATION-PLAN-20261003.md; baseline HEAD b2eb95ff.
  finding: no production modification in analysis chat; existing scenario evidence retained; production cache remains disabled.
- timestamp: 2026-10-03
  source: measurements/remediation-20261003/component-baseline and regression-before.
  finding: Redis/Qdrant/PostgreSQL direct C1/4/8/16 calls succeeded; SQLite C4/C8/C16 produced SQLITE_BUSY. New pooled connections had foreign_keys=0, busy_timeout=0, synchronous=2. Driver per-connection DSN configuration removes failures in 2000 real write/read samples; saturated SQLite tails remain measurable.
- timestamp: 2026-10-03
  source: measurements/remediation-20261003/targeted-after.
  finding: all seven targeted checks passed. Enabled=0 before vector acknowledgement; failed rows reaped after dependency recovery. Redis post-provider wait reduced from 202ms to 52-54ms; unrelated provider wait reduced from 200ms to 5.49ms; real C8 snapshots retain all 200 successes.
- timestamp: 2026-10-03
  source: measurements/remediation-20261003/lifecycle-after.
  finding: real vector replay produced one point after three writes; TTL rejects expired entries and removes both SQL/vector data. Application restart test captured real cleanup errors in logs but failed because it watched the previous application's metrics recorder; observer corrected, original failure retained.

## Final verification evidence

- completed_evidence: measurements/remediation-20261003/final-boundaries and pg-and-bypass-final.
  finding: eight boundary checks passed, including timed-out commands arriving late, exact success counters, vector replay, application restart and failed cleanup retry. PostgreSQL conditional cleanup and 4000 pgvector write/search samples passed. Restart was App reconstruction inside one OS process, not forced OS termination.
- completed_evidence: measurements/remediation-20261003/stream-direct, protocol-after and final-canonical-stream.
  finding: actual OpenRouter repeats finish_reason in a content-free final usage choice, matching official documentation; the adapter rejected it as provider_bad_response. Narrow accounting-frame normalization repaired stream settlement, Fusion and tool streaming/continuation. Actual GPT standard SSE also passed.
- completed_evidence: measurements/remediation-20261003/evaluation.json and mixed-final.
  finding: 1800 stable requests succeeded; application P95 off/on 4.774/4.950ms, full-response ratio 1.2226467 fails 1.05. C8 bypass application tail remains 15.098ms P95. One mixed Nemotron request spent 11999.778ms in Provider and about 5.058ms elsewhere; direct warmup returned 429 before matched samples were available.
- completed_evidence: measurements/remediation-20261003/final-supplemental and supplemental-profile.
  finding: two real embedding models pass separate-scope lifecycle and restore old hits. collection positive recall remains 2/50 with zero false hits in 100 negatives. Nomic clustering prefix at threshold .86 gives 49/50 positives but 18/100 false hits; no production input or threshold changed.

## Eliminated

- hypothesis: VM network hop is already proven the main stable embedding bottleneck.
  evidence: matched warmed Go client host/VM C4 differed by about 1ms in previous real tests; cold windows remain separate.

## Resolution

root_cause: SQLite pooled connections lacked per-connection PRAGMAs; cache persistence enabled SQL before confirmed indexing and had no terminal cleanup; health replication held a global lock during unbounded I/O and admitted late stale publications; stream parser incorrectly required an empty choices array for all final accounting frames.
fix: per-connection SQLite DSN settings; pending-to-active cache persistence with stable semantic point IDs and bounded conditional cleanup; bounded per-key health synchronization and atomic ordered Redis snapshots; restricted OpenRouter accounting-choice normalization.
verification: scoped real regressions passed, build/vet and existing focused package regressions passed. Latest 58/61 cases pass; semantic positive diagnosis, mixed-load Provider timeout and direct HTTP429 remain. Original low-hit P95 release gate fails separately. Complete evidence in ../phases/29-semantic-cache-latency-hardening/29-REMEDIATION-RESULTS-20261003.md.
files_changed: internal/controlstate/sqlite/connections.go; internal/cache/persistence.go and cleanup.go; internal/controlstate/{sqlite,postgres}/cache_cleanup.go; internal/storage/qdrant_cache.go; internal/health/redis_{store,snapshots,publication}.go; internal/hotstate/{redis,ordered_snapshots}.go; internal/providers/openai/{adapter,stream}.go; gateway health timing helper; opt-in real test harnesses and replay artifacts. Working changes remain reviewable; no merge.
