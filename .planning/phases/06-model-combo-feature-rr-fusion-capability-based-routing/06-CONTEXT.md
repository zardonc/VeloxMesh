# Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing) - Context

**Gathered:** 2026-06-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement persistent model combos for the AI gateway. A combo is a saved gateway configuration that users can call as a normal `model`; the gateway then selects one or more target provider models according to the combo algorithm, capability requirements, health, and fallback behavior.

</domain>

<decisions>
## Implementation Decisions

### Combo Definition Surface
- **D-01:** Combos must be persisted as durable gateway configuration, not just static config.
- **D-02:** Backend/admin configuration owns combo definition. For client requests, a combo appears as a new model name.
- **D-03:** The gateway automatically dispatches combo requests to target providers according to the configured algorithm.

### Combo-as-Model Contract
- **D-04:** `/v1/models` must include combo names as available models alongside provider-backed models.
- **D-05:** A request whose `model` matches a combo name should be accepted through the normal OpenAI-compatible data plane.
- **D-06:** Provider-specific details stay behind routing/provider boundaries; clients should not need to know which provider model was chosen unless existing response headers/metadata already expose it.

### Fusion Algorithm
- **D-07:** Fusion mode queries all models in the combo in parallel.
- **D-08:** Fusion then calls a judge model to synthesize one answer.
- **D-09:** The judge model is user-configurable and must be one of the models already connected to the current AI gateway.
- **D-10:** Fusion cost semantics are explicit: each request bills all panel models plus the judge, so a combo with N panel models makes N+1 upstream calls.

### Capability-Aware Routing
- **D-11:** Add a specific algorithm named `capacity auto-switch`.
- **D-12:** Capacity auto-switch sends image, PDF, or audio requests to a combo member that supports the required capability first.
- **D-13:** Capability filtering must use the existing provider/model capability contract where possible, including Phase 5 tool and multimodal signals.

### Failure Handling
- **D-14:** If a selected combo provider call fails, the gateway should try the next provider in combo order.
- **D-15:** Ordered fallback applies after algorithm selection; do not stop at the first failed provider if later combo members remain eligible.

### the agent's Discretion
- Choose the smallest durable schema/API shape that supports persisted combos, combo-as-model listing, round-robin, fusion, capacity auto-switch, and ordered fallback.
- Preserve existing provider health filtering, fallback-chain behavior, streaming behavior, cost tracking, and OpenAI-compatible responses unless a combo decision explicitly requires extension.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/PROJECT.md` — project contract, active Phase 6 scope, gateway constraints, and key prior decisions.
- `.planning/REQUIREMENTS.md` — milestone v5 requirements and Phase 6 combo requirements.
- `.planning/ROADMAP.md` — Phase 6 goal, dependency on Phase 5, and milestone context.
- `.planning/phases/05-tool-function-multimodal/05-CONTEXT.md` — Phase 5 decisions for tool/multimodal behavior and future rules pipeline.

### Existing Code
- `internal/routing/router.go` — current health-aware router, round-robin, least-latency, provider eligibility, and provider capability exposure.
- `internal/providers/catalog.go` — provider model/capability catalog shape.
- `internal/providers/registry.go` — provider registry and eligibility lookup.
- `internal/llm/types.go` — OpenAI-compatible request/response model contract.
- `internal/controlstate/repository.go` — durable repository interfaces for provider/routing state.
- `internal/controlstate/migrations` — existing SQLite/PostgreSQL migration patterns.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `routing.HealthAwareRouter`: already performs model eligibility, health filtering, round-robin, least-latency fallback, and provider override handling.
- `providers.Registry`: already knows provider models and capability metadata; combo routing should reuse this instead of duplicating provider lookup.
- `controlstate.Repository`: already exposes durable provider/routing repositories and should be extended minimally for persisted combos.

### Established Patterns
- Provider-specific behavior stays behind adapter packages.
- Static JSON config is now a compatibility/local-development seed path; durable database state is the intended source of truth.
- OpenAI-compatible data-plane request/response shape is preserved for clients.
- Redis and other hot-state systems are optional and must not be required for lite mode.

### Integration Points
- `/v1/models` must merge real provider models and combo model names.
- `/v1/chat/completions` model resolution must detect combo names before normal provider eligibility fails.
- Usage/cost settlement must account for fusion fan-out plus judge call.
- Admin/control-state APIs need a persisted combo configuration surface.

</code_context>

<specifics>
## Specific Ideas

- Fusion is intentionally high quality and high cost: all panel models run in parallel, then a configured judge model synthesizes the final answer.
- Capacity auto-switch should prioritize capable models for image, PDF, and audio requests.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within Phase 6 scope.

</deferred>

---

*Phase: 6-Model Combo Feature (RR, Fusion, Capability-based routing)*
*Context gathered: 2026-06-25*
