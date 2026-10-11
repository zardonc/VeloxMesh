# Phase 28: Tool Calling Protocol Completion - Pattern Map

**Mapped:** 2026-09-23
**Files analyzed:** 24 new/modified files
**Analogs found:** 24 / 24

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/llm/types.go` | model | transform | `internal/llm/types.go` | exact, extend in place |
| `internal/llm/tool_protocol.go` | model/utility | request-response + state validation | `internal/llm/types.go` | role-match |
| `internal/http/handlers/chat.go` | controller | request-response | `internal/http/handlers/chat.go` | exact, extend in place |
| `internal/http/handlers/chat_stream.go` | controller/serializer | streaming | `internal/http/handlers/chat_stream.go` | exact, preserve serializer |
| `internal/errors/errors.go` | utility | request-response | `internal/errors/errors.go` | exact, add stable constants |
| `internal/providers/toolstream/state.go` | utility/state machine | streaming + transform | `internal/gateway/stream_terminal.go` | data-flow match |
| `internal/providers/openai/adapter.go` | service/adapter | request-response + streaming | `internal/providers/openai/adapter.go` | exact, harden in place |
| `internal/providers/anthropic/adapter.go` | service/adapter | request-response + streaming | `internal/providers/openai/adapter.go` plus local SDK mapping | role-match |
| `internal/providers/gemini/adapter.go` | service/adapter | request-response + streaming | `internal/providers/openai/adapter.go` plus local SDK mapping | role-match |
| `internal/providers/capabilities.go` | model/utility | transform | `internal/providers/capabilities.go` | exact, extend in place |
| `internal/providers/catalog.go` | service/catalog | CRUD-like lookup/filter | `internal/providers/catalog.go` | exact, extend in place |
| `internal/routing/router.go` | service/router | request-response | `internal/providers/capabilities.go` | role + flow match |
| `internal/gateway/service.go` | service/orchestrator | request-response | `internal/gateway/service.go` | exact, add protocol preflight/cache guard |
| `internal/gateway/service_stream.go` | service/orchestrator | streaming | `internal/gateway/service_stream_terminal.go` | flow-match |
| `internal/gateway/fusion.go` | service | batch/fan-out | `internal/gateway/service.go` preflight guards | partial; reject before entry |
| `internal/providers/adaptertest/tool_contract.go` | test harness | request-response + streaming | `internal/providers/adaptertest/harness.go` | exact harness extension |
| `internal/llm/tool_protocol_test.go` | test | transform/state validation | `internal/gateway/stream_terminal_test.go` | test-pattern match |
| `internal/providers/toolstream/state_test.go` | test | streaming state machine | `internal/gateway/stream_terminal_test.go` | exact flow-match |
| `internal/providers/openai/adapter_tool_test.go` | test | request-response + streaming | `internal/providers/openai/adapter_test.go` + shared harness | role-match |
| `internal/providers/anthropic/adapter_tool_test.go` | test | request-response + streaming | `internal/providers/anthropic/adapter_test.go` + shared harness | role-match |
| `internal/providers/gemini/adapter_tool_test.go` | test | request-response + streaming | `internal/providers/gemini/adapter_test.go` + shared harness | role-match |
| `internal/routing/router_tool_test.go` | test | request-response | `internal/gateway/stream_terminal_test.go` table tests | test-pattern match |
| `internal/gateway/service_tool_test.go` / `service_stream_tool_test.go` | test | request-response + streaming | `internal/gateway/service_stream_test.go` | role-match |
| `internal/http/handlers/chat_tool_test.go` / `tests/integration/chat_tools_test.go` | test/integration | HTTP + SSE | `internal/http/handlers/chat_test.go` / `tests/integration/chat_completions_test.go` | role-match |

File names proposed by research may be merged into existing test files when the existing file would remain below the 500-line project limit. Production responsibility should remain in the files above; do not create a parallel protocol layer outside `internal/llm` and `internal/providers/toolstream`.

## Pattern Assignments

### Public Contract and Boundary Validation

**Applies to:** `internal/llm/types.go`, `internal/llm/tool_protocol.go`, `internal/http/handlers/chat.go`, `internal/errors/errors.go`, and their tests.

**Primary analog:** `internal/llm/types.go`

**Typed value-object pattern** (lines 38-76):

```go
type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

