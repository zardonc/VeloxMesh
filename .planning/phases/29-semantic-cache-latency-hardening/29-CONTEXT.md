# Phase 29: Semantic Cache Latency Hardening - Context

**Gathered:** 2026-09-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Harden the existing semantic cache for explicitly eligible, non-streaming gateway requests. Remove the hardcoded embedding model, bound cache-read latency, move cache writes off the response path, isolate entries by business and embedding version, and prove that cache failures do not impair primary model forwarding.

This phase does not add general provider retries or stage timeouts, cache tool or streaming requests, introduce a Console, expand providers, or implement MCP/Agent execution. Phase 27 remains the authority for terminal and Usage settlement; Phase 28 tool-protocol bypass remains intact.
</domain>

<decisions>
## Implementation Decisions

### Cache eligibility and answer validity

- **D-01:** Semantic caching is disabled by default. Enable it only for explicitly allowlisted tenant/application use cases involving static, non-sensitive FAQ or versioned document Q&A where paraphrases may safely reuse an answer.
- **D-02:** Requests involving tools, streaming, images/files/audio, structured output, high-freshness data, personalization, sensitive content, high-stakes advice, exact calculations/quotations, or answer-relevant multi-turn context bypass semantic reuse. A text-only request is not automatically eligible.
- **D-03:** A cache hit must not cross tenant/authorization scope, target model, answer-affecting prompt or generation settings, or knowledge/document version. Publishing a new FAQ/document version must immediately stop matching old entries; TTL is only a fallback expiry control.
- **D-04:** Cache hits retain the existing zero-token Usage behavior and do not debit tokens for an upstream call that did not happen. Record hit and estimated savings separately, without logging prompts, cached answers, credentials, or other sensitive payloads.

### Embedding and failure behavior

- **D-05:** The embedding model is manually configurable; no embedding model name may be hardcoded in cache business logic. The semantic cache remains off by default. Provider/model selection and vector dimension must be coherent, including when switching between models with different dimensions.
- **D-06:** Cache lookup, embedding, vector search, or background write failures, timeouts, and overload must immediately degrade to ordinary primary-model forwarding. Cache faults must not add main-request 5xx responses. A failed or full write queue may drop cache writes; record only a low-cardinality reason.
- **D-07:** The disabled cache path adds no external I/O. Enabled semantic reads have an independent short budget; cache writes must not delay successful response completion. Choose the concrete budget, queue bounds, and shutdown behavior during research and planning, then verify them.

### Acceptance and test evidence

- **D-08:** In a controlled, same-load, low-hit-rate non-streaming benchmark, cache-enabled P95 complete-response latency must be at most 1.05 times cache-disabled P95. Streaming TTFT is not the primary metric because this phase does not cache streaming requests.
- **D-09:** A human-labeled negative set containing near-identical questions with different correct answers, cross-tenant requests, and cross-version requests must produce zero erroneous hits. Report positive paraphrase hit rate, but do not invent a percentage gate before representative workload data exists.
- **D-10:** Failure-injection evidence must show immediate bypass for embedding, vector-store, and write-queue faults, no cache-caused primary-request 5xx, and a recorded reason without sensitive content.
- **D-11:** Both real embedding models already configured in `.env.local` must complete a controlled, full gateway-flow validation at phase end: manual model switch, primary response and cache write, paraphrased request hit, model/version isolation, and fault bypass. Use the actual configured vector-store path. Do not copy `.env.local` values into plans, logs, fixtures, or commits.
- **D-12:** Deterministic local tests remain the routine merge gate; real-provider calls are phase-end validation, not per-commit CI. Backend test commands have a hard 60-second timeout. Record a repeatable, redacted artifact with commands, environment shape, model identifiers, observed result, and latency comparison.

### Agent Discretion

- Decide whether an existing exact-key cache can be reused ahead of semantic lookup; do not invent a second cache subsystem solely for this phase.
- Select the smallest configuration and storage changes that enforce eligibility, version isolation, model/dimension compatibility, and bounded latency while preserving existing deployment modes.
- Set specific cache-read and worker limits from measured baselines rather than arbitrary defaults; document them before implementation.
</decisions>

