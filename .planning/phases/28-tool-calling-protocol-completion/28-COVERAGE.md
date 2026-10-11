# Phase 28 Coverage Contract

**Phase:** Tool Calling Protocol Completion
**Requirement:** TOOL-F01
**Planning date:** 2026-09-23
**Delivery estimate:** 7.5 engineer-days across seven plans
**Hard dependency:** Phase 27 stream finalizer and Usage settlement remain the only terminal owners.

## Requirement Resolution

`init.plan-phase` now maps `phase_req_ids` to `TOOL-F01`, sourced from `.planning/REQUIREMENTS.md` and assigned to all seven plans and the Phase 28 ROADMAP entry. No phase SPEC supplies Edge Coverage or Prohibitions, so the default-on spec-less fallback applies; its edge and prohibition disposition is recorded below and lifted into the relevant PLAN.md `must_haves`.

### Spec-less Probe Disposition

The deterministic edge probe was run against `TOOL-F01` with the English requirement text and its explicitly expanded collection, text, stateful, and I/O shapes (validated against D-01..D-27). All six applicable candidates are resolved to explicit, testable acceptance criteria; none is dismissed or left implicit:

| Edge | Acceptance criterion | Plan |
|---|---|---|
| Adjacency | Equal function names and call IDs are rejected; distinct calls do not merge merely because they share a function name; each `tool_call_id` settles only its own outstanding call. | 28-01, 28-03 |
| Empty | Omitted tools/choice and empty assistant text remain distinguishable from invalid blank tool names, malformed tool definitions, and missing call IDs; no-tools requests remain compatible. | 28-01 |
| Encoding | Function-name blankness and equality use one explicitly documented Unicode policy across public validation and provider adapters; argument fragments are concatenated byte-faithfully and parsed as JSON only at completion, without lossy text normalization. | 28-01, 28-03 |
| Ordering | Parallel results may arrive in any order; interleaved stream fragments retain their established per-call zero-based index, ID, and name without cross-call reassignment. | 28-01, 28-03 |
| Idempotency | Repeated result IDs, repeated call identities, and duplicate completion are rejected or finalized exactly once; no duplicate Usage settlement or admission release occurs. | 28-01, 28-03, 28-07 |
| Concurrency | Interleaved calls and concurrent terminal paths preserve independent identities and exactly-once Phase 27 finalization; cancellation releases bounded shadow state without changing settled Usage. | 28-03, 28-07 |

Prohibition recall surfaced two phase-specific privacy/safety intents beyond routine validation: (1) the gateway must never execute a client tool or silently turn tool data into MCP/Agent actions; (2) it must never expose raw tool schemas, arguments, results, call IDs, or tenant-defined names in observability or error text. Generic injection/OWASP protections remain owned by security review rather than minted as new phase prohibitions. Both kept prohibitions are projected as descriptor-less `must_haves.prohibitions` with `status: resolved` and `verification: test` in 28-01 and 28-07 respectively; with no wired enforcement descriptor, the verifier must flag each as unverified rather than silently marking it green until execution supplies review evidence. No probe candidate is silently dropped.

No package-manager install is planned. The pinned Anthropic and Gemini SDK versions remain unchanged, and no new dependency is introduced.

### Coverage Disposition Labels

- **INTEGRATE** means new Phase 28 behavior must be implemented and asserted in the named plan; the reason states the protocol or gateway contract being added.
- **REGRESSION** means existing no-tools, text-only, SSE, cache, cancellation, terminal, Usage, or privacy behavior must remain unchanged; the reason states the compatibility control.
- **OPT-OUT** means the behavior is intentionally rejected or deferred rather than silently supported; the reason must identify the locked boundary, capability uncertainty, or nondeterministic dependency.

Every provider below has an explicit disposition for tools, all five choice meanings, history/results, non-stream/stream, ID/index, finish reason, Usage, errors, and privacy. Provider-specific wire assertions remain in 28-04 through 28-06; 28-07 proves the normalized public and lifecycle contract.

## Interface Coverage Matrix

Every cell names the executable plan and the observable contract that must be proven. A provider is not considered complete from a generic "supports tools" assertion.

