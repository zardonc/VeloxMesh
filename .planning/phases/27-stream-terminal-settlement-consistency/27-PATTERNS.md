# Phase 27: Stream Terminal and Settlement Consistency - Pattern Map

**Mapped:** 2026-09-20
**Research:** Disabled for this phase; map derived from `27-CONTEXT.md`, Phase 27 in `ROADMAP.md`, `REQUIREMENTS.md`, and current tracked source.
**Files analyzed:** 15 expected new/modified files, plus 8 reference-only files
**Analogs found:** 15 / 15 expected files

## File Classification

The finalizer and stream-split filenames are required by the revised plans. Existing package boundaries remain unchanged, but streaming implementation/tests must move out of oversized legacy files so every modified Go file is 500 lines or fewer.

| New/Modified File | Role | Data Flow | Closest Tracked Analog | Match Quality |
|---|---|---|---|---|
| `internal/gateway/stream_terminal.go` (new, recommended) | utility/service lifecycle | event-driven + transform | `internal/gateway/fusion.go:185-205,350-383` | exact structural match |
| `internal/gateway/stream_terminal_test.go` (new, recommended) | test | event-driven + concurrent race | `internal/gateway/service_test.go:510-635`; `internal/scheduler/client_test.go:151-180` | role/data-flow match |
| `internal/gateway/service.go` | service shell / move source | non-streaming + shared service wiring | existing streaming block moved to `service_stream.go` | exact move source |
| `internal/gateway/service_stream.go` (new, required) | service | streaming request-response | existing stream contexts/pumps/finalization at `service.go:630-853` | exact move target |
| `internal/gateway/fusion.go` | service | streaming fan-in + transform | its existing `fusionStreamResult.finish` at `fusion.go:350-383` | exact |
| `internal/http/handlers/chat.go` | controller/handler | SSE streaming request-response | existing SSE loop at `chat.go:133-203` | exact |
| `internal/http/handlers/chat_stream_test.go` (new, required) | test | SSE protocol + write failure | existing channel-close/error tests at `chat_test.go:117-160` | exact move/expansion target |
| `internal/errors/errors.go` | utility | error transform/classification | `TranslateError` and `AffectsProviderHealth` at `errors.go:65-93,115-134` | exact |
| `internal/errors/errors_test.go` | test | table-driven transform/classification | `errors_test.go:27-92,95-127` | exact |
| `internal/gateway/service_test.go` | test shell / move source | non-streaming service behavior | streaming fixtures/tests move to `service_stream_test.go` | exact move source |
| `internal/gateway/service_stream_test.go` (new, required) | test | streaming lifecycle + settlement + Fusion member compatibility | `service_test.go:510-653` | exact move/expansion target |
| `tests/integration/chat_test.go` | integration test shell / move source | non-streaming full-router behavior | streaming fixtures/tests move to `chat_stream_test.go` | exact move source |
| `tests/integration/chat_stream_test.go` (new, required) | integration test | authenticated SSE request-response | `chat_test.go:399-492`; auth request helper at `chat_test.go:123-139` | exact move/expansion target |
| `.planning/phases/27-stream-terminal-settlement-consistency/27-PHASE-BASE.txt` (new, required) | verification evidence | immutable commit reference | `git rev-parse --verify HEAD` | exact repository primitive |
| `.planning/phases/27-stream-terminal-settlement-consistency/27-03-SUMMARY.md` | execution evidence | benchmark + scope evidence | GSD plan summary contract | exact workflow artifact |

## Pattern Assignments

### `internal/gateway/stream_terminal.go` (new utility/service lifecycle, event-driven)

**Primary analog:** `internal/gateway/fusion.go`

**Imports pattern** (`fusion.go:3-17`):

```go
import (
	"context"
	"sync"
	"time"

	"veloxmesh/internal/admission"
	"veloxmesh/internal/errors"
	"veloxmesh/internal/llm"
	"veloxmesh/internal/observability"
	"veloxmesh/internal/routing"
)
```

Use internal package imports directly; keep the finalizer private to `gateway`. Inject all path-specific inputs when constructing it. Do not instantiate repositories, health stores, breakers, or metrics implementations inside the finalizer.