type ToolCallChunk struct {
	Index    *int               `json:"index,omitempty"`
	ID       *string            `json:"id,omitempty"`
	Type     *ToolType          `json:"type,omitempty"`
	Function *FunctionCallChunk `json:"function,omitempty"`
}
```

Copy this enum-plus-struct style for `ToolChoiceKind`, a named-function choice, and `ToolProtocolRequirements`. Keep JSON-facing values explicit; replace `ToolChoice any` in both public and internal requests with the strong type.

**Boundary decode/early-return pattern** from `internal/http/handlers/chat.go` (lines 42-82):

```go
var pReq proxyReq
if err := json.NewDecoder(r.Body).Decode(&pReq); err != nil {
	sendError(w, "invalid_request", "Failed to parse JSON body", http.StatusBadRequest)
	return
}

if len(req.Messages) == 0 {
	sendError(w, "invalid_request", "Messages array is required", http.StatusBadRequest)
	return
}
```

Retain one boundary-owned validator before constructing/calling the gateway service. Unlike the current ignored content unmarshalling, every malformed content, tool, choice, call, and result state must return a stable 400 before routing.

**Structured error pattern** from `internal/errors/errors.go` (lines 9-26):

```go
type GatewayError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	HTTPStatus int               `json:"status"`
	Headers    map[string]string `json:"-"`
}

func NewGatewayError(code, message string, httpStatus int) *GatewayError {
	return &GatewayError{Code: code, Message: message, HTTPStatus: httpStatus}
}
```

Add low-cardinality constants for `unsupported_tool_calling` and `unsupported_tool_choice`; never include tool arguments, results, call IDs, or tenant-defined names in messages.

---

### Shared Stream Identity State

**Applies to:** `internal/providers/toolstream/state.go`, `state_test.go`, and all provider adapters.

**Analog:** `internal/gateway/stream_terminal.go` (lines 127-175)

```go
type terminalFinalizer struct {
	mu        sync.Mutex
	completed bool
	callbacks terminalFinalizerOptions
}

func (f *terminalFinalizer) Submit(outcome terminalOutcome) bool {
	f.mu.Lock()
	if f.completed {
		f.mu.Unlock()
		return false
	}
	f.completed = true
	f.mu.Unlock()
	f.run(outcome)
	return true
}
```

Copy the explicit transition API and first-terminal-wins behavior, but honor the project immutability rule by returning a copied state or copied per-index map from each tool-fragment transition. Inject the missing-ID generator. The state must bind `index -> {id,type,name,arguments,completed}` on first observation, reject drift/reuse/post-completion fragments, and validate concatenated arguments with `json.Valid` at completion.

**Concurrency/duplicate test pattern** from `internal/gateway/stream_terminal_test.go` (lines 114-180): use atomic callback counts, release simultaneous submissions through a start channel, and assert exactly one accepted terminal transition. Reuse table-driven cases for duplicate IDs, unknown result IDs, name/index drift, invalid final JSON, and finish-reason conflicts.

---

### Provider Adapter Mapping

**Applies to:** the three provider adapters and provider-local tool tests.

**Transport/error analog:** `internal/providers/openai/adapter.go` (lines 106-189)

```go
if len(req.Tools) > 0 {
	openAIReq["tools"] = req.Tools
}
if req.ToolChoice != nil {
	openAIReq["tool_choice"] = req.ToolChoice
}

