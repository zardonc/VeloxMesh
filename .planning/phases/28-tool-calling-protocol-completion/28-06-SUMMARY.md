---
phase: 28-tool-calling-protocol-completion
plan: "06"
subsystem: providers
status: complete
completed_at: 2026-09-25
---

# Plan 28-06 Summary

## Delivered

- Completed Gemini tool protocol mapping for full function schemas, all public tool-choice modes, assistant FunctionCall history, and correlated user FunctionResponse results.
- Preserved upstream function-call IDs, generated opaque IDs only when absent, rejected invalid or ambiguous provider shapes, and normalized valid calls to `tool_calls`.
- Emitted complete Gemini streamed function calls immediately through the shared `toolstream` validator; no partial argument fragments are fabricated.
- Kept Usage and finish-reason normalization provider-local, without owning gateway terminal settlement.
- Replaced positional Gemini adapter construction with `AdapterConfig` and explicitly selected `genai.BackendGeminiAPI` so inherited environment settings cannot select a different SDK backend.

## SDK Recheck

- Pinned module remains `google.golang.org/genai v1.60.0`; `go.mod` and `go.sum` are unchanged.
- Existing phase research and the installed module cache confirm `FunctionCall.ID`, `FunctionResponse.ID`, full `ParametersJsonSchema`, `ToolConfig`, `AUTO`/`ANY`/`NONE`, and `AllowedFunctionNames` are available in the pinned SDK.
- Gemini streams expose complete FunctionCall values, not incremental argument fragments. Deterministic fixtures remain the acceptance evidence; no live-provider calls were made.

## Verification

Passed with the required 60-second timeout:

`go test -timeout 60s ./internal/providers/gemini`

`go test -timeout 60s ./internal/providers/adaptertest ./internal/providers/gemini -run 'Tool|Conformance|Stream'`

`go test -timeout 60s ./internal/app ./internal/controlstate -run '^$'`

Also passed:

`go vet ./internal/providers/gemini`

`gofmt -l internal/providers/gemini/adapter.go internal/providers/gemini/tool_protocol.go internal/providers/gemini/adapter_test.go internal/providers/gemini/adapter_tool_test.go`

`git diff --check` and `git diff --exit-code -- go.mod go.sum`

## Scope Notes

- No dependency was added or upgraded.
- No test-environment service or provider credential was required for this deterministic provider slice.
- Gateway routing, Fusion, cache behavior, tool execution, and Phase 27 terminal settlement remain owned by subsequent Phase 28 work.