| Interface surface | OpenAI-compatible | Anthropic | Gemini |
|---|---|---|---|
| Tool definitions | **28-04:** preserve every function name, description, and opaque JSON Schema object in `tools`; reject provider-side lossy conversion | **28-05:** map each function to `tools[].name/description/input_schema` without reducing schema keywords | **28-06:** map each function declaration without reducing the opaque schema object supported by the pinned SDK/API |
| `tool_choice` omitted | **28-04:** omit upstream choice and preserve provider default | **28-05:** omit upstream choice and preserve provider default | **28-06:** omit upstream choice and preserve provider default |
| `tool_choice: auto` | **28-04:** send OpenAI-compatible auto | **28-05:** send Anthropic auto | **28-06:** send Gemini AUTO mode |
| `tool_choice: none` | **28-04:** send OpenAI-compatible none | **28-05:** use the verified pinned-SDK/API representation; reject before I/O if official revalidation disproves support | **28-06:** use verified NONE/disabled function-calling mode; reject before I/O if official revalidation disproves support |
| `tool_choice: required` | **28-04:** send required | **28-05:** send any | **28-06:** send ANY without an allowed-name restriction |
| Specified function | **28-04:** send named function choice exactly | **28-05:** send tool choice with the exact declared function name | **28-06:** send ANY with exactly one allowed function name |
| Historical assistant tool calls | **28-04:** serialize assistant `tool_calls` with stable public IDs and argument strings | **28-05:** serialize assistant calls as `tool_use` blocks while retaining public correlation separately | **28-06:** serialize model function calls in validated order and retain public correlation separately |
| Tool result continuation | **28-04:** serialize `role=tool` with the exact unsettled `tool_call_id` | **28-05:** serialize `tool_result` referencing the provider correlation derived from the validated public ID | **28-06:** serialize function responses by validated name/order while the public boundary remains strict on `tool_call_id` |
| Non-stream response | **28-04:** normalize IDs, names, opaque argument JSON strings, and `finish_reason=tool_calls` | **28-05:** normalize `tool_use` blocks and stop reason; preserve content/tool-call coexistence | **28-06:** normalize function calls and finish reason; preserve content/tool-call coexistence |
| Streaming response | **28-04:** emit first identity chunk immediately, append argument fragments, validate at completion | **28-05:** normalize content-block start/delta/stop through the shared shadow state machine | **28-06:** normalize streamed candidates/function calls through the shared shadow state machine |
| ID and index | **28-04:** preserve valid provider IDs; generate stable opaque IDs only when absent; zero-based stable index | **28-05:** create stable public IDs while hiding provider block IDs; zero-based stable index | **28-06:** recover provider correlation only from validated name/order; never expose correlation internals; zero-based stable index |
| Finish reason | **28-04:** any completed tool call requires `tool_calls`; contradictory provider reason is bad response | **28-05:** `tool_use` completion maps to `tool_calls`; contradiction is bad response | **28-06:** completed function call maps to `tool_calls`; contradiction is bad response |
| Usage | **28-04:** normalized Usage remains input to the Phase 27 finalizer; adapter does not settle | **28-05:** same sole-owner rule | **28-06:** same sole-owner rule |
| Malformed/error behavior | **28-04:** duplicate/drifting IDs or names, invalid final JSON, malformed deltas, and impossible transitions produce stable provider-bad-response errors without raw payloads | **28-05:** same, including block identity/type drift | **28-06:** same, including ambiguous name/order correlation |
| Explicit OPT-OUT | **28-02/28-04:** exact model capability evidence may narrow adapter defaults; unknown or disproven modes are ineligible and explicit selection returns stable 400 before I/O | **28-02/28-05:** same; no downgrade to auto | **28-02/28-06:** same; no downgrade to AUTO |

### Provider Disposition Matrix

