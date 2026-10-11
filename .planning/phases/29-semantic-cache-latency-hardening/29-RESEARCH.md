# Phase 29: Semantic Cache Latency Hardening - Research

**Researched:** 2026-09-25
**Scope:** Repository-grounded planning research only; no source changes or live-provider calls.

## User Constraints

The following are locked decisions copied from `29-CONTEXT.md`:

- **D-01:** Semantic caching is disabled by default. Enable it only for explicitly allowlisted tenant/application use cases involving static, non-sensitive FAQ or versioned document Q&A where paraphrases may safely reuse an answer.
- **D-02:** Requests involving tools, streaming, images/files/audio, structured output, high-freshness data, personalization, sensitive content, high-stakes advice, exact calculations/quotations, or answer-relevant multi-turn context bypass semantic reuse. A text-only request is not automatically eligible.
- **D-03:** A cache hit must not cross tenant/authorization scope, target model, answer-affecting prompt or generation settings, or knowledge/document version. Publishing a new FAQ/document version must immediately stop matching old entries; TTL is only a fallback expiry control.
- **D-04:** Cache hits retain the existing zero-token Usage behavior and do not debit tokens for an upstream call that did not happen. Record hit and estimated savings separately, without logging prompts, cached answers, credentials, or other sensitive payloads.
- **D-05:** The embedding model is manually configurable; no embedding model name may be hardcoded in cache business logic. The semantic cache remains off by default. Provider/model selection and vector dimension must be coherent, including when switching between models with different dimensions.
- **D-06:** Cache lookup, embedding, vector search, or background write failures, timeouts, and overload must immediately degrade to ordinary primary-model forwarding. Cache faults must not add main-request 5xx responses. A failed or full write queue may drop cache writes; record only a low-cardinality reason.
- **D-07:** The disabled cache path adds no external I/O. Enabled semantic reads have an independent short budget; cache writes must not delay successful response completion. Choose the concrete budget, queue bounds, and shutdown behavior during research and planning, then verify them.
- **D-08:** In a controlled, same-load, low-hit-rate non-streaming benchmark, cache-enabled P95 complete-response latency must be at most 1.05 times cache-disabled P95. Streaming TTFT is not the primary metric because this phase does not cache streaming requests.
- **D-09:** A human-labeled negative set containing near-identical questions with different correct answers, cross-tenant requests, and cross-version requests must produce zero erroneous hits. Report positive paraphrase hit rate, but do not invent a percentage gate before representative workload data exists.
- **D-10:** Failure-injection evidence must show immediate bypass for embedding, vector-store, and write-queue faults, no cache-caused primary-request 5xx, and a recorded reason without sensitive content.
- **D-11:** Both real embedding models already configured in `.env.local` must complete a controlled, full gateway-flow validation at phase end: manual model switch, primary response and cache write, paraphrased request hit, model/version isolation, and fault bypass. Use the actual configured vector-store path. Do not copy `.env.local` values into plans, logs, fixtures, or commits.
- **D-12:** Deterministic local tests remain the routine merge gate; real-provider calls are phase-end validation, not per-commit CI. Backend test commands have a hard 60-second timeout. Record a repeatable, redacted artifact with commands, environment shape, model identifiers, observed result, and latency comparison.

Agent discretion copied from `29-CONTEXT.md`:

- Decide whether an existing exact-key cache can be reused ahead of semantic lookup; do not invent a second cache subsystem solely for this phase.
- Select the smallest configuration and storage changes that enforce eligibility, version isolation, model/dimension compatibility, and bounded latency while preserving existing deployment modes.
- Set specific cache-read and worker limits from measured baselines rather than arbitrary defaults; document them before implementation.

Deferred ideas copied from `29-CONTEXT.md`:

- General connect/first-byte/stream-idle/total timeout and retry-policy redesign.
- Console, full capability catalog, new providers or endpoints, MCP, and Agent orchestration.
- Semantic caching for streaming, tool, multimodal, structured-output, personalized, sensitive, or time-critical requests.

## Standard Stack

