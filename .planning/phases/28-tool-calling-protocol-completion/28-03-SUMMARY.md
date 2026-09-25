---
phase: 28-tool-calling-protocol-completion
plan: "03"
subsystem: provider-tooling
tags: [go, streaming, tool-calling, provider-contracts, offline-tests]
requires:
  - phase: 28-01
    provides: normalized tool protocol types and request validation
provides:
  - Copy-on-write provider-neutral tool stream shadow state
  - Deterministic offline tool-contract harness reusable by provider fixtures
affects: [28-04, 28-05, 28-06, provider-adapters, stream-terminal]
actuals:
  tokens: 7745
  tasks: 3
  commits: 5
plan_head_before: c81a5d4fd339344b79684f919e756255e165e47c
tech-stack:
  added: []
  patterns: [copy-on-write stream shadow state, injected ID generation, callback-only contract harness]
key-files:
  created:
    - internal/providers/toolstream/state.go
    - internal/providers/toolstream/state_test.go
    - internal/providers/adaptertest/tool_contract.go
    - internal/providers/adaptertest/tool_contract_test.go
  modified: []
key-decisions:
  - "Tool-call shadow state remains side-effect free; Phase 27 retains all terminal ownership."
  - "Arguments are buffered as opaque bytes only until explicit completion and capped per unfinished call."
  - "The shared harness accepts first-turn tool requests and validates only malformed existing tool-result history."
patterns-established:
  - "Provider deltas establish immutable ID and name identity at first fragment; later fragments remain wire-opaque."
  - "Provider fixtures use a single completion observer rather than a second terminal finalizer."
requirements-completed: [TOOL-F01]
coverage:
  - id: D1
    description: Provider-neutral stream shadow state preserves identity, interleaving, opaque arguments, and safe completion.
    requirement: TOOL-F01
    verification:
      - kind: unit
        ref: go test -timeout 60s ./internal/providers/toolstream -run 'TestToolStream'
        status: pass
    human_judgment: false
  - id: D2
    description: Adapter-neutral offline harness covers tool choices, history, streams, errors, and one completion observer.
    requirement: TOOL-F01
    verification:
      - kind: unit
        ref: go test -timeout 60s ./internal/providers/adaptertest -run 'TestToolContract'
        status: pass
    human_judgment: false
duration: 16min
completed: 2026-09-25
status: complete
---

# Phase 28 Plan 03: Provider-Neutral Tool Stream Summary

**Copy-on-write tool-call stream shadow state with stable IDs, opaque argument completion checks, and a deterministic offline adapter contract harness**

## Performance

- **Duration:** 16 min
- **Started:** 2026-09-25T00:41:07Z
- **Completed:** 2026-09-25T00:56:49Z
- **Tasks:** 3
- **Files modified:** 4
- **Remote environment actions:** None. All verification used offline state and fixture tests; no credentials, SSH, provider call, or local substitute service was needed.

## Accomplishments

- Added immutable provider-neutral shadow state that immediately emits normalized first fragments, preserves supplied IDs, generates injected deterministic IDs when missing, supports interleaving, and rejects drift through generic `provider_bad_response` errors.
- Added completion-only JSON validation, stable ordered final `tool_calls` output, duplicate-completion rejection, and a bounded unfinished-argument buffer.
- Added one reusable offline contract runner for future OpenAI, Anthropic, and Gemini fixtures, covering five tool-choice modes, definitions, valid call/result history, non-streaming, streaming, secret-safe errors, and a single caller-owned completion observer.
- Preserved Phase 27 finalizer ownership: no edits were made to `internal/gateway/stream_terminal.go`, and neither new package invokes terminal, health, breaker, release, cancellation, retry, or usage-settlement effects.

## TDD Evidence

- **RED:** `go test -timeout 60s ./internal/providers/toolstream ./internal/providers/adaptertest -run 'TestToolStream|TestToolContract'` failed only on the intentionally absent `toolstream` and `adaptertest` interfaces.
- **GREEN:** The exact focused state and harness commands, plus the combined package matrix, passed with hard 60-second timeouts.
- **Additional RED/GREEN:** Added and first ran a failing bounded-buffer test before implementing the per-call argument limit.

