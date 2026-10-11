# Phase 28 Discussion Log

**Date:** 2026-09-22
**Phase:** Tool Calling Protocol Completion
**Purpose:** Record the gray-area decisions that shaped `28-CONTEXT.md`.

## Scope Selection

The source report ranked tool-calling correctness ahead of provider breadth, Console work, full capability catalogs, and MCP bridging. The user confirmed Phase 28 as the next phase and delegated all remaining gray-area choices to the agent.

## Gray Areas Resolved

### 1. Public Contract Shape

**Selected:** Modern OpenAI-compatible `tools` / `tool_choice`, function tools only, with strict structural and conversation-state validation.

**Alternatives considered:**

- Support legacy `functions` / `function_call` in the same phase.
- Keep `tool_choice` loosely typed and rely on upstream providers for validation.

**Rationale:** A single modern contract keeps the phase bounded and creates deterministic behavior across providers. Legacy compatibility is separable work, while loose pass-through would preserve the exact ambiguity this phase is intended to remove.

**User input:** User delegated the decision.

### 2. Tool Call Identity

**Selected:** Preserve valid upstream IDs; generate stable opaque IDs only when absent; enforce stable zero-based indexes and reject identity drift.

**Alternatives considered:**

- Replace every provider ID with a gateway ID.
- Accept missing/duplicate/changed IDs and repair them best-effort.

**Rationale:** Preserving valid IDs minimizes transformation and debugging friction. Conditional generation covers providers without compatible IDs, while strict rejection prevents incorrect tool results from being attached to the wrong call.

**User input:** User delegated the decision.

### 3. Provider Capability and Routing

**Selected:** Require one minimum contract across OpenAI-compatible, Anthropic, and Gemini; filter incompatible automatic candidates and reject explicit incompatibilities before upstream execution. Tool requests are rejected in Fusion mode.

**Alternatives considered:**

- Allow each adapter to expose a different partial contract.
- Silently downgrade unsupported `tool_choice` modes to `auto`.
- Attempt to merge tool calls from multiple Fusion branches.

**Rationale:** Partial or silently downgraded behavior makes routing outcomes model-dependent and hard to test. Fusion cannot safely reconcile independently generated call IDs, ordering, or tool arguments without becoming an Agent orchestration layer.

**User input:** User delegated the decision.

### 4. Streaming Behavior

**Selected:** Forward normalized fragments immediately, maintain bounded per-call state, validate concatenated arguments and finish semantics at completion, and reuse the Phase 27 terminal finalizer.

**Alternatives considered:**

- Buffer each complete tool call before emitting it.
- Pass provider fragments through without normalization or terminal validation.

**Rationale:** Full buffering harms first-event latency. Raw pass-through makes public behavior provider-specific and can produce unusable multi-round state. Immediate normalized forwarding preserves latency while terminal validation protects correctness.

**User input:** User delegated the decision.

## Agent Discretion Retained

- Exact Go type, helper, error constant, and generated ID names.
- Shared contract test harness and fixture organization.
- Exact number of executable PLAN files produced by the planning step.
- Optional real-provider smoke-test mechanics, provided deterministic local tests remain the merge gate.

## Deferred by Boundary

- Legacy function-calling compatibility.
- Tool execution, MCP bridging, Agent loops, permissions, and sandboxing.
- Full model capability catalog and Console management.
- New providers/endpoints and general retry, timeout, cache, or cost policy work.

---

*This log is audit context only. The binding phase decisions are in `28-CONTEXT.md`.*