**Exactly-once ownership pattern** (`fusion.go:185-205,350-383`):

```go
type fusionStreamResult struct {
	service       *Service
	ctx           context.Context
	req           *llm.LLMRequest
	comboDecision routing.RoutingDecision
	judgeDecision routing.RoutingDecision
	start         time.Time
	release       admission.ReleaseFunc
	trace         *observability.RequestTrace
	done          sync.Once
}

func (r *fusionStreamResult) finish(in streamFinishInput) {
	r.done.Do(func() {
		// health, breaker, model outcome, metrics, trace, release, settlement
	})
}
```

Copy the ownership idea, not the Fusion-specific fields. The shared finalizer must be the sole owner of:

- provider `EndRequest`, breaker `RecordResult`, and model outcome;
- request outcome metrics, request count, latency, and trace completion;
- admission `release`;
- successful Usage settlement or completed-without-Usage `missing_usage` logging.

The first call to `Submit(terminalCandidate)` wins and returns an immutable decision containing the winner, `Accepted=true`, and its normalized client terminal output. Every later submission returns the same winner with `Accepted=false`. Only the accepted decision may emit terminal output, close the downstream channel, or run the listed final side effects; no caller may perform those operations before or after submission on its own.

**Terminal outcome pattern:** define a compact private enum/struct representing exactly:

```go
type streamTerminalKind uint8

const (
	streamCompleted streamTerminalKind = iota
	streamProviderError
	streamClientCancelled
	streamPolicyRejected
	streamInternalError
)
```

The outcome should carry status, category, error, final Usage snapshot, TTFT, and whether provider health is affected. Derive settlement eligibility from `kind == streamCompleted`, never from `status == 200`.

**Error normalization pattern** (`errors.go:117-134`):

```go
func TranslateError(err error) *GatewayError {
	var gwErr *GatewayError
	if errors.As(err, &gwErr) {
		return gwErr
	}
	if errors.Is(err, context.Canceled) {
		return NewGatewayError("client_disconnected", "client disconnected", 499)
	}
	// ...
}
```

Normalize wrapped errors through `errors.TranslateError` before selecting status/category. Do not repeat direct `err.(*GatewayError)` assertions from `streamErrorStatus`; Fusion wraps errors with `%w`, so direct assertions lose the mapped category.

**Avoid:**

- a second finalizer for Fusion or buffered streams;
- `status == 200` as a proxy for successful completion;
- calling `settle`, `release`, health, breaker, metrics, or trace outside the finalizer;
- locks, repository calls, network calls, or model calls in per-chunk observation;
- mutating a shared `Usage` pointer after it becomes part of the terminal outcome.

---

### `internal/gateway/stream_terminal_test.go` (new test, event-driven/concurrent)

**Primary analogs:** `internal/gateway/service_test.go` and `internal/scheduler/client_test.go`

**Table/fake style** (`service_test.go:510-530`):

```go
p1 := &mockStreamAdapter{mockAdapter: mockAdapter{id: "p1"}, events: []llm.StreamEvent{
	{DeltaContent: "a", Usage: &llm.Usage{TotalTokens: 2}},
	{DeltaContent: "b", Usage: &llm.Usage{TotalTokens: 5}},
	{Done: true},
}}
// Consume the channel, then assert the final Usage record.
```

**Concurrent call-count assertion** (`internal/scheduler/client_test.go:151-180`):

```go
var calls int32
atomic.AddInt32(&calls, 1)
// ... invoke competing paths ...
if atomic.LoadInt32(&calls) != 1 {
	t.Fatalf("expected exactly one call, got %d", calls)
}
```

Build counting fakes for settlement, Usage log, release, health end, breaker result, metrics/trace hooks where injection exists. Race Done, EOF, provider error, cancellation, and policy rejection with a start barrier; assert one winning outcome and one call per owned side effect. Use `t.Cleanup`, bounded channels, and deterministic synchronization instead of sleeps.

Keep the focused benchmark here if a new file is used. No repository benchmark/`AllocsPerRun` analog exists, so use standard `testing.B`, `b.ReportAllocs()`, and a prebuilt event slice. Benchmark classification/forwarding separately from repository-backed finalization so the hot-path claim is measurable.