## Task Commits

1. **Task 1: Write the red stream and shared-contract failure matrix first** - `44b7924` (test)
2. **Task 2: Implement copy-on-write stream shadow state and completion checks** - `de46166` (feat)
3. **Task 3: Build the reusable three-provider tool contract harness** - `92ed357` (feat)
4. **Rule 2 correction: Bound unfinished argument buffers** - `a1904d5` (fix)
5. **Rule 1 correction: Permit valid initial tool requests** - `6118dda` (fix)

**Plan metadata:** skipped because `commit_docs` is disabled.

## Files Created

- `internal/providers/toolstream/state.go` - Copy-on-write stream protocol state and completion result.
- `internal/providers/toolstream/state_test.go` - Deterministic identity, interleaving, privacy, completion, and buffer-limit coverage.
- `internal/providers/adaptertest/tool_contract.go` - Offline reusable tool protocol fixture runner and completion observer seam.
- `internal/providers/adaptertest/tool_contract_test.go` - Choice-matrix, stream, error-projection, history, and ownership coverage.

## Decisions Made

- The state API returns a new value on each transition and accepts an injected ID generator so identity behavior is deterministic and does not depend on provider data.
- Later stream fragments preserve their incoming wire shape rather than re-emitting stored identity fields; final calls retain the established ID and name.
- Existing tool-result history is validated only when supplied, allowing a valid first-turn tool request without forcing artificial history.
- D-33 was rechecked against the pinned local SDKs (Anthropic `v1.50.1`, Gemini `v1.60.0`) and official provider documentation. No material conflict with the provider-neutral fragment model was found.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Preserved later fragment wire shape**
- **Found during:** Task 2 focused green test
- **Issue:** The first implementation re-emitted stored identity fields in later argument fragments, violating the opaque-fragment contract.
- **Fix:** Emit later chunks from copied incoming fields while preserving identity only in shadow state and final output.
- **Files modified:** `internal/providers/toolstream/state.go`
- **Verification:** `TestToolStreamPreservesIdentityAndEmitsImmediately`
- **Committed in:** `de46166`

**2. [Rule 2 - Security] Bounded unfinished argument buffers**
- **Found during:** Final threat-model review
- **Issue:** Unfinished opaque argument buffers lacked an explicit size bound.
- **Fix:** Added a configurable per-call cap with a named 1 MiB default and a deterministic rejection test.
- **Files modified:** `internal/providers/toolstream/state.go`, `internal/providers/toolstream/state_test.go`
- **Verification:** `TestToolStreamBoundsUnfinishedArgumentBuffers`
- **Committed in:** `a1904d5`

**3. [Rule 1 - Bug] Allowed valid first-turn tool requests**
- **Found during:** Final contract review
- **Issue:** The harness incorrectly required prior tool-call history even when no tool result existed.
- **Fix:** Validate supplied tool-result references, but accept requests with no prior tool history.
- **Files modified:** `internal/providers/adaptertest/tool_contract.go`
- **Verification:** `go test -timeout 60s ./internal/providers/adaptertest -run 'TestToolContract'`
- **Committed in:** `6118dda`

**Total deviations:** 3 auto-fixed (2 Rule 1, 1 Rule 2).

## Editor and Environment Notes

- Native patch editing was not available in this execution surface. Safe Node REPL and PowerShell file writes were used instead; every fallback edit was followed by an affected-file `git diff` inspection, using `git diff --no-index` for newly created files.
- An initial Go test was denied access to the local build cache by the sandbox. The same offline test was rerun with permission and passed. This was not a missing development dependency.
- No remote environment action was taken or needed.

## Known Stubs

None.

## Next Phase Readiness

The state and fixture seams are ready for later provider-specific mapping plans. Provider adapters still need to call this state beside the existing Phase 27 finalizer; this plan deliberately did not add that integration.

## Self-Check: PASSED

- Confirmed all four created source/test files exist.
- Confirmed task commits `44b7924`, `de46166`, `92ed357`, `a1904d5`, and `6118dda` exist.
- Confirmed final focused Go tests passed and `git diff --check` reported no whitespace errors.

---
*Phase: 28-tool-calling-protocol-completion*
*Completed: 2026-09-25*
