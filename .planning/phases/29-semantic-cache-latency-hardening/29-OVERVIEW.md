# Phase 29 Implementation Plan - Semantic Cache Latency Hardening

**Status:** Planned, not authorized for implementation. **Requirement:** `CACHE-F01`. **Dependency:** Phase 28 verified. **Milestone name/version:** To be assigned separately; this phase is not part of completed v7.9.

## Modules And Stages

| Module / plan | Goal and scope | Priority | Key tasks | Depends on | Estimate |
| --- | --- | --- | --- | --- | --- |
| 29-01: eligibility, configuration, isolation | A safe non-streaming FAQ/document vertical path; cache default off; server-side allowlist; explicit embedding provider/model/dimension; versioned tenant/application/model/settings key; zero-Usage hit. Excludes tool/stream/multimodal/structured/live/personalized/sensitive/high-stakes/exact/contextual traffic. | P0 | Resolve trusted identity/version contract; write red negative fixtures; extend existing config and gateway/cache interfaces; validate dimensions and cached answer; preserve normal response and settlement. | Phase 28; pre-development confirmations A-D | 1.5-2 engineer-days |
| 29-02: bounded lookup and asynchronous store | Remove cache I/O from response completion, constrain lookup tail latency, bypass immediately on dependency failure. | P0 | Measure baseline; set configurable short read budget and bounded worker capacity; stop SQLite fallback after vector fault; immutable queued write and shutdown policy; reason-only metrics/logging; fault-injection tests. | 29-01; measured baseline | 2-2.5 engineer-days |
| 29-03: release evidence | Prove correctness and latency under representative traffic and both real embedding models. | P0 release gate | Deterministic gateway E2E and negative set; same-load low-hit P95 comparison; two real-model full gateway runs; redacted repeatable evidence and rollback check. | 29-02; isolated test environment | 1.5-2 engineer-days plus external validation time |

**Critical path:** 29-01 -> 29-02 -> 29-03. Approximately 5-6.5 engineer-days after the open contracts and environment are ready; not a calendar commitment. No deployment is included.

## Business And Interface Contracts To Confirm Before Development

| ID | Decision required | Proposed default / acceptance contract | Owner or gate |
| --- | --- | --- | --- |
| A | Which tenant/application use cases are allowlisted, and who may change the list? | Explicit server-side allowlist for static non-sensitive FAQ and versioned document Q&A only; unlisted requests bypass. Do not infer eligibility from text similarity or a client header. | Product/security owner before 29-01 |
| B | What is the authoritative knowledge version and publication boundary? | Publisher/control-plane owns an immutable version per corpus; atomically activate new version before serving new content. Reader scope changes immediately; TTL only cleans old entries. | Content owner + gateway contract before 29-01 |
| C | What is the cache authorization partition? | Use authenticated API-key identity plus trusted use-case/tenant boundary, unless a stronger tenant identifier is already guaranteed by the auth layer. Never share across authorization scopes. | Auth owner before 29-01 |
| D | Which generation settings and request shapes may be cached? | Only a fixed, explicitly approved single-turn text profile. Temperature/max-tokens and any answer-affecting settings enter the key or trigger bypass; unknown/unrepresented request fields never silently become cache eligible. | API owner before 29-01 |
| E | How are embedding configuration, dimensions and cache validity bounds selected? | Manual provider/model/dimension selection; reject invalid enabled configuration and wrong returned vector length; model or dimension switch uses a new isolated collection/scope. Confirm TTL, similarity threshold and maximum candidates against the approved corpus rather than retaining fixed application constants. Validate both existing `.env.local` models without exposing values. | Platform owner before 29-01/real test |
| F | What are timeout, queue, concurrency, and shutdown bounds? | Measure baseline first, then record bounded values in configuration and runbook. Timeout or queue saturation bypasses/drops optional cache work; never blocks or fails primary response. | SRE/platform owner before 29-02 |

No new public API or client-controlled cache selector is assumed. If the trusted use-case/version information does not exist, 29-01 stops at its decision checkpoint; development must not substitute a free-form request header. Validate config format and any internal metadata propagation across HTTP -> gateway -> cache, plus backward compatibility with the existing `/v1/chat/completions` response, `X-Cache-*` headers, and zero-token cache-hit Usage.

## Acceptance Gate

1. Default-off and all excluded traffic cause no embedding/vector I/O. Eligible requests return ordinary primary responses, then become available as valid paraphrase hits without settling nonexistent upstream Usage.
2. Cross-tenant/auth, use-case, knowledge version, target model, embedding provider/model/dimension, and answer-affecting settings cannot share hits. Curated near-identical/different-answer negatives yield **zero false hits**; positive paraphrase hit rate is reported, not gated by an invented percentage.
3. Embed, vector, repository, malformed-entry, timeout and queue faults bypass immediately to one normal primary-model call; no cache-caused 5xx, reason-only telemetry, and no payload leakage. Background write cannot hold response completion.
4. Matched-load low-hit non-streaming P95 complete-response latency with cache enabled is <= 1.05 x disabled. Preserve raw samples, concurrency, workload, hit ratio and environment. Streaming TTFT is not used for this phase.
5. Both real `.env.local` embedding models each pass a full gateway run: manual switch, primary response, async persistence, paraphrase hit, model/version isolation and forced cache fault bypass against the actual configured vector store. Repeatable redacted artifact required before completion.

## Explicit Non-Goals

No general retries/timeouts redesign, Console, provider expansion, MCP/Agent runtime, semantic reuse for excluded request classes, exact-cache subsystem without measured need, deployment, or routine live-provider CI.
