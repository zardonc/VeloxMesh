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
- **D-16:** After the architecture v2.1 refactor, combo persistence and runtime loading are SQLite-first. PostgreSQL support is retained only as a later adapter/extension path under Phase 12.

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
- When aligning Phase 06 with the new architecture, keep non-conflicting combo code, but prioritize removing PostgreSQL-first assumptions and any direct storage coupling that would fight the new Adapter/DAL direction.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/PROJECT.md` — project contract, active Phase 6 scope, gateway constraints, and key prior decisions.
- `.planning/REQUIREMENTS.md` — milestone v5 requirements and Phase 6 combo requirements.
- `.planning/ROADMAP.md` — Phase 6 goal, dependency on Phase 5, and milestone context.
- `.planning/phases/05-tool-function-multimodal/05-CONTEXT.md` — Phase 5 decisions for tool/multimodal behavior and future rules pipeline.
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-refactor-design.md` — updated SQLite + Redis Stack + Qdrant refactor direction.
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` — merged architecture v2.1; downstream planning must treat it as the current source of truth.

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
- Static JSON config is a compatibility/local-development seed path; SQLite-backed durable state is the intended Plan 1 source of truth.
- OpenAI-compatible data-plane request/response shape is preserved for clients.
- Redis and other hot-state systems are optional and must not be required for lite mode.
- PostgreSQL/pgvector work is not deleted, but it is no longer a Phase 06 priority. It belongs behind adapter interfaces and Phase 12 extension planning.

### Architecture Conflict Audit
- The audit scope is system-wide, not limited to Phase 06. Check startup wiring, durable storage, routing/gateway, Admin APIs, deployment defaults, README/project docs, and legacy architecture artifacts before continuing new feature work.
- SQLite connection setup must follow architecture v2.1 pragmas (`foreign_keys`, WAL mode, busy timeout, and normal synchronous mode) because it is now the primary durable control-state path.
- Default deployment and developer-facing docs must not present PostgreSQL/pgvector or LanceDB as required middleware. Redis Stack and Qdrant are the Plan 1/2 baseline; LanceDB is Plan 3 edge-only.
- Existing PostgreSQL code may remain when non-conflicting, but any new default path, test plan, or roadmap item should treat it as Phase 12 extension work.

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
- Combo routing must translate the client-facing combo model to the selected upstream provider model before invoking the adapter, while preserving the combo name in client-facing responses.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within Phase 6 scope.

</deferred>

---

*Phase: 6-Model Combo Feature (RR, Fusion, Capability-based routing)*
*Context gathered: 2026-06-25*
