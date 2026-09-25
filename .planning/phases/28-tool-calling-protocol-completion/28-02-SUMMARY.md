---
phase: 28-tool-calling-protocol-completion
plan: "02"
subsystem: routing
tags: [go, tool-calling, routing, capabilities, fail-closed]
requires:
  - phase: 28-01
    provides: Normalized ToolProtocolRequirements on LLMRequest
provides:
  - Per-model, deep-cloned tool protocol capability snapshots
  - Fail-closed routing preflight across override, automatic, combo, and Fusion paths
  - Deterministic zero-I/O tool protocol routing tests
affects: [28-03, 28-04, provider-adapters, fusion]
actuals:
  tokens: 10949
  tasks: 3
  commits: 3
plan_head_before: e595f05dcad06b023b7811cb50ebf7d1f10696c1
tech-stack:
  added: []
  patterns:
    - Normalized tool requirements are authoritative for routing preflight
    - Capability maps are deep-cloned per catalog model snapshot
key-files:
  created:
    - internal/routing/router_tool_test.go
  modified:
    - internal/llm/tool_protocol.go
    - internal/providers/capabilities.go
    - internal/providers/catalog.go
    - internal/providers/openai/adapter.go
    - internal/providers/anthropic/adapter.go
    - internal/providers/gemini/adapter.go
    - internal/routing/router.go
key-decisions:
  - "Use a small fail-closed ToolProtocolCapability rather than a generic provider feature registry."
  - "Treat assistant tool-call history as an explicit capability so unsupported continuation requests never reach a provider."
  - "Reject all tool protocol requests before Fusion decision construction."
patterns-established:
  - "Route only through model catalog snapshots, never mutable adapter capability maps."
  - "Return 400-class unsupported tool errors before upstream I/O when normalized requirements cannot be honored."
requirements-completed: []
coverage:
  - id: D15-D17
    description: Explicit, clone-isolated per-model tool protocol capability snapshots.
    verification:
      - kind: unit
        ref: go test -timeout 60s ./internal/routing ./internal/providers/... ./internal/llm
        status: pass
    human_judgment: false
  - id: D16-D19
    description: Automatic filtering and explicit override errors preserve normalized tool choices before provider I/O.
    verification:
      - kind: unit
        ref: internal/routing/router_tool_test.go#TestHealthAwareRouter_ToolProtocolFiltersAutomaticCandidates
        status: pass
    human_judgment: false
  - id: D20
    description: Combo preflight cannot bypass capability constraints and Fusion rejects all tool protocol requests.
    verification:
      - kind: unit
        ref: internal/routing/router_tool_test.go#TestHealthAwareRouter_ToolProtocolRejectsFusionBeforeIO
        status: pass
    human_judgment: false
duration: 37m
completed: 2026-09-24
status: complete
---

# Phase 28 Plan 02: Capability-Aware Routing Summary

**Fail-closed per-model tool protocol snapshots and deterministic router preflight that reject unsupported choices before any provider or Fusion I/O.**

## Performance

- **Duration:** 37m
- **Started:** 2026-09-24T20:19:32Z
- **Completed:** 2026-09-24T20:56:27Z
- **Tasks:** 3/3
- **Files modified:** 8

## Accomplishments

- Added minimal per-model protocol capabilities for definitions, assistant history, tool results, stream deltas, and each public tool-choice mode; nested maps are deep-cloned.
- Routed normalized requirements through automatic, override, capacity, round-robin, and Fusion selection paths with stable 400-class errors before any provider execution.
- Added deterministic in-memory tests for all choice modes, unsupported candidates, clone isolation, no-I/O rejection, and text-only or stream-only compatibility.

## TDD Evidence

- **Failure modes encoded before production edits:** missing definitions, each omitted/auto/none/required/named choice mode, missing result or streamed deltas, assistant history, unsafe overrides, exhausted candidates, combo bypass, Fusion, and mutable snapshots.
- **RED:** `go test -timeout 60s ./internal/routing` failed before implementation because the tool protocol capability surface was absent. Committed as `3b0bd9b`.
- **GREEN:** `go test -timeout 60s ./internal/routing ./internal/providers/... ./internal/llm` passed after implementation.