| Component | Existing route | Planning implication |
| --- | --- | --- |
| Gateway | `internal/gateway/service.go:103-357` [VERIFIED] | Keep cache pre-routing lookup and post-settlement candidate creation inside the existing non-streaming service path. Do not change Phase 27 settlement ownership. |
| Cache | `internal/cache/semantic.go:17-180` [VERIFIED] | Extend `SemanticCacheService`; do not introduce another cache stack. `Lookup` and `Store` currently call embedding synchronously. |
| Configuration | `internal/config/config_types.go:130-139`, `internal/config/config_file.go:241-246`, `internal/config/config_validation.go:76-99` [VERIFIED] | Extend the current nested cache config plus legacy ENV mapping; enabled mode must validate provider, model, dimension, and time budgets together. |
| Vector stores | `internal/storage/interfaces.go:29-44`, `internal/app/semantic_cache.go:15-105` [VERIFIED] | Use the existing adapter interface and configured Qdrant, LanceDB, or pgvector route. No new vector dependency is justified. |
| Provider | `internal/providers/openai/adapter.go:482-535` [VERIFIED] | `Embed` accepts `EmbeddingRequest.Model`; cache code currently ignores this configurability. The two real models can be selected without a new provider adapter if each is supported by its configured provider. |
| Persistence | `internal/controlstate/sqlite/repository.go:865-946`, `internal/controlstate/postgres/repository.go:824-902` [VERIFIED] | Candidate queries already filter scope, model, enabled and expiry. Prefer an opaque composite scope for version/embedding identity if it can be introduced without an unsafe migration. |

## Architecture Patterns

1. **Trusted eligibility boundary.** [VERIFIED: `internal/http/handlers/chat.go:15-47,59-143`, `internal/llm/types.go:63-87`] The public chat request has model, messages, temperature, max tokens, stream, and tools. `Message.MultiContent` is omitted from `json.Marshal(req.Messages)`. Therefore eligibility must inspect the typed request before serialization and reject multimodal, tool, multi-turn, and unknown answer-affecting options from cache participation. An allowlist must be selected by trusted server-side identity/use-case configuration, not an untrusted request label. The exact trusted application/version source remains a pre-development contract decision.
2. **One answer-validity identity.** [VERIFIED: `internal/cache/semantic.go:59-99,120-180`] Today candidate identity is scope plus target model, and Qdrant collection is derived from those two values. Plan a stable, opaque identity over authorization scope, allowlisted use case, knowledge version, target model, answer-affecting settings, embedding provider/model, and dimension. Both repository lookup and vector collection must use the same identity; old entries should become unreachable immediately after version or embedding switch, with TTL cleanup as backup. Avoid raw prompt or document identifiers in collection names or metrics.
3. **Single bounded read attempt.** [VERIFIED: `internal/cache/semantic.go:40-103`] Today vector-search errors silently fall through to relational full-candidate scan. That can multiply tail latency and conceal faults. Plan one short cache-read budget covering embedding, vector retrieval, candidate validation, and hit accounting; timeout/error should return a miss reason to the gateway, then continue ordinary primary forwarding. Distinguish clean miss from dependency fault in low-cardinality observability.
4. **Bounded post-response write.** [VERIFIED: `internal/gateway/service.go:336-355`, `internal/cache/semantic.go:105-155`] Current `Store` blocks response completion and uses request context, which may be cancelled after client completion. Plan a bounded in-process write queue with immutable snapshot payload, independent short worker context, explicit shutdown/drain limit, and drop-on-full behavior. The caller must not wait for embedding or vector persistence. Record failed writes by reason only.
5. **Fail-closed hit validation, fail-open cache availability.** [VERIFIED: `internal/cache/semantic.go:88-99,158-178`, `internal/gateway/service.go:142-186`] A matching vector alone is insufficient: verify dimensions and returned candidate scope/model/version/expiry before returning a hit. Invalid entry or malformed stored response becomes a miss, never a 200 with empty choices. Cache dependency failure bypasses to the primary model without cache-caused 5xx.

## Don't Hand-Roll

- No custom vector index or additional cache service. Existing adapters and relational repositories are adequate [VERIFIED: `internal/storage/interfaces.go:29-44`].
- No general-purpose background job platform. An in-process bounded queue is enough for optional cache writes; dropping a candidate is allowed by D-06.
- No exact-cache subsystem solely to satisfy the report's suggested O(1) lookup. Source search found only semantic cache in the gateway path [VERIFIED: `internal/gateway/service.go:138-208`]; decide later only if measured latency justifies it.
- No new public endpoint or embedding provider. Phase scope is an internal cache hardening slice.

## Common Pitfalls

