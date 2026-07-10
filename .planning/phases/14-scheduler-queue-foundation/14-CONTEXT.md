# Phase 14: Scheduler Queue Foundation - Context

**Gathered:** 2026-07-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the cold-start Scheduler queue foundation for the gateway: optional scheduler scoring, one internal task/queue/executor path, Redis ZSET queueing with in-memory fallback, FIFO degradation, trusted priority handling, safe structured feature extraction, and core low-cardinality observability. The OpenAI-compatible data-plane contract stays unchanged.

</domain>

<decisions>
## Implementation Decisions

### Queue Placement and API Contract

- **D-01:** Use one internal task/queue/executor API. Do not build separate synchronous and asynchronous execution chains.
- **D-02:** Existing OpenAI-compatible HTTP handlers remain synchronous from the client perspective. Internally, the handler submits a task, waits for the task result, and returns the normal response.
- **D-03:** Phase 14 must not add a public queue/task API. A future async API may expose the same internal task core for submit/status/result, but it must not introduce a second scheduler or executor path.
- **D-04:** Queue storage should carry task IDs, scores, state needed for execution, and safe scheduling metadata. Raw prompts/messages, auth headers, API keys, provider secrets, and sensitive payloads must not be written to Redis, logs, metrics, or durable history.
- **D-05:** The supplemental API design's "Sync Facade over Async Core" direction is accepted. Treat its detailed module sketch as reference material, not a mandatory implementation checklist.

### Fallback and Backpressure

- **D-06:** Default failure behavior is fail-open. Scheduler disabled, unavailable, timing out, or breaker-open falls back to FIFO scoring without blocking gateway startup or normal forwarding.
- **D-07:** Scheduler calls use the v7.4 budget already captured in requirements: 15ms timeout, no inline retry, breaker-safe fallback.
- **D-08:** Redis ZSET is the primary queue backend. If Redis queue operations are unavailable, fall back to a single-node in-memory queue so the gateway can continue operating with reduced coordination guarantees.
- **D-09:** The gateway may reject or cancel only at real safety boundaries: queue hard-cap/backpressure, invalid/unauthorized input, request timeout, client disconnect, or explicit strict configuration. Queue soft limits should throttle or rate-limit before hard rejection.
- **D-10:** Ordinary data-plane responses must not expose internal scheduler topology, scheduler IDs, queue depth, routing internals, or provider secrets. Public 429/503/504 responses may include generic retry guidance such as `Retry-After` when appropriate.
- **D-11:** Do not implement durable task history, Redis Stream training buffers, or SQLite scheduler history in Phase 14 unless required for the foundation. Training feedback belongs to Phase 15.

### Priority Model

- **D-12:** Use exactly three Scheduler priority classes: `high`, `normal`, and `low`.
- **D-13:** Do not introduce an `urgent` class and do not build a four-level priority model or four-to-three alias table.
- **D-14:** `high` priority must not bypass admission, quotas, queue hard caps, concurrency limits, or tenant max-priority policy. It can only influence score/order within the same safety envelope.
- **D-15:** Priority must be resolved only from trusted config, trusted service headers, or structured request fields. Prompt text and user-authored content must never raise priority.
- **D-16:** If a requested priority exceeds policy, downgrade it silently for the data-plane response and emit only sanitized, low-cardinality audit/metric signals.
- **D-17:** Existing code may contain older `interactive`, `batch`, and `background` admission terms. Any compatibility translation must stay at a narrow integration seam or be migrated intentionally; Phase 14 planning/docs should present Scheduler priority as `high`, `normal`, `low` only.

### Heuristic Feature Boundary

- **D-18:** Gateway may parse the request locally to extract low-sensitive structured features and numeric/categorical prompt-summary features for scheduling.
- **D-19:** Scheduler receives only structured fields, numeric summaries, and bounded enum categories; it must never receive raw prompts, messages, tool arguments, authorization headers, API keys, provider secrets, or original request payloads.
- **D-20:** Allowed Phase 14 feature examples include model ID/class, input/output token estimates, stream flag, `high/normal/low` priority class, timeout/deadline class, enqueue timestamp, request kind, route/provider hint when low-cardinality, tool-call presence/depth, turn count, multimodal flag, and safe local summary counters such as question count, code block count, enumeration hint, instruction verb count, max sentence length bucket, and vocabulary richness bucket.
- **D-21:** Local prompt inspection must be deterministic and in-memory only. The extracted feature must be lower-information than the raw text, bounded in cardinality, and safe for logs/metrics only when explicitly sanitized; raw snippets such as "first 500 chars" must not be transmitted to Scheduler or stored.
- **D-22:** Prompt-derived features may influence scoring/classification but must never influence trusted priority elevation.
- **D-23:** Qdrant semantic-neighbor features from the prompt-process reference are useful later, but not required for Phase 14. If added in a future phase, semantic lookup must happen in Gateway and Scheduler may receive only aggregate statistics such as similar-history P50/std, never embeddings or raw text.
- **D-24:** Do not adopt asynchronous Qdrant score rewriting in Phase 14; it conflicts with the locked static virtual deadline preference and risks Redis write amplification. Cold-start scoring should remain heuristic/static virtual deadline based. ONNX/model paths, Qdrant-enhanced prediction, and training-data export are Phase 15+ or future-scheduler concerns.
- **D-25:** `request_kind` should be a fine-grained bounded enum produced locally by Gateway, not just coarse `chat`/`embedding` labels. Useful Phase 14 values include `simple_qa`, `code_gen`, `code_review`, `summarization`, `translation`, `structured_output`, `multi_step`, `tool_call`, `rag`, and `creative`; planner may trim the list, but must keep it bounded and non-textual.
- **D-26:** Accept the prompt-process reference's information-loss framing: without raw prompt, prediction will be imperfect. Use the Scheduler score's uncertainty penalty or conservative fallback when classifier confidence is low, rule signals conflict, or summary features imply high variance; do not try to recover perfect prompt understanding in Scheduler.

