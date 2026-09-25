---
phase: 28
plan: "04"
subsystem: provider-adapter
tags: [openai-compatible, tool-calling, sse, tdd]
requires:
  - 28-01 shared tool protocol types
  - 28-02 provider capability and request validation
  - 28-03 toolstream state and adapter contract harness
provides:
  - Lossless OpenAI-compatible tool request mapping
  - Validated complete and streaming tool-call normalization
  - Synthetic SSE coverage for bounded first-fragment delivery
  - Safe provider_bad_response handling for malformed tool payloads
affects: [internal/providers/openai]
tech_stack:
  added: []
  patterns:
    - Reuse toolstream.State for complete and stream normalization
    - Use httptest synthetic SSE rather than live providers
key_files:
  created: []
  modified:
    - internal/providers/openai/adapter.go
    - internal/providers/openai/adapter_tool_test.go
decisions:
  - Preserve request tools and tool choices through typed payload serialization.
  - Generate only missing provider tool-call IDs through the adapter-injected generator.
  - Treat malformed, incomplete, conflicting, or post-terminal tool streams as provider_bad_response.
  - Keep terminal ownership in the caller-facing stream contract; the adapter emits no secondary terminal owner.
metrics:
  duration: "approximately 18 minutes"
  completed: 2026-09-25
  tasks: 4
status: complete
plan_head_before: 6118ddabff507df44be1c88d642c01e248f03669
commits: 2
actuals:
  tokens: 7754
  tasks: 4
  commits: 2
---

# Phase 28 Plan 04: OpenAI-Compatible Adapter Summary

OpenAI-compatible tool calling now preserves the wire contract while normalizing complete and SSE tool-call responses through the shared protocol state.

## Completed Work

- Preserved lossless tools, all five tool_choice states, assistant tool_calls, role=tool history, and tool_call_id in complete and streaming request payloads.
- Validated complete calls with shared toolstream.State: missing IDs are generated, provider IDs are preserved, duplicate IDs, invalid JSON arguments, and finish-reason conflicts return safe provider_bad_response errors.
- Validated streaming tool fragments incrementally without buffering the full body. Incomplete EOF, malformed SSE, identity drift, finish conflicts, and post-terminal data fail safely.
- Added synthetic httptest coverage for prompt delivery of the first tool fragment and exercised the shared 28-03 adapter harness for complete and streaming paths.

## TDD Evidence

- RED commit: 3eb1dda1 test: add OpenAI tool protocol red fixtures.
- Intentional RED command: go test -timeout 60s ./internal/providers/openai -run ToolRequest|ToolChoice|History|Result|Schema|ToolComplete|ToolStream|ToolContract.
- RED failures: malformed complete tool calls and unsafe terminal streaming cases incorrectly completed without provider_bad_response.
- GREEN command: go test -timeout 60s ./internal/providers/openai -run ToolRequest|ToolChoice|History|Result|Schema|ToolComplete|ToolStream|ToolContract.
- GREEN result: pass.
- GREEN commit: 93adda1 feat: complete OpenAI-compatible tool protocol.

## Verification

- Passed: go test -timeout 60s ./internal/providers/openai.
- Passed: go vet ./internal/providers/openai.
- Passed: gofmt -l internal/providers/openai/adapter.go internal/providers/openai/adapter_tool_test.go with no output.
- Passed: source-only diff check. No changes to go.mod or go.sum.
- Passed: no logging calls in the scoped adapter/test files; provider response data is not logged.
- Attempted: go test -timeout 60s ./...
  It failed outside this plan scope because local Redis, PostgreSQL, and Qdrant services were unavailable, and unrelated gateway/HTTP integration tests timed out or lacked configured providers. The OpenAI provider package passed in the same run. No environment, credential, or remote-provider action was taken, per plan scope.

## D-33 Compatibility Check

Reviewed the current official OpenAI function-calling and chat-completions streaming documentation on September 25, 2026, alongside local SDK pins (anthropic-sdk-go v1.50.1, google.golang.org/genai v1.60.0; no OpenAI SDK is used). No material conflict with the plan was found. The adapter keeps the existing direct HTTP transport and does not alter dependencies.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Prevented a missing fallback model panic in streamed chunks**
- **Found during:** stream normalization implementation.
- **Fix:** use the request model as the stream fallback rather than indexing the adapter model list.
- **Files modified:** internal/providers/openai/adapter.go.
- **Commit:** 93adda1.

**2. [Rule 1 - Bug] Restored the context-aware stream send helper after refactoring the bounded reader**
- **Found during:** focused provider compilation.
- **Fix:** restored the helper and reran the focused suite.
- **Files modified:** internal/providers/openai/adapter.go.
- **Commit:** 93adda1.

## Known Stubs

None.

## Threat Flags

None. This plan adds no endpoint, authentication path, file access, schema, or external network surface.

## Scope Notes

- The user restricted planning changes to this summary. Existing modified and untracked Phase 28 planning artifacts were left untouched.
- commit_docs is disabled, so this summary is intentionally not committed.

## Self-Check: PASSED

- Found: internal/providers/openai/adapter.go
- Found: internal/providers/openai/adapter_tool_test.go
- Found commit: 3eb1dda1
- Found commit: 93adda1