---

### `internal/gateway/service_stream.go` (service, ordinary and buffered streaming)

Move the existing streaming implementation and directly related helpers out of `service.go` into this same-package file. Move enough code to leave both files at 500 lines or fewer; this is a responsibility-preserving move, not an unrelated service refactor.

**Existing context carrier** (`service.go:721-743`):

```go
type streamRuleContext struct {
	ctx      context.Context
	req      *llm.LLMRequest
	decision routing.RoutingDecision
	respMeta *llm.LLMResponse
	events   <-chan llm.StreamEvent
	start    time.Time
	release  admission.ReleaseFunc
	attempts int
	trace    *observability.RequestTrace
}
```

Reuse this request-scoped dependency bundle or replace it with the shared finalizer input. Do not add parallel parameter lists to ordinary and buffered helpers.

**Last-Usage observation pattern** (`service.go:642-660`; buffered equivalent `service.go:774-800`):

```go
for event := range ch {
	if event.Usage != nil {
		finalUsage = event.Usage
	}
	if event.Error != nil {
		streamErr = event.Error
	}
	outCh <- event
}
```

Preserve “last observed Usage wins”, but copy the value into an immutable snapshot before finalization. Usage observed before provider error, policy rejection, cancellation, or write failure remains diagnostic only and must not reach `Service.settle`.

**Current duplicated finalization to replace** (`service.go:819-853`):

```go
func (s *Service) finishStreamRequest(f streamFinish) {
	// provider health, breaker, model outcome, metrics, trace
	f.release()
	if f.status == 200 {
		s.settle(f.ctx, f.req, f.decision, f.usage, latency)
	}
}
```

Delegate this method to the shared finalizer or remove it. The non-buffered goroutine at `service.go:630-712` must also use the same object; it currently duplicates all final side effects inline.

**Ordinary stream pump constraints:**

- select on `ctx.Done()` while receiving upstream events and while sending to the downstream channel;
- provider `Error` and explicit `Done` submit terminal candidates immediately; only the accepted decision's normalized output may be forwarded;
- clean upstream channel close without `Done` is successful completion;
- on terminal, the accepted decision runs final effects, emits its normalized output, and closes downstream exactly once; drain upstream only if required to prevent a provider goroutine leak, and never let draining own finalization;
- cancellation/write-failure context must win as `499/client_cancelled` when observed first.

**Buffered response-policy constraint:** upstream Done/EOF means provider collection completed, but request finalization waits until `ProcessResponse` finishes. A policy rejection after successful buffering is the request terminal outcome, so it must not be preempted by the provider's Done marker and must not settle Usage.

**Settlement pattern to keep behind finalizer** (`service.go:73-108`):

```go
if usage != nil {
	record.PromptTokens = usage.PromptTokens
	record.ResponseTokens = usage.CompletionTokens
	record.TotalTokens = usage.TotalTokens
} else {
	record.Status = controlstate.SettlementStatusMissingUsage
}
if record.Status == controlstate.SettlementStatusMissingUsage {
	_ = s.repo.Usage().Log(context.Background(), record)
	return
}
_ = s.repo.Settle(context.Background(), record)
```

Call this only for `streamCompleted`. Do not change repository contracts or add a new settlement status.

---

### `internal/gateway/fusion.go` (service, Fusion streaming)

**Existing reusable once pattern** (`fusion.go:350-383`): use this as the behavioral baseline, then delegate to the shared finalizer.

**Immutable Usage augmentation pattern** (`fusion.go:271-286`):

```go
if event.Usage != nil {
	usage := *event.Usage
	usage.PromptTokens += promptTokens
	usage.CompletionTokens += completionTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	event.Usage = &usage
}
```

Preserve this copy-before-modify pattern. It satisfies the repository's immutability convention and prevents mutation of provider-owned Usage objects.

**Changes to align:**

- replace `fusionStreamResult.done sync.Once` and its side-effect body with the shared finalizer;
- make `forward()` obey the same Done/error/EOF/context rules as ordinary streaming;
- use the judge provider/model for health, breaker, metrics, and trace, while using the combo decision for client Usage settlement, matching current `finish` behavior;
- keep member request finalization separate: member calls are non-streaming provider operations and are not the client stream terminal owner;
- ensure buffered Fusion policy rejection is classified `response-policy rejected`, health-neutral, and unsettled.

