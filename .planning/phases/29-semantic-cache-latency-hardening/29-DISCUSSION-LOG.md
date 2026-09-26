# Phase 29: Semantic Cache Latency Hardening - Discussion Log

> Audit trail only. Planning and implementation should use `29-CONTEXT.md`.

**Date:** 2026-09-25
**Phase:** 29-semantic-cache-latency-hardening
**Areas discussed:** Cache business rules, acceptance standards

## Cache business rules

| Question | Recommendation presented | User decision |
| --- | --- | --- |
| Which requests may reuse a semantically similar answer? | Explicitly allowlist static, non-sensitive FAQ/versioned document Q&A; bypass all other traffic. | Accepted. Cache stays disabled by default. |
| How is the embedding model selected? | Manually configure provider/model and preserve model/dimension consistency. | No hardcoded embedding model. Two real model configurations in `.env.local` may be called during validation. |
| How are cache hits metered? | Return zero-token Usage, do not debit nonexistent upstream token usage, and record hits/savings separately. | Accepted. |
| What happens when FAQ or document content changes? | Separate entries by knowledge version; use TTL as fallback expiry. | Accepted. |

The accepted bypass recommendation included tools, streaming, multimodal, structured output, live/personalized/sensitive or high-stakes content, exact calculations/quotations, and answer-relevant multi-turn context. Model/prompt, tenant, and knowledge-version boundaries must prevent cross-context reuse.

## Acceptance standards

| Question | Recommendation presented | User decision |
| --- | --- | --- |
| Which performance metric gates release? | For matched low-hit-rate non-streaming load, cache-on P95 complete-response latency <= 1.05 x cache-off P95; streaming TTFT is inapplicable here. | Accepted. |
| What is the wrong-answer gate? | Zero erroneous hits in curated near-match, cross-tenant, and cross-version negative cases; report positive hit rate without an arbitrary threshold. | Accepted. |
| How should dependency faults behave? | Immediately bypass cache and forward to the primary model; write failures may drop work and should record only a reason. | Accepted. No additional cache-caused request 5xx. |
| Are real embedding models part of phase-end validation? | Exercise both user-configured models in a controlled gateway integration scenario, outside per-commit CI. | Required full-flow verification with real models. |

## Agent Discretion

- Determine the concrete short lookup budget, bounded write capacity, and compatible version/key representation from baselines and existing components.
- Use deterministic tests for routine merges and redact real-provider validation evidence.

## Deferred Ideas

- General timeout/retry policy, Console, MCP, new providers, and caching for currently excluded request types.