| Risk | Evidence and required check |
| --- | --- |
| Hardcoded model remains in one direction | `Lookup` and `Store` each construct `"text-embedding-3-small"` [VERIFIED: `internal/cache/semantic.go:45-51,110-116`]. Both must use the same manually configured model. |
| Fixed validity and search bounds | Application wiring currently supplies threshold `0.9`, max candidates `10`, and TTL `24 * time.Hour` [VERIFIED: `internal/app/semantic_cache.go:35-40`]. Choose confirmed corpus-specific values through existing cache configuration. |
| Mixed embedding dimensions | SQLite cosine loop uses the shorter length rather than rejecting mismatch [VERIFIED: `internal/cache/semantic.go:188-199`]; pgvector validates dimension and Qdrant collection creation uses vector length [VERIFIED: `internal/storage/pgvector.go:47-80`, `internal/storage/qdrant.go:146-168`]. Reject wrong length before lookup/store and isolate collections by embedding identity. |
| Hidden vector fault | Vector search error falls through to SQLite; vector insert error is discarded [VERIFIED: `internal/cache/semantic.go:58-72,143-152`]. Add reason-only logging/metrics and primary forwarding, while retaining a verifiable write-failure signal. |
| Unsafe response decode | Cached `Choices` JSON unmarshal error is ignored [VERIFIED: `internal/gateway/service.go:158-186`]. Validate before treating entry as a hit. |
| Cache-key collision or information leak | `safeCollectionPart` replaces several characters with `_` [VERIFIED: `internal/cache/semantic.go:180-186`]. Composite identity should be canonical and opaque, with names compatible with selected vector store. |
| Async store outlives config/version | Queue work can be delayed past a version switch. Capture immutable version/model identity at enqueue; old-version writes may finish but must never match new-version reads. |
| Latency gate measured on wrong signal | Report section 5 proposed TTFT; `29-CONTEXT.md` D-08 overrides it for this non-streaming phase. Measure full response P95 at same load and low hit ratio, not streaming TTFT. |

## Validation Architecture

| Layer | Evidence | Gate |
| --- | --- | --- |
| Offline boundary/integration | Red-first deterministic tests at config, gateway, cache, SQLite/Postgres and vector-adapter boundaries, using actual request/response path without real network for routine checks. | Ineligible/default-off traffic causes zero embedding/vector calls; valid hits preserve response and zero Usage; negative near-matches, cross-scope/version/model cases yield zero false hits; all Go test invocations use `-timeout 60s`. |
| Fault injection | Embed timeout/error, vector timeout/error, malformed response, queue full, worker cancellation, shutdown, dimension mismatch. | No cache-caused 5xx, no second cache attempt after failure, low-cardinality reason emitted, primary model called once. |
| Performance | Repeated matched-load non-streaming runs, warm-up and low-hit workload, fixed concurrency, same primary provider/mock latency, recorded hit ratio and distribution. | Cache-on P95 complete response <= 1.05 x cache-off P95; preserve raw measurements and benchmark procedure. |
| Real providers | Phase-end controlled run using both user-provided `.env.local` embedding configurations and the actual configured vector-store route, with redacted evidence. | For each model: manual switch, primary response, async persistence observed, paraphrase hit, version/model isolation, forced cache fault bypass. No secrets in tracked artifacts. |

## Pre-Development Confirmations

1. Define who owns the allowlist and how a request selects one trusted application/use-case; confirm whether API-key ID is the intended authorization scope or whether a tenant ID exists elsewhere.
2. Define authoritative FAQ/document version publishing and propagation. A client-supplied version header alone is insufficient for isolation.
3. Confirm whether temperature/max-tokens are allowed only at a fixed profile, and whether unknown chat fields are rejected or cache-bypassed.
4. Confirm the two configured embedding models' provider IDs, dimensions, vector-store compatibility, TTL, similarity threshold and candidate bound in an isolated test environment without putting credentials in planning files.
5. Measure baseline cache-read/embedding/vector and primary latency before setting read budget, queue size, worker concurrency, and shutdown grace. The bounds must satisfy D-08 rather than be invented here.

## Project Constraints (from AGENTS.md)

- Planning only in this session; no code, deployment, or real-model calls. During later development, tests precede production changes, E2E is preferred for complex flows, and full E2E runs only at the end.
- Backend tests require a hard 60-second timeout. Preserve errors in diagnostics while allowing optional cache dependency faults to bypass primary forwarding per D-06.
- No secrets or configuration values hardcoded into tracked source. No tracked `.env.local` content.
- Avoid broad edits or migrations without explicit impact review. Respect function/file complexity limits and existing Go patterns.

## Confidence And Open Questions

Repository findings above are high-confidence observations from files read in this session. The opaque composite-scope representation and in-process queue are recommendations, not verified existing behavior. The trusted allowlist/version interface, quantitative read/queue budgets, and two-model compatibility remain open until the listed pre-development confirmations and baseline evidence exist.