| Provider | INTEGRATE | REGRESSION | OPT-OUT |
|---|---|---|---|
| OpenAI-compatible | **28-04 + 28-07:** function `tools`; `omitted`/`auto`/`none`/`required`/specified-function choice; assistant `tool_calls` and `tool_call_id` results; non-stream and immediate stream; stable ID/index; `finish_reason`; normalized Usage; provider errors; privacy-safe public JSON/SSE. **Reason:** complete the shared public contract while preserving the provider's compatible wire shape. | **28-07:** no-tools and text-only JSON/SSE, existing cancellation/write-failure behavior, Phase 27 finalizer/Usage ownership, and ordinary cache/rule behavior. **Reason:** tool integration must not regress existing chat traffic or terminal accounting. | **28-02/28-04:** legacy `functions`/`function_call`, unsupported model/mode combinations, Fusion tool traffic, tool execution/MCP, and credential-dependent live smoke. **Reason:** locked scope or non-deterministic capability must fail/defer before upstream I/O, never silently become `auto`. |
| Anthropic | **28-05 + 28-07:** function tools; `omitted`/`auto`/`none`/`required` mapped to the verified Anthropic shape and specified-function choice; mixed assistant `tool_use` history and `tool_result`; non-stream and `content_block` stream; stable public ID/index; `tool_calls` finish reason; Usage; malformed block/JSON/stop-reason errors; privacy-safe output. **Reason:** normalize Anthropic blocks and partial JSON without exposing provider correlation or payloads. | **28-07:** no-tools/text-only messages, normal SSE framing, cancellation/write failure, Phase 27 health/breaker/release/Usage effects, and unchanged ordinary cache/rule behavior. **Reason:** adapter integration must remain isolated from existing lifecycle semantics. | **28-02/28-05:** legacy OpenAI fields, gateway tool execution/MCP, any mode or model the revalidated docs/SDK cannot express faithfully, Fusion tool traffic, and live-provider-only smoke. **Reason:** reject unsupported mappings before I/O instead of degrading to `auto`. |
| Gemini | **28-06 + 28-07:** function declarations; `omitted`/`auto`/`none`/`required`/specified-function choice through `ToolConfig`; model function-call history and user function responses; non-stream and interleaved stream; stable public ID/index with validated provider association; `tool_calls` finish reason; Usage; malformed/ambiguous correlation and provider errors; privacy-safe output. **Reason:** complete Gemini mapping while keeping provider-only association internal and public IDs strict. | **28-07:** no-tools/text-only JSON/SSE, cancellation/write failure, existing terminal/Usage and health/breaker behavior, and ordinary cache/rule behavior. **Reason:** the new function-call path must not alter established chat or finalizer behavior. | **28-02/28-06:** legacy OpenAI fields, unverified model/mode combinations, ambiguous name/order recovery, Fusion tool traffic, gateway execution/MCP, and live-provider-only smoke. **Reason:** ambiguous or unsupported mappings are protocol errors or optional evidence, not guesses. |

### Cross-Cutting Disposition

| Surface | Disposition | Reason |
|---|---|---|
| Cache and response-rule isolation | **INTEGRATE + REGRESSION** via 28-07 | Add the narrow protocol bypass, then prove ordinary text-only cache/rule behavior is unchanged. |
| Fusion and fallback safety | **INTEGRATE + OPT-OUT** via 28-02/28-07 | Integrate pre-I/O rejection and pre-visible safe fallback; explicitly opt out of Fusion tool execution and post-visible provider switching. |
| Phase 27 terminal ownership | **REGRESSION** via 28-07 | Preserve the existing exactly-once finalizer, release, health, breaker, trace, terminal metrics, and Usage semantics. |
| Privacy and cardinality | **INTEGRATE + REGRESSION** via 28-04..07 | Add protocol outcomes without raw payloads or per-fragment labels, then prove existing telemetry remains safe. |
| Full E2E and full suite | **REGRESSION gate** via 28-07 | Run complete deterministic E2E and `go test -timeout 60s ./...` only at the final gate; optional live smoke is OPT-OUT from merge evidence. |

## Cross-Cutting Coverage

