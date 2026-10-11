---
status: awaiting_human_verify
trigger: "修正已经确定网关兼容性缺陷"
created: 2026-10-02
updated: 2026-10-02
source_revision: c8849c98aae1a57c9c0b04e21e19896e82c33cd8
execution: inline
---

# Debug Session: Gateway protocol compatibility

## Symptoms

- Expected: real GPT streams accept a final usage-only chunk and settle once; Gemini tool results continue successfully with the original opaque signature.
- Actual: GPT streams return provider_bad_response after finish_reason; Gemini continuation returns HTTP 400 provider_invalid_request.
- Reproduction: existing real HTTP App.New tests with env.local GPT/GEM providers; local embedding only uses the user-selected model.
- Prior red evidence: ship-e2e-20261001/final-refactor-gpt/TestLiveStreamSettlement.log and gem-final/TestLiveToolRequired.log.
- Baseline direct-provider controls: GPT finish frame 12, usage frame 13, DONE; native Gemini succeeds with complete content, fails after removing only signature.

## Current Focus

- hypothesis: OpenAI adapter confuses generation finish with stream finish; Gemini conversion discards provider metadata in both complete and stream tool paths.
- test: real ordinary/buffered/Fusion/tool streams, all tool-choice continuations, persisted Usage and balances; malformed continuation metadata must fail before forwarding.
- expecting: original compatibility failures disappear without accepting content/tool deltas after finish or losing signature identity.
- next_action: user verifies GPT SSE and Gemini tool continuation in their actual client before archiving this scoped debug session; no merge to main.
- authorization: user explicitly requested these fixes; symptoms and scope already supplied by this conversation. No extra symptom questionnaire or agent delegation is needed.

reasoning_checkpoint:
  hypothesis: "Generation finish is mistaken for SSE termination; Gemini conversion omits opaque function-call metadata from both response and continuation."
  confirming_evidence:
    - "Real GPT direct stream has finish, then empty-choice usage, then DONE; the original adapter rejects the usage frame."
    - "The same native Gemini call continues with its signature, but returns 400 when only that signature is removed."
  falsification_test: "Original parsers pass the same real gateway requests, or fixed parsers still fail with working direct-provider controls."
  fix_rationale: "Allow one validated usage-only trailer and preserve signatures across shared DTO/state/native Part conversion; retain fail-closed guards."
  blind_spots: "Provider availability and quota can change; real requests cannot exhaust every invalid upstream frame shape."
  candidate_causes:
    - "code: terminal guard and native/shared conversion lose protocol information."
    - "environment/config: provider quota, latency or model support could independently fail requests; direct controls separate these cases."
  and_gate: "Yes: the OpenAI bug requires a provider emitting a legal trailing usage frame; Gemini requires a model that validates tool signatures. Both real providers meet those conditions."

## Evidence

- timestamp: 2026-10-02
  observation: handleStreamData rejects any non-DONE data when terminal before JSON decode; normalizer already understands empty-choice usage chunks.
- timestamp: 2026-10-02
  observation: Gemini Part.ThoughtSignature is omitted from public ToolCall/ToolCallChunk and rebuilt history; toolstream state also reconstructs calls without metadata.
- timestamp: 2026-10-02
  observation: native signature preservation/removal controls isolate the cause; provider quota/latency issues are separate and remain outside these two repairs.
- timestamp: 2026-10-02
  observation: overlay-built original parsers reproduce GPT ordinary/tool SSE provider_bad_response and Gemini complete/stream signature loss with unchanged real regression assertions.
- timestamp: 2026-10-02
  observation: fixed GPT matrix passes 12/12; Gemini's 11 distinct gateway cases each have a passing record, including malformed signature rejection and actual continuation with two settlements.
- timestamp: 2026-10-02
  observation: repeat Gemini tool streaming had one pre-header 12-second timeout and one native APIError mapped to provider_error. Latest native stream and gateway stream both pass; these reliability failures remain recorded and unresolved.
- timestamp: 2026-10-02
  observation: product source hashes are unchanged across fixed binaries; later changes only correct a skipped negative test, make selected SKIP fail the runner, and add a native streaming diagnostic.

## Failure Boundaries

- Reject duplicate final usage frames and generation/tool content after finish; preserve cancellation and exactly-once settlement.
- Reject malformed/oversized signature metadata; retain opaque bytes without logging them; clone metadata at normalization/state/output boundaries.
- Preserve tool IDs, argument fragments, provider-independent routing and tool/cache bypasses; do not execute tools in the gateway.
- Keep historical measurements unchanged; full Phase 29 latency gate remains failed, so no merge to main.

## Resolution

root_cause: generation/transport lifecycle conflation and loss of Gemini tool signature metadata
fix: validated final-usage handling plus copy-on-write tool metadata preservation and native signature reconstruction
files_changed:
  - internal/providers/openai/adapter.go
  - internal/providers/openai/stream.go
  - internal/providers/gemini/tool_protocol.go
  - internal/providers/toolstream/state.go
  - internal/llm/types.go
  - internal/llm/tool_protocol.go
  - internal/app/live_ship_protocol_test.go
  - internal/app/live_ship_gemini_diagnostic_test.go
  - .planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner/main.go
verification:
  scope: two confirmed protocol defects, not overall Phase 27-29 release acceptance
  target_test: { result: pass, evidence: actual GPT SSE settlement and Gemini complete/stream tool continuation }
  mutation_check: { result: skipped, reason_if_skipped: Go repository without configured Stryker; original-code overlay causal control used separately }
  no_op_deletion: { result: pass, deletion_justified_by_rca: true, explanation: stream code moved intact; only legal usage trailers added; opaque signature round trips restored; no assertion weakening }
  adjacent_tests:
    result: pass
    suites_run: [real tool_choice matrix, buffered, Fusion, cancellation GPT, authentication, real embedding/vector/cache versioning, malformed client signature]
    note: all scoped cases have successful records; later reliability failures occurred before continuation or in native SDK APIError mapping and remain unresolved outside protocol acceptance
  revert_and_reconfirm: { result: pass, bug_returned_on_revert: true, fixed_on_reapply: true, method: original-code Go build overlay; no working-tree restoration or stash }
  guardrail_verdict: accepted
  full_acceptance_verdict: failed
  unresolved: [Gemini repeated-stream timeout/API error cause, previous Phase 29 P95 gate, other historical acceptance gaps]
  human_verification: pending
  report: .planning/debug/gateway-protocol-compat-verification.md