if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
	return nil, gatewayErr.NewGatewayError(
		gatewayErr.ProviderBadResponse,
		"Malformed JSON from provider",
		http.StatusBadGateway,
	)
}
```

Keep provider wire construction local, but feed normalized complete calls and stream fragments through the shared validator. Public contract errors are 400s; malformed upstream calls are `ProviderBadResponse`.

**OpenAI message preservation pattern** from `internal/providers/openai/adapter.go` (lines 76-95):

```go
if len(m.ToolCalls) > 0 {
	mapped["tool_calls"] = m.ToolCalls
}
if m.ToolCallID != "" {
	mapped["tool_call_id"] = m.ToolCallID
}
```

Anthropic and Gemini must reach semantic parity with this history preservation rather than flattening messages to text.

**Anthropic stream wire pattern** from `internal/providers/anthropic/adapter.go` (lines 331-375): preserve `content_block_start` as the identity fragment and `input_json_delta.partial_json` as opaque argument fragments. Add `content_block_stop` finalization and message stop-reason consistency through the shared state machine. Keep SDK errors mapped through the existing `mapError` switch (lines 274-295).

**Gemini schema/wire pattern** from `internal/providers/gemini/adapter.go` (lines 140-160):

```go
funcs = append(funcs, &genai.FunctionDeclaration{
	Name:                 t.Function.Name,
	Description:          t.Function.Description,
	ParametersJsonSchema: t.Function.Parameters,
})
config.Tools = []*genai.Tool{{FunctionDeclarations: funcs}}
```

Extend the same builder with `ToolConfig`, model `FunctionCall` history, and user `FunctionResponse` parts. Replace ordinal `call_%d` synthesis (lines 196-205 and 329-348) with upstream ID preservation plus injected opaque ID generation only when absent.

---

### Capabilities, Catalog, and Routing

**Applies to:** `internal/providers/capabilities.go`, `catalog.go`, `internal/routing/router.go`, `internal/gateway/fusion.go`, `internal/errors/errors.go`, and router tests.

**Deep-copy pattern** from `internal/providers/capabilities.go` (lines 50-78):

```go
func (c CapabilitySet) Clone() CapabilitySet {
	clone := CapabilitySet{
		ProviderType: c.ProviderType,
		Streaming:    c.Streaming,
		ToolCalling:  c.ToolCalling,
	}
	if c.SupportedOperations != nil {
		clone.SupportedOperations = make([]Operation, len(c.SupportedOperations))
		copy(clone.SupportedOperations, c.SupportedOperations)
	}
	return clone
}
```

Any slice/map added for supported choice modes must be defensively copied here and through `ModelProvider.Clone` in `catalog.go` lines 12-18.

**Fail-closed capability predicate** from `internal/providers/capabilities.go` (lines 91-113): use guard clauses returning false for every unmet requirement. Change the three booleans to an options/requirements object so streaming, protocol use, choice mode, and image requirements cannot drift between route modes.

**Catalog filtering pattern** from `internal/providers/catalog.go` (lines 98-110): return cloned eligible providers only. Apply exact-model overrides when constructing entries (lines 40-76), then make every router branch consume the effective capability set. Unknown support for required/named choices is ineligible, not assumed.

Fusion must be rejected from the shared preflight whenever `ToolProtocolRequirements.UsesProtocol` is true, before admission or provider I/O. Do not add tool-aware Fusion identity merging.

---

### Gateway Streaming and Terminal Ownership

**Applies to:** `internal/gateway/service.go`, `service_stream.go`, `fusion.go`, gateway tests, and integration tests.

**Single-owner terminal pattern** from `internal/gateway/service_stream_terminal.go` (lines 39-58):

```go
state.finalizer = newTerminalFinalizer(terminalFinalizerOptions{
	providerHealth:         state.recordHealth,
	circuitBreaker:         state.recordBreaker,
	metrics:                state.recordMetrics,
	traceClose:             state.recordTrace,
	settlementDecision:     state.recordSettlement,
	admissionRelease:       state.releaseAdmission,
	clientTerminalEmission: state.recordClientResult,
})

func (t *streamTerminalState) complete(err error, usage *llm.Usage, ttft time.Duration) bool {
	return t.finalizer.Submit(classifyTerminal(err, usage))
}
```

All normalized protocol failures must enter this existing completion path. Do not add adapter-owned health, release, settlement, or final client emission.

**Immediate SSE serialization pattern** from `internal/http/handlers/chat_stream.go` (lines 93-111):

```go
Choices: []llm.ChunkChoice{{
	Index: 0,
	Delta: llm.Delta{Content: event.DeltaContent, ToolCalls: event.ToolCalls},
	FinishReason: finishReason,
}},
```

The handler already serializes normalized tool chunks correctly. Preserve it and make the gateway bypass response-rule collection before the first event when protocol use is detected.

**Cache guard placement:** derive protocol requirements after request pipeline validation and before `service.go` cache lookup (current lines 135-195). Apply the same predicate to cache store (lines 341-349). This is a narrow read/write bypass; do not redesign cache keys in this phase.

## Shared Test Patterns

### Provider-Neutral Contract Harness

**Source:** `internal/providers/adaptertest/harness.go` (lines 13-47, 97-166)

```go
type SuccessCase struct {
	Name                 string
	Request              *llm.LLMRequest
	SetupFake            func()
	AssertRequest        func(t *testing.T)
	ExpectedFinishReason string
}