Do not duplicate a Fusion-specific terminal enum or recreate the finalizer fields in `fusionStreamResult`.

---

### `internal/http/handlers/chat.go` (controller, SSE boundary)

**Current SSE boundary** (`chat.go:133-203`):

```go
flusher, ok := w.(http.Flusher)
// ...
for event := range ch {
	if event.Error != nil {
		fmt.Fprintf(w, "event: error\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		break
	}
	if event.Done {
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		break
	}
	_ = encoder.Encode(chunk)
}
```

Retain the protocol shape, but route all writes through small helpers that return errors. Check every `Write`/`Fprintf`/`Encode` result. `Flush` has no error return, so the preceding write is the failure boundary.

Create a derived request context before calling `HandleChatCompletionStream`:

```go
streamCtx, cancel := context.WithCancel(r.Context())
defer cancel()
```

On the first downstream write failure, call `cancel()` and return immediately. Do not emit `event:error` or `[DONE]` after a failed write. The gateway stream pump must observe the cancellation and finalize `499/client_cancelled` without provider-health impact.

SSE terminal rules:

- provider error: exactly one `event: error` frame followed by exactly one `[DONE]` if both writes succeed;
- completed Done or clean EOF: exactly one `[DONE]`;
- client cancellation or downstream write failure: stop writes, no synthetic provider error, no trailing `[DONE]` after failure;
- ignore duplicate terminal events because the service channel must close after the first terminal outcome; keep a defensive `sentDone` guard in the handler.

Do not move provider health, settlement, or metrics into the handler. Its only lifecycle signal back to the service is context cancellation.

---

### `internal/http/handlers/chat_stream_test.go` (test, SSE protocol/write boundary)

**Existing setup pattern** (`chat_test.go:117-160`): construct a local adapter, in-memory health store, registry/router/service, wrap handler with `middleware.RequestID`, and call it with `httptest.NewRequest`.

Extend this pattern with:

- exact counts using `bytes.Count(body, []byte("data: [DONE]"))` and `bytes.Count(body, []byte("event: error"))`;
- Done followed by extra events/close, error followed by Done/close, and clean EOF;
- a custom `http.ResponseWriter` + `http.Flusher` wrapper whose `Write` fails after a configured number of writes;
- a cancellation-aware stream adapter that exposes a channel closed when its context is cancelled, proving prompt release and no post-failure writes.

No existing failing `ResponseWriter` test double exists in the repository. Keep it local to this test file and minimal; do not add production abstractions solely for testing.

---

### `internal/errors/errors.go` (utility, error transform/classification)

**Provider-health pattern** (`errors.go:65-93`):

```go
func AffectsProviderHealth(err error) bool {
	gwErr, ok := err.(*GatewayError)
	if !ok {
		return true
	}
	switch gwErr.Code {
	case ProviderInvalidRequest:
		return false
	// provider categories...
	}
	return true
}
```

Update this pattern rather than adding classification switches in gateway paths:

- canonicalize cancellation as `client_cancelled` with HTTP 499;
- classify `client_cancelled` and `policy_blocked` as health-neutral;
- use `errors.As` so wrapped `GatewayError` values retain category and health semantics;
- preserve provider categories and their current health behavior.

`TranslateError` is the single mapping boundary for `context.Canceled`. Do not retain both `client_disconnected` and `client_cancelled` for the same condition unless compatibility evidence requires an alias internally; the locked Phase 27 category is `client_cancelled`.

---

### `internal/errors/errors_test.go` (test, classification)

**Table-driven pattern** (`errors_test.go:27-92,95-127`):

```go
tests := []struct {
	name     string
	err      error
	expected bool
}{ /* cases */ }
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) { /* assertion */ })
}
```

Add cases for raw and wrapped `context.Canceled`, canonical 499/code translation, raw/wrapped `ErrPolicyBlocked`, each provider category, and unknown/internal errors. Keep retryability assertions separate from provider-health assertions.

