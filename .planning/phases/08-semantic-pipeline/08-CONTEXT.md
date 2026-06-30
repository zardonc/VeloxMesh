# Phase 8: Semantic Pipeline - Context

**Gathered:** 2026-06-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement the configurable semantic processing pipeline for chat requests and responses: handler registry, per-user rule configuration, rule toggles, execution ordering, hot-reloadable settings, and exception logging. This phase owns backend pipeline behavior and configuration APIs; the full Admin Console UI for managing those toggles is deferred to Phase 11.

</domain>

<decisions>
## Implementation Decisions

### Rule Scope and Defaults
- **D-01:** The initial rules are RTK, Headroom, PII, Rewrite, Caveman, Ponytail, and Filter.
- **D-02:** Every rule has an enable/disable switch. All rules default to disabled.
- **D-03:** Rule configuration is per-user. One user's enabled rules and options must not affect another user.
- **D-04:** Configuration precedence is only `user config > global default`. Do not add provider, model, route, or API-key override layers in Phase 8.

### Request and Response Behavior
- **D-05:** Default request order is `Filter(pre) -> PII Redaction -> Rewrite -> RTK -> Headroom -> Provider`.
- **D-06:** Default response order is `Provider -> Caveman OR Ponytail -> Filter(post) -> PII Restore`.
- **D-07:** PII redaction must protect request text before Rewrite or RTK sees it. PII restore runs near the end of response handling.
- **D-08:** Filter has two slots: request pre-filter may reject early, response post-filter may block or replace output.

### Caveman and Ponytail
- **D-09:** Caveman and Ponytail are mutually exclusive by default.
- **D-10:** On the request side, Caveman/Ponytail normally inject only a style hint and do not rewrite user text.
- **D-11:** Caveman/Ponytail may rewrite request text only when the user explicitly enables that special option. That option defaults to disabled.
- **D-12:** On the response side, Caveman/Ponytail may transform assistant output according to the selected style.

### Failure Handling
- **D-13:** If a handler fails, skip that handler and continue the pipeline.
- **D-14:** Every skipped handler failure must be logged with enough context to debug the rule, user scope, request ID, and exception without leaking prompts, secrets, or raw sensitive content.

### Admin Console Boundary
- **D-15:** Admin Console must eventually expose switches for all rules, but Phase 8 should focus on backend configuration, validation, storage, and hot reload. Full UI work belongs to Phase 11 unless a tiny existing admin endpoint is enough to verify the backend.

### the agent's Discretion
- Keep the first implementation narrow. Reuse the existing `internal/pipeline` package and gateway integration points instead of introducing a broad workflow engine.
- Use the smallest persisted config shape that supports global defaults, per-user overrides, rule enabled flags, Caveman/Ponytail mutual exclusion, and the request-rewrite special option.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` — Phase 8 scope, dependency on Phase 7, and v7 phase ordering.
- `.planning/PROJECT.md` — current architecture context and requirement lifecycle.
- `.planning/phases/07-adapter-interfaces-sqlite-foundation/07-CONTEXT.md` — Phase 7 decisions about SQLite, Redis Stack, Qdrant, minimal adapter seams, and deferred semantic pipeline work.

### Existing Code
- `internal/pipeline/pipeline.go` — current minimal request/response rule chain.
- `internal/gateway/service.go` — current chat request/response integration points for pipeline, routing, semantic cache, settlement, and streaming.
- `internal/llm/types.go` — request/response message structures that rules will inspect or transform.
- `internal/config/config.go` — existing config loading, validation, and defaults pattern.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/pipeline.Pipeline` already executes request rules forward and response rules in reverse order. It is the obvious starting point.
- `internal/gateway.Service` already constructs a pipeline and calls `ProcessRequest` before provider selection and `ProcessResponse` after provider success.
- `middleware.GetAuthIdentity(ctx)` already provides a user/API-key identity scope used by semantic cache and settlement.

### Established Patterns
- Configuration validation lives in `internal/config.Config.Validate`.
- Durable control state repositories already exist for SQLite/PostgreSQL-backed records and can guide the per-user rule config repository shape.
- Observability code avoids raw prompts and sensitive provider payloads; handler exception logging must follow that pattern.

### Integration Points
- Non-streaming requests pass through `HandleChatCompletion`.
- Streaming requests pass through `HandleChatCompletionStream`, but response transformation is harder because output arrives as chunks. Plan streaming behavior explicitly instead of assuming full-response transforms work there.
- Semantic cache currently looks up before `ProcessRequest`; planning must decide whether Phase 8 moves request processing before semantic cache, limits pipeline to non-cacheable paths, or records the current ordering as a deliberate constraint.

</code_context>

<specifics>
## Specific Ideas

- Keep Phase 8 intentionally small: no provider/model/route override matrix.
- Caveman/Ponytail request rewriting is a special opt-in because it can change user intent.
- Admin Console toggle support is required as a capability, but the real UI can wait until BFF/Admin Console Phase 11.

</specifics>

<deferred>
## Deferred Ideas

- Provider/model/route/API-key override layers for rule configuration.
- Full Admin Console UI for rule switches and rule editing.
- Advanced rule conflict resolution beyond Caveman/Ponytail mutual exclusion.

</deferred>

---

*Phase: 8-Semantic Pipeline*
*Context gathered: 2026-06-29*