| Contract | Required behavior | Plan and proof |
|---|---|---|
| Request validation | Invalid tool shape, blank/duplicate names, malformed specified choice, undeclared specified name, bad assistant call history, unknown/duplicate tool results, and unsettled calls followed by non-tool messages fail as stable 400 errors before routing/provider I/O | 28-01 contract and handler tests |
| Capability preflight | Automatic routing filters incompatible candidates; explicit incompatible selection returns `unsupported_tool_calling` or `unsupported_tool_choice`; no candidate fails before I/O | 28-02 router tests with zero-call provider spies |
| Strategy safety | Single and round-robin use only eligible candidates; Fusion explicitly rejects protocol-bearing requests; no tool request enters fusion | 28-02 routing tests |
| Fallback safety | No retry/fallback is added; only an existing, pre-visible, semantically safe fallback may continue and only to another fully eligible candidate | 28-02 and 28-07 tests |
| Stream shadow validation | Chunks emit immediately while a bounded per-request shadow state validates identity, index, ordering, final JSON, and finish reason | 28-03 shared state tests plus provider fixtures |
| Cache bypass | Requests containing tool definitions, non-omitted choice, assistant tool calls, or tool results bypass semantic-cache read and write only; ordinary chat cache behavior is unchanged | 28-07 gateway tests |
| Response-rule bypass | Response rules never inspect or rewrite tool arguments/results, including coexisting text and tool-call streams | 28-07 gateway tests |
| Terminal ownership | Phase 27 finalizer alone owns cancel/done classification, provider health, breaker, release, Usage settlement, and terminal metrics | 28-07 finalizer regression tests |
| Leakage control | Errors, logs, traces, and metric labels omit raw tool arguments/results and tenant-defined tool names; no per-fragment high-cardinality telemetry is added | Every provider plan plus 28-07 integration assertions |
| Boundedness | Unfinished-call shadow state is request-scoped, bounded by validated call count and existing request limits, and released on every Phase 27 terminal path | 28-03 and 28-07 tests |

## Locked Decision Coverage

| Decision | Binding implementation plan |
|---|---|
| D-01, D-02, D-03, D-04, D-05 | 28-01 public request contract and boundary validation |
| D-06, D-07, D-08 | 28-01 history/result state validation; gateway never executes tools |
| D-09, D-10, D-11, D-12, D-13, D-14 | 28-03 shared identity/index/correlation shadow state; 28-04..28-06 provider mappings |
| D-15, D-16, D-17, D-18, D-19, D-20 | 28-02 capability model, routing preflight, strategy, Fusion, and fallback gates |
| D-21 | 28-01 stable client errors; 28-04..28-07 provider and gateway leakage tests |
| D-22, D-23, D-24, D-25, D-26 | 28-03 stream state machine; 28-04..28-06 streamed adapter fixtures |
| D-27 | 28-07 Phase 27 finalizer/Usage ownership regression coverage |
| D-28 | 28-07 response-rule bypass |
| D-29, D-30, D-31, D-32 | 28-03 shared fixture matrix; 28-07 deterministic E2E and final validation architecture |
| D-33 | First tracer task in every implementation-facing plan rechecks official protocol and pinned SDK documentation; any conflict halts execution and requires plan revision |

## Source Audit

### GOAL

- **Covered:** strict end-to-end tool-calling protocol for OpenAI-compatible, Anthropic, and Gemini across request validation, routing, non-stream, stream, continuation, and gateway integration.
- **Covered:** Phase 27 remains the sole terminal/Usage owner.

### REQ

- **TOOL-F01:** covered by 28-01 through 28-07 and explicitly listed in each plan frontmatter.

### RESEARCH

- Shared normalized contract, minimal capability granularity, shared streaming state, provider fixture matrix, semantic-cache bypass, response-rule bypass, deterministic validation architecture, pinned SDKs, and no new dependencies are all assigned.
- Package Legitimacy Audit result: no package installation task; supply-chain checkpoint is not applicable.
- Per `.codex/gsd-core/workflows/plan-phase.md`, when `research_enabled=false`, Nyquist artifacts are not required for this run; the plan directly consumes the `Validation Architecture` in `28-RESEARCH.md`, so no `28-VALIDATION.md` is created.

### CONTEXT

- D-01 through D-33 are all mapped above.
- Deferred ideas are excluded: legacy `functions/function_call`, MCP, tool execution, agent loops, new providers/endpoints, full dynamic capability discovery, Console work, generic retry redesign, and broad cache strategy changes.

## Reachability and Completion Gate

The path is reachable as: HTTP handler boundary (28-01) -> capability-aware router (28-02) -> provider adapter using shared stream state (28-03..28-06) -> gateway cache/rule/finalizer integration (28-07) -> deterministic integration tests.

Phase 28 is complete only when every matrix row has an automated assertion, all High threats have a failing test before production edits and a passing test afterward, focused suites pass during development, and the full backend suite runs only in the final provisioned integration task.
