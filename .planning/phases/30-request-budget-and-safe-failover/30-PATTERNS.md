# Phase 30: Existing Patterns and Integration Map

**Mapped:** 2026-10-17 | **Input:** `30-CONTEXT.md`, current source; CodeGraph first.
**Verification:** static mapping only; no tests, external calls, SDK-default claims, or product changes.
**Scope:** TIMEOUT-F01; preserve D-02 conservative replay and D-03 terminal ownership.

## 1. Entry context and configuration

- `internal/http/handlers/chat.go:43` — `ChatHandler.ChatCompletions`: decode, normalize tools, then branch ordinary/stream; ordinary passes `r.Context()` at line 58.
- `internal/http/handlers/chat_stream.go:16` — `streamChatCompletions` derives a cancelable child and owns SSE forwarding until return; write failures call cancel at lines 73–85.
- `internal/config/config_file.go:9,153,163` — `fileConfig`, `applyFileConfig`, `applyRootFileConfig`; optional root settings and copying of provider policies at lines 184–188 are the config analog.
- `internal/config/config.go:10,144,442` — `LoadConfig`, `applyDefaults`, validation call; `config_types.go:69` stores `ProviderProtection`.
- `internal/config/provider_protection.go:13,47` — `ProviderProtectionConfig`, `validateProtectionPolicy`: parse positive durations, enforce a ceiling, require total when phase deadlines are set.
- `internal/controlstate/runtime.go:171` — `protectAdapters` owns/reuses one `ProtectionSet` and wraps every runtime adapter; retain this lifecycle.
- **Missing:** no request-wide deadline at the handler/service entry. New optional budget must enclose cache, routing, admission, attempts and bounded completion; earlier parent deadline wins.

## 2. Ordinary attempt and queue propagation

Data flow: handler context → `HandleChatCompletion` → request rules/cache → `SelectExcluding` → `Admit` → `runScheduledChat` → protected adapter → response rules → settlement → response.

- `internal/gateway/service.go:92,147,214,258,276,348,362` — `HandleChatCompletion`: existing vertical path; copy `upstreamReq` before remapping model at lines 272–275.
- `internal/gateway/service.go:389,400` — `runScheduledChat` / `runScheduledStream`: direct execution or injected `SynchronousRunner` with the supplied context.
- `internal/scheduler/executor.go:83,124,192` — `RunChat`, `RunStream`, `waitForTask`; `internal/scheduler/intake.go:128` — `waitForSoftLimit` selects `ctx.Done()`.
- `internal/admission/controller.go:61` — `LimitAdmissionController.Admit` accepts caller context for limit/credit operations.
- `internal/cache/semantic.go:156,169,193` — `LookupWithVector` has a child read timeout and `readTimeout` reports an optional-cache fault.
- **Required distinction:** cache child deadline may bypass; expired parent must stop. Neither another provider attempt nor judge call receives a fresh whole-request budget.
- **Audit boundary:** `executor.go:98,138` cancels using `Background`; completion evidence also uses `WithoutCancel` at lines 159/187. Verify bounded teardown without expanding into a new scheduler redesign.

## 3. Existing single-attempt protection

- `internal/providers/protection.go:13,22,69,92` — `ProtectionPolicy`, `NewProtectionSet`, `Wrap`, `acquireProtectionPermit`; shared resource-group pool survives runtime replacements.
- `internal/providers/protected_adapter.go:29,43,69` — `Complete`, `protectedEmbedding`, `Stream`: invoke inner adapters with the protected child context and release on finish.
- `internal/providers/protection_attempt.go:20` — `ProtectionPolicy.begin` starts total/first-byte/first-content timers per attempt, not per gateway request.
- `internal/providers/protection_stream.go:45,78,82,139` — relay, meaningful-event check, idle timeout, reader drain then permit release.

Verbatim reuse point — `internal/providers/protection_attempt.go:25-29`:

```go
child, cancel := context.WithCancelCause(ctx)
run := &protectedAttempt{ctx: child, cancel: cancel, release: release}
run.addTimer(policy.Total, gwerr.ProviderOverallTimeout)
firstByte := run.addTimer(policy.FirstByte, gwerr.ProviderFirstByteTimeout)
run.content = run.addTimer(policy.FirstContent, gwerr.ProviderFirstContentTimeout)
```

## 4. Conservative cross-provider fallback

- `internal/gateway/service.go:99,115,214,329` — max-attempt gate, provider exclusion loop, parent cancellation guard; retry decision follows at line 334.
- `internal/gateway/service_stream.go:27,43,146` — bounded startup fallback only; accepted event channel leaves the attempt loop at line 237.
- `internal/errors/errors.go:116` — `IsRetryableProviderError` uses coarse categories; only old transient classes qualify at lines 125–127.
- New first-byte/content/idle/overall categories currently fall through to false; do not simply add them to the retry list.
- **Missing:** explicit transmission/execution certainty, remaining-budget gate before each attempt, and conservative tool/history replay policy. No body/token is not proof of non-execution.
- Keep finite cross-provider attempts; no same-provider retries, backoff queue or hedging per D-06.

Verbatim existing decision to refine — `internal/gateway/service.go:334-337`:

```go
if errors.IsRetryableProviderError(err) {
	attempted[decision.ProviderID] = true
	continue
}
```

## 5. Transport and pinned SDK boundaries