func RunConformance(t *testing.T, spec ConformanceSpec) {
	t.Run("Success", func(t *testing.T) {
		for _, sc := range spec.SuccessCases {
			t.Run(sc.Name, func(t *testing.T) {
				resp, err := spec.Adapter.Complete(context.Background(), sc.Request)
				if err != nil { t.Fatalf("unexpected error: %v", err) }
			})
		}
	})
}
```

Create `tool_contract.go` as an additive harness with provider-neutral scenario inputs and callback assertions for provider wire shapes. Provider packages supply fixtures; the shared harness owns expected normalized behavior. Reuse `AssertGatewayError` and `AssertSecretSafeError` (lines 168-189).

### HTTP and SSE Contract Tests

Use `httptest.NewRequest`/`httptest.NewRecorder`, decode the structured body, and assert both status and stable error code. For streaming, assert the first tool identity chunk is observable before later argument/completion events, not merely that replayed content is unchanged.

### Required Test Matrix

- Boundary: all five choice states, duplicate/blank/undeclared names, malformed call arguments, and full settlement ordering.
- State machine: preserved/generated IDs, interleaved indexes, drift/reuse, valid/invalid final JSON, and terminal conflicts.
- Providers: definitions, all choice modes, mixed text plus calls, history/results, non-stream and stream, provider-specific IDs.
- Routing: automatic, explicit override, round-robin, capacity-auto, no candidate, and Fusion preflight before side effects.
- Gateway: cache/rule bypass, pre-visible versus post-visible failure, terminal exactly once, Usage retained, and no sensitive payload logging.
- Integration: public JSON/SSE equivalence across text-only regression and tool-protocol scenarios.

## Shared Patterns

### Validation Ownership

HTTP owns public structural/conversation validation. Adapters own untrusted provider response validation. Routing owns capability compatibility. Gateway terminal code owns lifecycle side effects.

### Immutability and Injection

Return defensive copies from capabilities/catalog/state transitions. Inject the missing-ID generator and provider fakes; do not call constructors for implementations inside protocol business logic.

### Error and Privacy Contract

Use `GatewayError` with stable codes and generic messages. Never record raw schemas, arguments, results, prompts, call IDs, or tool names in logs/metrics. Error labels remain low-cardinality.

### Streaming Performance

Emit identity and argument fragments immediately. Keep state proportional to unfinished calls only. Do not add network I/O, whole-stream buffering, or per-fragment logs.

## No Analog Found

No file is completely without an analog. The new toolstream state machine has no existing tool-specific implementation, but the Phase 27 terminal state/finalizer supplies the repository's concurrency, transition, and exactly-once pattern. Provider wire details must follow the pinned SDK shapes documented in `28-RESEARCH.md`.

## Tracked-Source Verification

All named existing analogs were verified with `git ls-files -- <path>`. Proposed new files are under tracked source directories and are not capability mirrors or generated runtime copies. No `.gsd/capabilities/...` path is used.

## Metadata

**Analog search scope:** `internal/llm`, `internal/http/handlers`, `internal/providers`, `internal/routing`, `internal/gateway`, `internal/errors`, `tests/integration`

**Files scanned:** 62 through CodeGraph; 17 final analog paths verified as git-tracked

**Strong analog set:** `internal/llm/types.go`, `internal/http/handlers/chat.go`, `internal/providers/openai/adapter.go`, `internal/providers/adaptertest/harness.go`, `internal/gateway/stream_terminal.go` plus its service integration

**Pattern extraction date:** 2026-09-23