---

### `internal/gateway/service_stream_test.go` (test, path parity and settlement)

Move all streaming fixtures/tests plus enough directly related helpers out of `service_test.go` so both test files are 500 lines or fewer. Keep the existing Fusion member fixture here and make S-K02 unconditional.

**Reuse existing fakes:**

- `mockRepoWithUsage` (`service_test.go:313-330`) for settlement capture;
- `mockStreamAdapter` (`service_test.go:637-653`) for deterministic event sequences;
- in-memory health snapshots (`service_test.go:582-599`) for health-neutral policy assertions;
- Fusion setup (`service_test.go:602-635`) for path parity.

Upgrade `mockRepoWithUsage` from booleans to counts and capture both `Settle` and `Usage().Log` records. Add a counting admission controller implementing the existing `admission.Controller` interface so release count is observable.

Use one shared table of terminal scenarios and run each scenario through ordinary, buffered, and Fusion adapters where applicable. Required assertions per scenario are: winning kind/status/category, settle count, missing-usage log count, release count, health failure delta, breaker/model outcome, and final Usage snapshot.

Cover explicit Done, clean EOF, empty stream, Usage-only EOF, multiple Usage events, provider error before/after content, health-affecting/non-affecting provider errors, cancellation before/after content, response-policy rejection, duplicate terminal events, and terminal races. Non-streaming regression stays in this file but should not be rewritten.

---

### `tests/integration/chat_stream_test.go` (integration test, authenticated endpoint compatibility)

Move streaming integration fixtures/tests plus enough directly related helpers from `tests/integration/chat_test.go` so both files are 500 lines or fewer. Non-streaming compatibility remains in the legacy file or is shared through small same-package helpers.

**Authenticated request pattern** (`tests/integration/chat_test.go:399-443`):

```go
req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
req.Header.Set("Authorization", "Bearer "+application.Config.DevAPIKey)
rec := httptest.NewRecorder()
application.Router.ServeHTTP(rec, req)
```

Keep this full-router path for endpoint compatibility. Add table cases for missing Authorization, invalid bearer token, and valid key. Missing/invalid must remain 401 before provider execution; valid streaming must remain 200 `text/event-stream` with one terminal sequence.

Replace the ambiguous cancellation expectations at `tests/integration/chat_test.go:482-490` with deterministic assertions: provider context is cancelled promptly, response contains no terminal frame after the cancellation/write boundary, and no settlement/debit occurs. Use local `httptest.Server` and fakes only; no live provider.

---

## Shared Patterns

### One Authoritative Terminal Outcome

**Source:** `internal/gateway/fusion.go:350-383`
**Apply to:** ordinary, buffered, and Fusion streaming.

The finalizer is request-scoped and exactly-once. Transport adapters classify candidate outcomes and submit them; only the winning outcome drives all terminal side effects. Path-specific code may transform events or choose provider/model metadata, but it must not own lifecycle effects.

### Client Cancellation and Write Failure

**Sources:** `internal/errors/errors.go:117-134`; `internal/http/handlers/chat.go:133-203`
**Apply to:** handler and both stream pumps.

Use a derived cancellable context at the SSE boundary. Raw request cancellation and downstream write failure both become `499/client_cancelled`, release promptly, do not emit a provider error, do not affect provider health/breaker, and do not settle Usage.

### Usage Settlement

**Sources:** `internal/gateway/service.go:73-108`; `internal/controlstate/types.go:127-155`
**Apply to:** the shared finalizer only.

The last observed Usage is diagnostic state until successful completion wins. Completed + Usage calls `Repository.Settle` once. Completed + no Usage logs one existing `missing_usage` record. Every other terminal kind calls neither debit path. No repository or schema changes are needed.

### SSE Protocol

**Sources:** `internal/http/handlers/chat.go:147-203`; existing tests moved from `internal/http/handlers/chat_test.go:117-160` into `chat_stream_test.go`
**Apply to:** HTTP handler only.

Keep OpenAI-compatible chunk JSON and headers unchanged. Count terminal frames in tests, not just presence. A downstream write failure is a local transport terminal and prohibits any later write.

### Testing and Verification

