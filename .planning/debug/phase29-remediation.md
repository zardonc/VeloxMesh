---
status: awaiting-evidence
trigger: "尝试修复 Phase 29 遗留问题并验证；修复前验证本地 VM 组件性能"
created: 2026-10-03
updated: 2026-10-03
---

## Symptoms

Expected: cache writes become readable only after indexing; dependency faults have bounded latency and visible errors; scenario-specific performance and correctness evidence precede release.
Actual: persistence/health/SQLite/stream defects have passing scoped regressions. Current low-hit ratios1.140/1.190 still exceed original1.05, independent .92 recall is0/12, and prefix/.84 has3/20 unsafe hits. Scenario release SLOs and embedding backend/first-concurrent-tail evidence remain open.
Reproduction: existing real Phase 29 test runner, VM Redis/Qdrant/PostgreSQL, local SQLite and actual model APIs. No mock services or replies.

## Current Focus

hypothesis: remaining low-hit costs include real embedding round trips; recall depends on representation/threshold calibration. Nemotron long waits occur after upstream headers, outside gateway processing.
test: completed six verified direct/off/on/memo blocks, actual C4/C8/hit scenarios, independent semantic holdouts, memo/configuration/fault boundaries, healthy GPT protocol/business checks, host independent-arrival experiment and sanitized backend timing extraction. Earlier Redis/lifecycle/pgvector/two-model evidence is retained as history.
expecting: scoped code defects have passing real regressions; retain failed release gates and upstream resource evidence.
next_action: obtain a business-approved scope/recall contract, a safe independent semantic policy, explicit per-scenario release SLOs and embedding backend queue/inference timing. Six verified blocks, host independent-arrival experiment, backend timing extraction and configuration boundaries are complete. HTTP429/known-resource Providers are skipped without clearing historical failures; no quota retry needed this round. Do not enable production cache or merge main on current evidence.

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

## Follow-up evidence (baseline f19f316)

- Real memo regression first failed because the second identical query made another embedding HTTP request. Added default-disabled bounded per-service vector memo with scope/model isolation, copied vectors, TTL/LRU eviction and visible error handling; real boundary, cancelled-context, disconnect/recovery and 100-hit billing tests pass.
- Explicit input prefix participates in a representation version in scope; empty prefix restores the old scope. Actual write/hit/prefix-switch/restore and two-seed billing pass. No default prefix or production threshold was changed.
- Independent, frozen multi-FAQ set: collection/raw/.92 and nomic/raw/.92 each retrieve 0/12 same-answer questions and have 0/20 unsafe negative hits. Nomic/search_query/.84 retrieves 4/12 positives but falsely hits 3/20 negatives (eighth-day refund, seven-day trial, fourteen-day refund). Seen-set calibration did not generalize. Nominal recall targets remain diagnostic proposals, not approved business contracts.
- Actual GPT availability, SSE/buffered/Fusion/cancellation, all selected tool modes/continuation, direct and gateway business facts pass. OR/Gemini/SANS not dispatched because of authorized resource exclusions.
- VM fresh capacity: Redis/Qdrant/PostgreSQL and SQLite windows have zero errors. SQLite C16 P99 39.113ms/max90.677ms remains a saturation boundary. Model API identifies collection as nomic-bert-moe Q8_0 and Nomic as nomic-bert Q4_K_M, both 768 returned dimensions; API does not expose effective task prefix/pooling/inference queue timing.
- Test-harness corrections: nonexistent test selection had incorrectly returned PASS; runner now requires a named PASS marker. First three stable windows did not activate memo because stdin environment reset test parameters; these windows and interrupted dispatches are preserved and excluded from optimized results. Corrected wrappers set options after environment loading, log effective profile, and analysis requires four observed memo hits per stable window.
- Final follow-up: 2,400 formal requests succeed; exact-hit P95 36.511→6.444ms, embedding calls101→1 including seed, only seed billed. Normal application P95 off/on/memo5.665/5.221/4.981ms. Low-hit ratios1.140005/1.189665 fail original1.05, pass proposed1.25/+40ms; joint block bootstrap does not establish memo low-hit P95 improvement. C8 miss application P95 10.718ms, no approved saturation SLO. Two normal residual>15ms samples retained.
- Actual model host independent-arrival windows:300 successful samples; no gateway/SSH involved, co-load embedding P99101.456ms. LM Studio C8-window logs show193 parseable tasks, four observed simultaneous slots, decode P95245.07ms/total271.19ms; this supports model generation/scheduling as upstream contributors, not a configured slot-limit assertion. No per-request gateway ID correlation or embedding inference timing available.
- Full details: ../phases/29-semantic-cache-latency-hardening/29-FOLLOWUP-RESULTS-20261003.md. Build/vet, cache/config/gateway regressions and eight real invalid-config startup cases pass; isolated containers/tunnels stopped. Production cache and input threshold remain unchanged; no main merge.

## Resolution

root_cause: SQLite pooled connections lacked per-connection PRAGMAs; cache persistence enabled SQL before confirmed indexing and had no terminal cleanup; health replication held a global lock during unbounded I/O and admitted late stale publications; stream parser incorrectly required an empty choices array for all final accounting frames.
fix: per-connection SQLite DSN settings; pending-to-active cache persistence with stable semantic point IDs and bounded conditional cleanup; bounded per-key health synchronization and atomic ordered Redis snapshots; restricted OpenRouter accounting-choice normalization.
verification: scoped real regressions passed, build/vet and existing focused package regressions passed. Latest 58/61 cases pass; semantic positive diagnosis, mixed-load Provider timeout and direct HTTP429 remain. Original low-hit P95 release gate fails separately. Complete evidence in ../phases/29-semantic-cache-latency-hardening/29-REMEDIATION-RESULTS-20261003.md.
files_changed: internal/controlstate/sqlite/connections.go; internal/cache/persistence.go and cleanup.go; internal/controlstate/{sqlite,postgres}/cache_cleanup.go; internal/storage/qdrant_cache.go; internal/health/redis_{store,snapshots,publication}.go; internal/hotstate/{redis,ordered_snapshots}.go; internal/providers/openai/{adapter,stream}.go; gateway health timing helper; opt-in real test harnesses and replay artifacts. Working changes remain reviewable; no merge.