### Agent Discretion

Planner may choose the smallest internal task/result bridge that satisfies the synchronous facade. Buffered per-task channels are acceptable, but planner should keep cancellation, timeout cleanup, and goroutine leak prevention explicit. Avoid building a broad status manager unless a plan slice needs it for the queue foundation.

### Tooling and Verification Constraints

- **D-27:** Use the local protoc binary at `C:\Soft\1A-Coding\protoc-35.1-win64\bin\protoc.exe` for Scheduler protobuf generation. Go generator plugins (`protoc-gen-go`, `protoc-gen-go-grpc`) must be installed on `PATH` before generated files are updated.
- **D-28:** Any Phase 14 development that enables services or validates component calls must use real components and real network calls. Mock clients, mock data, and skipped necessary tests are not acceptable for service integration verification.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning Scope

- `.planning/ROADMAP.md` - Phase 14 goal, success criteria, and candidate plan slices for v7.4 Gateway Scheduler.
- `.planning/REQUIREMENTS.md` - SCH/PRIO/SCORE/OBS requirements and out-of-scope boundaries.
- `.planning/PROJECT.md` - Current milestone constraints and existing architectural decisions.

### Scheduler Design References

- `Agent-gateway/Gateway-Scheduler-API-Design.md` - Supplemental queue/API design. Use as reference only; do not adopt unsafe response details, raw-payload storage, unlimited urgent bypass, or Phase 15 training/history scope into Phase 14.
- External reference from project context: `C:/Users/inthe/IdeaProjects/Notes-sur-l-IA/Projects/Agent-gateway/Gateway-Scheduler-Implementation.md` - Original scheduler implementation reference used to seed v7.4 planning. If available to the downstream agent, read for intent; repository-local decisions above take precedence.
- External reference from user discussion: `C:/Users/inthe/IdeaProjects/Notes-sur-l-IA/Projects/Agent-gateway/Gateway-Scheduler-prompt-process.md` - Prompt-processing reference. Adopt the Gateway-local numeric/categorical summary boundary; defer Qdrant neighbor enhancement and async score rewriting unless a later phase explicitly scopes them.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/admission/controller.go`: Existing admission code has older `interactive`, `batch`, and `background` terms; planner must account for that without leaking them into the new Scheduler priority vocabulary unless a narrow compatibility seam is needed.
- `internal/hotstate/hotstate.go`, `internal/hotstate/redis.go`, `internal/hotstate/local.go`: Existing hot-state and local/Redis fallback patterns can inform the queue backend boundary.
- `internal/gateway/circuitbreaker.go`: Reusable circuit-breaker shape for Scheduler client fallback.
- `internal/observability/metrics.go`, `internal/observability/prometheus.go`: Existing low-cardinality metrics interface and Prometheus style should constrain scheduler/queue metrics.

### Established Patterns

- Config/env/file loading and validation live in `internal/config/config.go`; Scheduler config should follow the existing disabled-by-default pattern.
- App wiring in `internal/app/app.go` already handles Redis/local degradation and admission setup; Scheduler queue wiring should fit that style.
- Request flow in `internal/gateway/service.go` already coordinates admission, provider circuit breakers, metrics, settlement, and semantic cache; queue insertion must preserve the OpenAI-compatible data-plane contract.

### Integration Points

- Insert queue admission after request validation/auth/admission and before provider execution.
- Scheduler remains a stateless scoring oracle. Gateway owns intake, queue storage, task state, execution, timeout/cancellation, fallback, and response writing.
- Keep ordinary logs/traces/metrics sanitized and low-cardinality; do not include raw prompts, auth material, provider secrets, task payloads, or high-cardinality per-task labels.

</code_context>

<specifics>
## Specific Ideas

- Use "sync facade over async core" as the mental model: the client sees the same synchronous HTTP API while the gateway internally submits and waits on a queued task.
- Future public async task APIs are allowed only as a thin exposure of the same internal task core.
- The supplemental API design is useful for flow diagrams and race-condition reminders, especially buffered result delivery and timeout cleanup, but its broad status/history/training sections should be trimmed to the phase boundary.
- The prompt-process reference sharpens the feature boundary: Gateway may compute local summary features from prompt content, but Scheduler receives only bounded numbers/enums and never raw text.
- The prompt-process reference's local summary counters, fine-grained `request_kind`, and uncertainty-penalty posture are in scope for Phase 14; Qdrant semantic neighbors, local ONNX classifiers, and training-data/model paths stay deferred.

</specifics>

<deferred>
## Deferred Ideas

- Public async queue/task APIs (`POST /tasks`, `GET /tasks/{id}`) - future phase after the internal task core is stable.
- Durable training feedback, SQLite scheduler history, Redis Stream sample buffering, and ONNX/model rollout - Phase 15+.
- Qdrant semantic-neighbor scheduling features, local ONNX prompt classifiers, and any asynchronous score refinement - future scheduler enhancement after static scoring is proven.
- UI/Admin Console queue health views - future BFF/Admin Console milestone.

</deferred>

---

*Phase: 14-Scheduler Queue Foundation*
*Context gathered: 2026-07-03*
