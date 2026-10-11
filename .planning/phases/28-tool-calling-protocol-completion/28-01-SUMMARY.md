---
phase: 28-tool-calling-protocol-completion
plan: "01"
subsystem: api
tags: [go, openai-compatible, tool-calling, request-validation, http]
requires:
  - phase: 27-stream-terminal-settlement
    provides: "Stable request lifecycle and provider-independent gateway path"
provides:
  - "Closed normalized tool_choice model with omission preserved"
  - "Pure tool conversation ledger and provider-neutral requirements"
  - "Early HTTP rejection of invalid tool protocol requests"
affects: [28-02-provider-capability-preflight, tool-calling-protocol-completion]
plan_head_before: 6c7febb37c9917a52649a8c1b26cb43b5960df6d
actuals:
  tokens: 8154
  tasks: 3
  commits: 3
tech-stack:
  added: []
  patterns:
    - "Normalize public tool protocol once at the HTTP boundary before routing"
    - "Use a pure pending-call ledger for assistant calls and tool results"
key-files:
  created:
    - internal/llm/tool_protocol.go
    - internal/llm/tool_protocol_test.go
    - internal/http/handlers/chat_tool_test.go
  modified:
    - internal/llm/types.go
    - internal/http/handlers/chat.go
    - internal/errors/errors.go
    - internal/http/handlers/chat_test.go
key-decisions:
  - "Use *ToolChoice so an omitted choice remains distinguishable from explicit auto."
  - "Retain parameters as json.RawMessage and validate only supplied JSON-object structure."
  - "Reject invalid tool protocol before gateway service invocation with generic invalid_request responses."
patterns-established:
  - "Tool protocol is data-only at the gateway boundary; it is never executed or dispatched."
  - "Later routing receives ToolProtocolRequirements instead of reparsing the public request."
requirements-completed: [TOOL-F01]
coverage:
  - id: D1
    description: "Closed normalized tool contract, schema boundary, and conversation ledger"
    requirement: TOOL-F01
    verification:
      - kind: unit
        ref: "go test -timeout 60s ./internal/llm -run 'TestToolProtocol'"
        status: pass
    human_judgment: false
  - id: D2
    description: "HTTP boundary rejects invalid tool payloads before service invocation and preserves compatible requests"
    requirement: TOOL-F01
    verification:
      - kind: unit
        ref: "go test -timeout 60s ./internal/http/handlers -run 'TestChat.*Tool|TestChatCompletions'"
        status: pass
    human_judgment: false
duration: 17min
completed: 2026-09-24
status: complete
---

# Phase 28 Plan 01: Tool Calling Boundary Contract Summary

**Closed OpenAI-compatible tool request normalization with a pure conversation ledger and early secret-safe HTTP rejection before routing.**

## Performance

- **Duration:** 17 min
- **Started:** 2026-09-24T12:54:14-07:00
- **Completed:** 2026-09-24T20:10:51Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments

- Added a closed `ToolChoice` model that preserves omission separately from `auto`, `none`, `required`, and named-function choices.
- Added immutable normalization for function definitions, opaque JSON Schema parameters, and assistant-call/tool-result settlement.
- Enforced tool protocol validation in the chat handler before service routing, while retaining text-only and mixed-content compatibility.
- Added stable, payload-safe `invalid_request` errors plus reusable tool capability classifications for later routing preflight.

## Task Commits

1. **Task 1: Write the red boundary contract and failure-mode matrix first** - `652a7f4e` (`test`)
2. **Task 2: Implement the normalized public contract and conversation ledger** - `ba3fc63d` (`feat`)
3. **Task 3: Wire early handler validation and preserve compatible requests** - `e595f05d` (`feat`)

**Plan metadata:** skipped (`commit_docs` disabled)

## Files Created/Modified

- `internal/llm/tool_protocol.go` - Closed tool-choice normalization, definition validation, pending-call ledger, and requirements derivation.
- `internal/llm/types.go` - Typed raw JSON parameters and downstream protocol requirements.
- `internal/http/handlers/chat.go` - One-time public decoding and validation before gateway service invocation.
- `internal/errors/errors.go` - Stable protocol error constructor and non-provider-health client classification.
- `internal/llm/tool_protocol_test.go` - Intentional RED contract test followed by focused passing coverage.
- `internal/http/handlers/chat_tool_test.go` - Handler no-upstream-call, compatibility, ordering, and payload-safety coverage.
- `internal/http/handlers/chat_test.go` - Updated existing fixture capability for legal tool-call compatibility coverage.

## Decisions Made

- `ToolChoice` is a closed model with a nil pointer for omission; explicit `auto` is a distinct value.
- Function names use Unicode whitespace trimming only for blank detection and exact decoded string comparison without case folding or normalization.
- The handler validates the public protocol once and passes derived `ToolProtocolRequirements` downstream for later capability preflight.
- Official OpenAI Chat Completions, Anthropic Messages, and Gemini GenerateContent documentation plus pinned SDK types were rechecked before implementation; no material conflict with the locked protocol was found.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Preserved protocol requirements on the downstream request**
- **Found during:** Task 2
- **Issue:** Normalization derived `ToolProtocolRequirements`, but the handler initially discarded them before routing.
- **Fix:** Added `ToolRequirements` to `llm.LLMRequest` and assigned the derived immutable value once in the handler.
- **Files modified:** `internal/llm/types.go`, `internal/http/handlers/chat.go`
- **Verification:** Focused LLM and handler test commands pass.
- **Committed in:** `ba3fc63d`, `e595f05d`

**2. [Rule 2 - AGENTS.md constraint] Split the expanded handler into bounded helpers**
- **Found during:** Task 3
- **Issue:** The updated `ChatCompletions` function exceeded the repository's 50-line function limit.
- **Fix:** Extracted decoding, `LLMRequest` construction, and response encoding helpers without changing boundary ownership.
- **Files modified:** `internal/http/handlers/chat.go`
- **Verification:** `go test -timeout 60s ./internal/http/handlers -run 'TestChat.*Tool|TestChatCompletions'` passes.
- **Committed in:** `e595f05d`

---

**Total deviations:** 2 auto-fixed (2 Rule 2)
**Impact on plan:** Both changes were required to carry the planned routing input and comply with repository limits; no provider or execution scope was added.

## Issues Encountered

- The first direct handler suite invocation during the RED stage exceeded the local command window while warming Go build state; no result was used as verification. The final exact focused commands completed in 0.444 s and 0.186 s.
- `apply_patch` is not available in this environment, so the same scoped edits were applied through the available filesystem mechanism and formatted with `gofmt`.

- requirements.mark-complete TOOL-F01 found no matching requirement entry in .planning/REQUIREMENTS.md; implementation is complete, but the planning registry needs reconciliation before phase-level requirement closure.

## User Setup Required

None - no external service configuration required.

## Remote Test Environment

None - the Plan 28-01 contract and handler tests are deterministic and offline. `.env.local` was not sourced, no SSH host was contacted, and no remote test component was enabled.

## Next Phase Readiness

Plan 28-02 can consume `LLMRequest.ToolRequirements` for capability preflight without reparsing the public request. Provider response normalization, stream lifecycle, tool execution, MCP, and agent dispatch remain out of scope.

## Self-Check: PASSED

- `internal/llm/tool_protocol.go`, `internal/llm/tool_protocol_test.go`, and `internal/http/handlers/chat_tool_test.go` exist.
- Task commits `652a7f4e`, `ba3fc63d`, and `e595f05d` exist in Git history.