## Task Commits

1. **Task 1: Write red routing and capability contract tests first** - `3b0bd9b` (`test`)
2. **Task 2: Implement minimal fail-closed tool capabilities and catalog snapshots** - `8ef1b61` (`feat`)
3. **Task 3: Enforce protocol preflight across all router branches** - `c81a5d4` (`feat`)

**Plan metadata:** skipped (commit_docs disabled)

## Files Created/Modified

- `internal/routing/router_tool_test.go` - Deterministic routing and capability contract tests using in-memory catalogs and provider spies.
- `internal/llm/tool_protocol.go` - Adds the authoritative normalized `UsesProtocol` predicate.
- `internal/providers/capabilities.go` - Defines and deep-clones fail-closed tool protocol capability snapshots.
- `internal/providers/catalog.go` - Exposes an immutable effective capability snapshot for a provider-model pair.
- `internal/providers/openai/adapter.go` - Declares the normalized protocol modes currently mapped by the OpenAI-compatible adapter.
- `internal/providers/anthropic/adapter.go` - Declares only currently mapped protocol behavior and leaves unsupported modes fail closed.
- `internal/providers/gemini/adapter.go` - Declares only currently mapped protocol behavior and leaves unsupported modes fail closed.
- `internal/routing/router.go` - Applies normalized capability preflight to every supported routing path and blocks Fusion before decision creation.

## Decisions Made

- Kept the capability shape protocol-specific and small; it does not infer support from provider type or the legacy broad `ToolCalling` flag.
- Made assistant tool-call history explicit because continuation messages require provider-specific preservation, not merely tool definitions.
- Kept provider request/response mapping out of this plan; unsupported Anthropic and Gemini normalized modes are declared false until later adapter work implements them.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Correctness] Added the normalized `UsesProtocol` predicate and assistant-history capability declaration.**
- **Found during:** Task 2
- **Issue:** Existing requirements had no single authoritative predicate, and definition/result/choice fields alone could not prove safe assistant tool-call continuation.
- **Fix:** Added `ToolProtocolRequirements.UsesProtocol` and `AssistantToolCalls` to the protocol snapshot.
- **Files modified:** `internal/llm/tool_protocol.go`, `internal/providers/capabilities.go`
- **Verification:** Focused routing, provider, and normalization tests passed.
- **Committed in:** `8ef1b61`

**2. [Rule 2 - Correctness] Tightened built-in adapter declarations to avoid optimistic capability claims.**
- **Found during:** Task 2
- **Issue:** Anthropic and Gemini adapters do not yet map every normalized explicit choice and continuation shape, so broad tool-calling declarations would allow unsafe routing.
- **Fix:** Declared only currently mapped behavior and marked unsupported protocol modes false; OpenAI-compatible retains explicit support for the mapped contract.
- **Files modified:** `internal/providers/openai/adapter.go`, `internal/providers/anthropic/adapter.go`, `internal/providers/gemini/adapter.go`
- **Verification:** Focused provider and routing tests passed.
- **Committed in:** `8ef1b61`

**Total deviations:** 2 auto-fixed (Rule 2 correctness)

**Impact on plan:** Required to satisfy fail-closed continuation and provider-contract correctness. No provider I/O, feature directory, retry layer, or tool execution was added.

## Issues Encountered

- The sandbox initially denied Go build-cache writes; focused test commands were rerun with approved cache access and passed.
- `.env.local`, SSH, and remote components were intentionally untouched because all plan verification is deterministic in-memory routing and capability coverage.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Later provider adapter plans can add mappings and widen only the corresponding explicit capability declarations.
- `TOOL-F01` remains open until the remaining Phase 28 plans complete the end-to-end provider mappings.

---
*Phase: 28-tool-calling-protocol-completion*
*Completed: 2026-09-24*

## Self-Check: PASSED

All eight implementation files and all three task commits were found in the repository. 