<acceptance_contract>
## Phase Gate

1. Default-off and ineligible requests do not call embedding or vector services and preserve the existing OpenAI-compatible response contract.
2. Eligible requests can produce a primary response, persist a cache candidate off the response path, and later return an allowed semantic hit with zero-token Usage. New knowledge/model versions and other tenants cannot hit the old entry.
3. Curated negative fixtures produce no erroneous hits; positive fixtures demonstrate that useful paraphrases can hit, with measured hit rate reported.
4. Low-hit-rate P95 complete-response latency meets D-08 under matched load; raw runs, workload, concurrency, and cache-hit ratio are recorded. Do not substitute HTTP-header time or streaming TTFT.
5. Cache dependency faults only cause observable bypass; write overload never blocks or fails the client response.
6. Both real embedding configurations pass the end-to-end gateway scenario in D-11, with redacted evidence and no credentials in tracked files.
</acceptance_contract>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Scope and prior contracts

- `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` - Section 5 and the short-term plan define the cache latency direction; its numeric acceptance is refined here for non-streaming traffic.
- `.planning/ROADMAP.md` - Phase 29 goal, `CACHE-F01` mapping, and Phase 28 dependency.
- `.planning/PROJECT.md` - Gateway latency, security, provider isolation, and deployment constraints.
- `.planning/REQUIREMENTS.md` - Future requirement `CACHE-F01`; v7.9 protocol requirements are already verified.
- `.planning/phases/27-stream-terminal-settlement-consistency/27-CONTEXT.md` - Existing terminal and Usage ownership.
- `.planning/phases/28-tool-calling-protocol-completion/28-CONTEXT.md` - Tool traffic bypass and protocol boundary.

### Existing cache path and validation

- `internal/gateway/service.go` - Current eligibility checks, synchronous lookup/store placement, cache-hit Usage.
- `internal/cache/semantic.go` - Current hardcoded embedding model, vector search, and persistence behavior.
- `internal/app/semantic_cache.go` - Cache wiring and embedding adapter selection.
- `internal/config/config_file.go` - Cache configuration and legacy environment mapping.
- `internal/cache/semantic_test.go` - Current cache-level tests.
- `tests/integration/semantic_cache_test.go` - Existing gateway integration coverage.
</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `SemanticCacheService`, `EmbedAdapter`, `VectorAdapter`, and the existing semantic-cache repository provide the core contracts. Reuse these rather than adding a parallel cache stack.
- Existing `CacheConfig` already has enablement, provider, vector-store, and dimension fields, but the cache implementation still hardcodes its embedding model.

### Established Patterns

- The gateway bypasses cache for tool-protocol requests and streaming; Phase 27/28 own those semantics. The current cache hit returns zero Usage.
- SQLite is the default relational store and Qdrant is the primary vector path; PostgreSQL/pgvector is an extension path. Preserve their compatibility without widening this phase to storage redesign.

### Integration Points

- `Service` performs semantic lookup before upstream routing and currently stores successful non-streaming responses before returning. The planning work must remove cache-only blocking without changing provider execution or settlement ownership.
- `.env.local` contains two user-provided real embedding configurations for later validation. Read it only in the execution/test environment; never copy its contents into planning artifacts.
</code_context>

<specifics>
## Specific Ideas

- Compare cache-on and cache-off using the same eligible, low-hit-rate non-streaming workload. Treat live-provider measurements as supporting evidence; use controlled fixtures to make the gate reproducible.
- The real-model check must exercise the gateway cache lifecycle, not merely call each model's embeddings endpoint.
- No Phase 29 source-code change or real-model call was authorized during this discussion.
</specifics>

<deferred>
## Deferred Ideas

- General connect/first-byte/stream-idle/total timeout and retry-policy redesign.
- Console, full capability catalog, new providers or endpoints, MCP, and Agent orchestration.
- Semantic caching for streaming, tool, multimodal, structured-output, personalized, sensitive, or time-critical requests.
</deferred>

---

*Phase: 29-semantic-cache-latency-hardening*
*Context gathered: 2026-09-25*