**Sources:** existing tests moved from `internal/gateway/service_test.go:510-653` into `service_stream_test.go`; `internal/scheduler/client_test.go:151-180`; existing integration tests moved from `tests/integration/chat_test.go:399-492` into `chat_stream_test.go`
**Apply to:** all Phase 27 tests.

Prefer deterministic buffered channels, barriers, atomic counters, local `httptest` servers, and injected fakes. Every backend test command in PLAN.md must use `go test -timeout 60s ...` and state the failure signal. Include `go test -race -timeout 60s` only for the focused gateway package if it fits the hard timeout.

## Alignment Constraints

| Concern | Ordinary | Buffered | Fusion | Required Common Rule |
|---|---|---|---|---|
| Done | terminal completed | provider collection complete; wait for response policy | terminal completed after judge stream | finalizer sees completed only after path-specific validation |
| Clean EOF | completed | provider collection complete; wait for response policy | completed | same completed outcome |
| Provider error | preserve mapped status/category | preserve mapped status/category | preserve wrapped judge status/category | health impact only from `AffectsProviderHealth` |
| Policy rejection | n/a unless response rules enabled | policy-rejected terminal | policy-rejected terminal | health-neutral, no settlement |
| Client cancellation | 499 | 499 | 499 | release once, no health/breaker failure, no settlement |
| Downstream write failure | context cancellation -> 499 | replay write failure -> 499 | context cancellation -> 499 | stop writes and forwarding immediately |
| Usage | last observed | last collected | last combined Usage | settle once only on completed; otherwise diagnostic |
| Final side effects | shared finalizer | shared finalizer | shared finalizer | each at most once |

## Logic That Must Not Be Duplicated

- terminal kind/status/category classification;
- provider-health and breaker success/failure derivation;
- metrics/trace/request-count completion;
- admission release;
- completed-only settlement and `missing_usage` logging;
- cancellation/write-failure mapping;
- SSE terminal marker counting rules.

## Reference-Only Files

| File | Use | Expected Change |
|---|---|---|
| `internal/llm/types.go:151-166` | Existing `Usage` and `StreamEvent` shape | none; no public/internal event field required |
| `internal/controlstate/types.go:127-155` | Existing `missing_usage` status and Usage record | none |
| `internal/controlstate/repository.go:11-29,74-76` | Existing settlement/log contracts | none |
| `internal/controlstate/sqlite/repository.go:715-729` | Missing-usage log implementation | none |
| `internal/controlstate/postgres/repository.go:77-150` | Debit/settlement implementation | none |
| `internal/http/middleware/logging.go:9-23` | Confirms wrapped writer preserves `Write` and `Flush` | none unless tests reveal interface forwarding issue |
| `internal/http/middleware/auth_test.go:13-65` | Existing valid/invalid API-key test style | none |
| `internal/admission/controller.go:18-26` | Release/controller interface for counting fake | none |

## No Analog Found

| Needed Pattern | Reason | Planner Guidance |
|---|---|---|
| Failing/flaky `http.ResponseWriter` test double | No repository test writer returns write errors | Add a minimal local wrapper in `internal/http/handlers/chat_stream_test.go` |
| Stream forwarding allocation benchmark | No existing `Benchmark*`, `ReportAllocs`, or `AllocsPerRun` test was found | Add a focused standard Go benchmark next to finalizer tests; do not benchmark external settlement I/O |

## Metadata

**Analog search scope:** `internal/gateway`, `internal/http/handlers`, `internal/http/middleware`, `internal/errors`, `internal/controlstate`, `internal/admission`, `internal/scheduler`, `tests/integration`

**Tracked-source gate:** All named analog source paths were verified with `git ls-files`; no `.gsd` capability mirror or ignored runtime/install path is referenced.

**Primary symbols traced with CodeGraph:** `Service.HandleChatCompletionStream`, `streamRuleContext`, `finishStreamRequest`, `collectBufferedStream`, `fusionStreamResult.forward`, `fusionStreamResult.finish`, `ChatHandler.ChatCompletions`, `errors.TranslateError`, `errors.AffectsProviderHealth`, `controlstate.Repository.Settle`.

**Pattern extraction date:** 2026-09-20