- `internal/providers/openai/adapter.go:41,51,157,167` — `NewAdapter` constructs `http.Client` with existing 60-second timeout; request context reaches `client.Do`.
- `internal/providers/anthropic/adapter.go:34,39,265,284` — `NewAdapter` constructs pinned client; `Complete`/`Stream` pass context into SDK Messages calls.
- `internal/providers/gemini/adapter.go:34,36,40,145,170` — `NewAdapter` injects an HTTP client with `streamAuditTransport`; SDK content calls receive context.
- `go.mod:8,22` pins Anthropic v1.50.1 and genai v1.60.0. Default retries and supported disable/injection options remain execution-time audit work; this mapping asserts no default retry count.
- `internal/providers/gemini/stream_audit.go:24,26,45` — `streamAuditTransport.RoundTrip` / `streamAuditBody.Read` are an existing transport/body observation seam, not transmission certainty.
- **Missing:** unified optional connect/TLS limits and proof that SDK/transport cannot amplify the permitted attempt count; preserve Gemini EOF/error audit when injecting transport.

## 6. Terminal ownership and synchronous completion

- `internal/gateway/stream_terminal.go:40,148,161` — `classifyTerminal`, `terminalFinalizer.Submit`, `run`: first terminal wins; callbacks include settlement before admission release.
- `internal/gateway/service_stream_terminal.go:39,53,96,103` — `newStreamTerminal`, `complete`, `recordSettlement`, `releaseAdmission`; retain this single owner.
- `internal/gateway/service_settlement.go:23` — `settleTerminal` permits billing only for completed; missing Usage logs separately; failed persistence is sanitized via `logSettlementFailure` at line 93.
- **Unbounded gap:** `repo.Settle(Background)` at line 38, `Usage().Log(Background)` at line 75, and `AggregateCost(Background)` at line 88 discard the incoming deadline.
- Preserve known Usage and completed-first ownership while bounding these synchronous operations. A budget wrapper around provider calls alone cannot close this gap; do not hide failures or add a durable queue.

Verbatim first-terminal guard — `internal/gateway/stream_terminal.go:149-155`:

```go
f.mu.Lock()
if f.completed {
	f.mu.Unlock()
	return false
}
f.completed = true
f.mu.Unlock()
```

## 7. Stream, buffered stream and Fusion reach

- `internal/gateway/service_stream.go:131,200,271,300` — terminal construction, ordinary relay, `bufferStreamWithResponseRules`, `collectBufferedStream`; include buffered paths and tool deltas.
- `internal/http/handlers/chat_stream.go:59` — `streamSSEEvents`: actual client write loop; do not replay after output starts.
- `internal/gateway/fusion.go:22,69,146,209,348` — `executeFusion`, parallel `runFusionMembers`, `executeFusionProvider`, `executeFusionStream`, buffered Fusion path.
- Members and judge share caller context through `runScheduledChat`/`runScheduledStream`; judge routing after members is a new-work boundary that must check remaining budget.
- `internal/gateway/fusion.go:389` — `rejectFusionToolProtocol` is an existing prohibition to retain, not replace with tool execution.

## 8. Observability and repeatable validation analogs

- `internal/observability/stages.go:17,28` — `WithTimingID` / `Stage`: optional diagnostic observer; ordinary metrics do not automatically enable stage export.
- `internal/observability/tracing.go:62,78,98` — request trace, routing attributes, final outcome; request ID belongs to traces/evidence, not metrics labels.
- `internal/observability/cache.go:30` — `RecordCacheOutcome` uses an explicit allowed-label set; reuse the low-cardinality style for bounded reasons, not its cache-only vocabulary.
- `internal/app/live_timing_test.go:88,118,141,201` — `RecordStage`, transport `RoundTrip`, `clientTrace`, `dump`: existing stage/HTTP evidence seams.
- `internal/app/live_provider_protection_test.go:30,87,156,186` — deadline/shared-resource/concurrency/cancellation scenarios; `live_provider_lifecycle_protection_test.go:15,32` — total deadline/runtime capacity.
- `internal/gateway/service_settlement_test.go:95,108,122,147,200` — once-only billing, missing Usage, non-completed skip, sanitized errors, settlement-before-release tests.
- Reuse assertion/evidence shapes, but add deterministic local HTTP/SSE cases for request budget and safe failover before implementation; existing live tests may require external setup.

Verbatim evidence shape — `internal/app/live_provider_lifecycle_protection_test.go:29`:

```go
shipLogJSON(t, map[string]any{"type": "provider_overall_timeout", "elapsed_ms": float64(time.Since(started).Microseconds()) / 1000})
```

## 9. Explicit non-patterns and limits

- OpenAI `HealthCheck` only checks configured key (`internal/providers/openai/adapter.go:143-148`); Anthropic `:66-67` and Gemini `:100-101` return placeholder success.
- `internal/controlstate/admin_service.go:370` calls that same `HealthCheck`; neither it nor the background Prober is proof of real connection success. True probing is deferred by CONTEXT.
- No new budget/connection config, execution-certainty type, conservative eligibility policy or bounded settlement exists yet; names above describe current symbols only.
- No test was executed for this map. Stage-end evidence must distinguish named PASS, SKIP and no-matching-test, with both hard outer 60-second cap and `go test -timeout 60s`.
