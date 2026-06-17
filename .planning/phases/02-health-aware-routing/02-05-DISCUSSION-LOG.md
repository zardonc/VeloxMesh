# Phase 2.5: Provider Retry and Fallback Execution - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-15
**Phase:** 2.5-provider-retry-and-fallback-execution
**Areas discussed:** Next phase focus, retry eligibility, fallback selection, scope boundaries

---

## Next Phase Focus

| Option | Description | Selected |
|--------|-------------|----------|
| Provider retry/fallback | Use Phase 2.4 provider error categories to retry another healthy provider on transient failure. | ✓ |
| Admission control/rate limiting | Start building concurrency queues and rate limit behavior. | |
| Admin API/provider CRUD | Add control-plane runtime management of providers. | |
| Redis-backed health | Move provider health state out of process into Redis. | |

**User's choice:** The user asked to plan the next implementation function after Phase 2.4.
**Notes:** Provider retry/fallback is the most natural next slice because it directly uses the provider health/error contract that Phase 2.4 just stabilized. It improves runtime availability while keeping Phase 2 scope bounded.

---

## Retry Eligibility

| Option | Description | Selected |
|--------|-------------|----------|
| Retry transient provider errors only | Retry rate limit, timeout, unavailable, bad response, and generic provider error. | ✓ |
| Retry every provider error | Try another provider for any upstream failure, including auth/model/request errors. | |
| No retry policy yet | Only prepare interfaces for future fallback. | |

**User's choice:** Agent-selected recommendation based on current architecture and 02-04 error taxonomy.
**Notes:** Invalid request, invalid model, and auth errors should fail fast. Retrying them would hide bad input or bad configuration.

---

## Fallback Selection

| Option | Description | Selected |
|--------|-------------|----------|
| Exclusion-aware router selection | Reuse routing strategy while excluding providers already attempted in this request. | ✓ |
| Hard-coded provider loop in service | Iterate over registry list directly inside gateway service. | |
| Fallback-chain config only | Require an explicit ordered fallback list before any fallback works. | |

**User's choice:** Agent-selected recommendation.
**Notes:** The router already owns health-aware provider selection. Phase 2.5 should extend that boundary rather than duplicating routing logic in `gateway.Service`.

---

## Scope Boundaries

| Option | Description | Selected |
|--------|-------------|----------|
| Small non-streaming fallback slice | Implement fallback for non-streaming `/v1/chat/completions` only. | ✓ |
| Full circuit breaker | Add Closed/Open/HalfOpen state machine and probes. | |
| Streaming retry support | Retry SSE requests and replay partial output. | |
| Runtime admin policies | Add API/config storage for retry policy management. | |

**User's choice:** Agent-selected recommendation.
**Notes:** Streaming retry, full breaker state, Redis, Admin API, and dynamic policies all belong in later phases. This phase should stay small enough to verify thoroughly with fake providers.

---

## the agent's Discretion

- Exact internal type names for attempt history and retry policy.
- Whether fallback metadata is carried in `llm.LLMResponse` or a gateway-layer response wrapper.
- Whether the exclusion-aware selection API is added to `routing.Router` or implemented as a specialized helper around existing registry/router state.

## Deferred Ideas

- Full circuit breaker state machine.
- Redis-backed provider health.
- Admin API/runtime retry policy management.
- Streaming retry.
- Same-provider exponential backoff.
- Hedged requests.
- Model degradation and cost-aware fallback.
