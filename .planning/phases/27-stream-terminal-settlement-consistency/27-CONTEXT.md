# Phase 27: Stream Terminal and Settlement Consistency - Context

**Gathered:** 2026-09-20
**Status:** Ready for planning
**Source:** `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` plus user-selected short-iteration priority

<domain>
## Phase Boundary

Unify terminal classification and finalization for existing ordinary streaming, buffered streaming, and Fusion streaming paths. One authoritative terminal outcome must drive client protocol output, provider health, breaker state, metrics, trace, admission release, and usage settlement.

This is a narrow protocol-correctness iteration. It does not add public API fields, providers, database migrations, retry behavior, semantic-cache redesign, Tool Calling completion, or Console UI.
</domain>

<decisions>
## Implementation Decisions

### Locked
- **D-01:** The first terminal outcome wins. Done, clean EOF, provider error, response-policy rejection, client cancellation, and downstream write failure must not trigger duplicate finalization.
- **D-02:** Done or clean EOF maps to HTTP 200/completed, provider success, normal observability, and normal settlement processing.
- **D-03:** A provider error preserves its mapped status and category. Provider-health impact is determined only by `errors.AffectsProviderHealth`.
- **D-04:** Client cancellation or downstream write disconnection maps internally to `499/client_cancelled`, releases resources promptly, and never counts as a provider-health or breaker failure.
- **D-05:** Response-policy rejection keeps its own error category, does not count as provider failure, and does not settle client usage.
- **D-06:** Only a successful completed stream may debit the client. The final observed Usage settles once; no Usage produces `missing_usage`; Usage observed before failure or cancellation remains diagnostic and unsettled.
- **D-07:** Provider health, breaker result, request metrics, trace completion, admission release, model outcome, and usage finalization each run at most once.
- **D-08:** Ordinary, buffered, and Fusion streams must share the same terminal semantics; path-specific transport behavior may remain behind adapters or handlers.
- **D-09:** SSE emits one terminal sequence. Provider error may emit one `event:error` followed by one `[DONE]`; clean completion emits one `[DONE]`; downstream write failure stops further writes.
- **D-10:** No new external I/O, storage read, lock-heavy operation, or model call may be introduced in the per-chunk forwarding path.
- **D-11:** Existing OpenAI-compatible authentication and endpoint behavior remain unchanged; permission tests prove unauthorized requests are still rejected and authorized streaming still works.
- **D-12:** This planning round must specify test environment, test data, operation steps, and expected results for normal, abnormal, boundary, permission, compatibility, and performance scenarios.

### The Agent's Discretion
- Exact internal type and helper names for terminal outcomes and finalization.
- Whether the unification is a new finalizer object, a private method family, or a compact immutable result struct, provided side effects are exactly once and existing dependency injection boundaries are preserved.
- Choice of focused benchmark, allocation assertion, or deterministic instrumentation for demonstrating no material per-chunk regression.
</decisions>

<canonical_refs>
## Canonical References

- `internal/gateway/service.go` - ordinary and buffered stream lifecycle, settlement, health, breaker, metrics, traces, and admission release.
- `internal/gateway/fusion.go` - Fusion stream completion and existing `sync.Once` finalization pattern.
- `internal/http/handlers/chat.go` - OpenAI-compatible SSE output and downstream write boundary.
- `internal/http/handlers/chat_test.go` - handler-level stream completion and error protocol tests.
- `internal/gateway/service_test.go` - stream Usage, buffering, policy, and Fusion regression tests.
- `internal/llm/types.go` - `StreamEvent` and Usage event shape.
- `internal/gateway/errors` - error translation and provider-health classification.
- `.planning/milestones/v4-phases/04-streaming-rate-limits-cache-and-cost/` - original streaming and settlement decisions.
- `Agent-gateway/2026-09-20-网关项目-下一步开发方向调研报告.md` - roadmap rationale and latency boundary.
</canonical_refs>

<test_contract>
## Required Verification Matrix

Plans must provide executable tests for:

1. Normal completion: explicit Done, clean EOF without Done, one Usage, and multiple Usage events where the last event wins.
2. Provider failure: error before output, error after partial output, provider-health-affecting and non-affecting categories, and error followed by Done/close.
3. Cancellation and write failure: cancel before first token, cancel after partial output, handler writer failure, and request-context cancellation.
4. Boundary behavior: empty stream, Usage-only stream, duplicate terminal events, missing Usage, and terminal races.
5. Permission control: missing/invalid API key remains rejected; valid API key reaches the unchanged streaming endpoint.
6. Compatibility: ordinary, buffered, and Fusion paths produce equivalent finalization semantics; non-streaming behavior remains unchanged.
7. Performance: no per-chunk repository/network call, bounded allocations or benchmark delta, and prompt resource release after cancellation.

Every runnable verification command must state the observable failure signal and must use the repository's 60-second backend-test timeout.
</test_contract>

<delivery_constraints>
## Delivery Constraints

- Expected implementation duration: 4-6 engineer-days.
- Keep each plan independently reviewable and executable; prefer 2-3 plans with explicit dependencies over one large plan.
- No implementation, code modification, deployment, or live-provider call occurs during this planning phase.
- Use fakes or deterministic local test doubles for protocol and lifecycle tests; real-provider soak testing is optional and must not be an acceptance blocker.
- Preserve current Go/Chi architecture, immutable result passing, dependency injection, and existing provider-adapter boundaries.
</delivery_constraints>

<deferred>
## Deferred Ideas

- Complete Tool Calling and MCP-related mappings.
- Semantic-cache embedding configuration and asynchronous writes.
- Stage-specific timeout injection and retry eligibility.
- Capability Catalog, Provider Adapter Kit, and new provider coverage.
- Console v1 and control-plane UX.
</deferred>
