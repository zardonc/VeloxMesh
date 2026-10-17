# Phase 31: Operator Diagnostics — Existing Patterns

**Mapped:** 2026-10-17 | **Method:** CodeGraph first, bounded current-source reads.
**Reviewed:** 2026-10-17 after final `31-CONTEXT.md`; D-03–D-09 scope applied and `internal/controlstate/admin_service.go` symbols rechecked. No `admin_provider.go` implementation is assumed.
**Status:** static mapping only; no product tests, live requests, generation, or SDK upgrades.
**Dependency:** Phase 30 is planned, not implemented; 30 → 31. Its proposed files are not reusable implementation.
**User decision:** background probes are non-generating and report connectivity/auth only; generation tests require explicit administrator action and bounded limits. Console follows later.

## 1. Current probe path and semantic hazard

Data flow: runtime `Prober.Start` → adapter `HealthCheck` → `RecordProbe` → local/Redis health state → router/readiness.

- `internal/providers/adapter.go:11-20` — `ProviderAdapter` exposes `Models`, `HealthCheck`, `Capabilities`; no separate connectivity/generation diagnostic operation.
- `internal/providers/openai/adapter.go:143-148` — `HealthCheck` checks configured key only; Anthropic `adapter.go:66-67` and Gemini `adapter.go:100-101` return placeholder success.
- `internal/health/prober.go:49,64,93,123` — `getTimeout`, `ProbeProvider`, `ProbeOnce`, `Start`; global/per-provider timing, enabled flag and bounded concurrent probe framework exist.
- `prober.go:80-89` wraps a probe timeout and records results; lines 110/180 acquire a semaphore without selecting cancellation, a narrow shutdown/capacity audit point.
- `internal/controlstate/runtime.go:153-166` builds registry/router/prober together and replaces the prior probe loop through cancellation.
- `internal/health/store.go:162-188` — `RecordProbe` changes the same failure/success counters as live traffic; successful probes can clear actual request failures.
- `internal/health/redis_store.go:133-153` does the same through `redisOutcome`, then publishes a distinct probe snapshot; connectivity success must stop repairing generation health.
- `internal/health/redis_snapshots.go:13-40` — `Snapshot` propagates read/decode failures explicitly; reuse explicit unavailable semantics instead of defaulting a failed diagnostic store to healthy.
- `internal/http/handlers/health.go:27-70` — `Readyz` consumes local snapshots/capabilities/probe details; separate its routing-health meaning from lightweight diagnostic results.

Verbatim hazard to replace, not copy semantically — `internal/health/redis_store.go:139-142`:

```go
updated := redisOutcome(state, latency, err)
updated.LastProbeAt, updated.LastProbeDuration = time.Now(), latency
updated.LastProbeSuccess, updated.LastProbeError = success, errMsg
return updated
```

## 2. Manual testing, non-generating provider seams

- `internal/controlstate/admin_service.go:37-44` — `ProviderTestConnectionResponse` has one `OK`/code/message/latency result, insufficient to distinguish connectivity/auth/model generation.
- `admin_service.go:303-385` — `TestConnection` loads a provider, decrypts its secret, builds an independent adapter and invokes `HealthCheck` at line 370; it neither calls `RecordProbe` nor uses the runtime protected adapter.
- `internal/http/handlers/admin_providers.go:134-159` — manual handler uses request context and existing `WithIdempotency`; preserve explicit operation triggering and repeat-call protections.
- `internal/http/handlers/models.go:30-55` — public `ListModels` returns configured catalog entries, not an upstream request; Adapter `Models()` is configured metadata too.
- OpenAI-compatible adapter already has context-aware `http.Client` calls (`openai/adapter.go:151-167`), but no upstream GET-models probe implementation.
- Pinned cached Anthropic SDK has non-generating `ModelService.List`: `C:/Users/inthe/go/pkg/mod/github.com/anthropics/anthropic-sdk-go@v1.50.1/model.go:65-77` uses GET `v1/models`.
- Pinned cached Gemini SDK has `Models.List`: `C:/Users/inthe/go/pkg/mod/google.golang.org/genai@v1.60.0/models.go:5643`; listing plumbing starts at line 5171. Neither listing is currently used by gateway HealthCheck.
- These are candidate seams, not evidence that a configured proxy supports listing or authenticates it. Unsupported/unknown, transport failure, rejected credentials and generation failure require distinct bounded results.
- Generation tests must remain manual, minimal and limited; never infer permission for background billing or expose test output/body in diagnostic records.
- Final D-07 keeps manual generation outside business tenant billing/cache/queue/fallback, with distinct Usage and delay categories; require registered validated targets, safe redirects, provider/model and bounded output/time/wire calls.
- `internal/controlstate/admin_service.go:313`, `audit.go:27-45` provide an audit hook, but actor defaults to system and persistence errors are currently discarded; do not reproduce silent audit loss for a billable action.

## 3. Request ID and trace correlation

Data flow: `RequestID` middleware → `GetReqID` → `LLMRequest.RequestID` → trace/timing ID → provider/fallback/finalizer → HTTP output.

- `internal/http/router.go:18-20` installs RequestID before logging; `handlers/chat.go:43-63,96-102` passes the correlation ID to ordinary/stream paths.
- `internal/http/middleware/requestid.go:16-28` accepts caller `X-Request-ID` verbatim or generates 16 random bytes; no length/charset/duplicate-ownership validation exists.
- Final D-06 requires a new server-generated internal ID through attempts/trace/diagnostics with an explicit response header; preserve settlement UUID/public protocol-ID contracts. This internal-ID path does not exist yet.
- External IDs need format/length validation and safe correlation (prefer digest); bounded multiple matches must preserve duplicate requests, rather than treating caller IDs as unique keys or tenant scope.
- `internal/observability/tracing.go:62-74,78-109` — `StartRequestTrace`, `RecordRouting`, `RecordOutcome` attach ID, route, cache/error and latency attributes.
- `tracing.go:21-26,45-52` is optional OTLP export/no-op; it contains no query client or guaranteed retained local record.
- `internal/observability/stages.go:17,28` — `WithTimingID` / `Stage` offer an optional observer; ordinary Prometheus metrics do not enable `RecordStage` automatically.
- `internal/observability/prometheus.go:322-326` accepts request ID but deliberately excludes it from metric labels; retain this boundary.

Verbatim correlation seam — `internal/http/middleware/requestid.go:25-28`:

```go
ctx := context.WithValue(r.Context(), RequestIDKey, reqID)
w.Header().Set(RequestIDHeader, reqID)

next.ServeHTTP(w, r.WithContext(ctx))
```

## 4. Timing ownership at the actual HTTP output

- `internal/http/middleware/logging.go:25-40` owns a timestamp before the handler and duration after return, including auth; `responseWriterObserver` forwards Flush at lines 19-22 but does not measure writes or flush errors.
- `internal/http/handlers/chat.go:44,58-63,120-144` starts after auth, calls the service, then writes JSON; E2E header is set before body encoding, whose error is discarded.
- `internal/http/handlers/chat_stream.go:16-36` owns SSE lifetime, initial header flush, and the output loop; initial headers/role-only/Usage/error/DONE are not a valid first-content event.
- `chat_stream.go:59-91,133-152` — `streamSSEEvents`, `writeData`, `writeDone` expose successful write boundaries; `http.Flusher.Flush()` currently returns no error.
- The handler/output observer is the reusable owner for gateway-observed client TTFT and stream completion; network arrival at the remote client still needs client-side evidence and must not be claimed from Flush alone.
- Final D-04 uses monotonic entry-to-first/last successful semantic write+flush with text/tool types; complete TTLT requires successful completion. Interrupted/no-content/non-stream token timings are nullable; retain last-output and independent E2E.
- Do not mix these output timings into old service/attempt metrics without an explicit compatibility meaning; successful writes do not prove remote receive/decode time.
- `internal/gateway/service_stream.go:130,207-225` currently measures first event since the post-admission attempt start, before HTTP output; this is an upstream/service signal, not client TTFT.
- `internal/gateway/service_stream_terminal.go:76-100` records metrics/trace/settlement when upstream terminal wins; keep Phase 27 ownership separate from HTTP timing completion.
- `internal/http/handlers/chat_stream.go:39-45` labels setup duration `X-Latency-E2E-Ms` before streaming, so it cannot provide final TTLT.

Verbatim successful chunk boundary — `internal/http/handlers/chat_stream.go:84-88`:

```go
if emitter.writeChunk(streamChunk(reqID, model, event, first)) != nil {
	cancel()
	return
}
first = false
```

## 5. Buffered/Fusion/cache paths must reach the same recorder

- `internal/gateway/service_stream.go:271-297,300-330` — `bufferStreamWithResponseRules` / `collectBufferedStream` collect upstream content and finalize before replay/single-text output.
- `internal/gateway/fusion.go:209,306-341,348` — Fusion stream setup/forward/buffered path; member generation is not client first content, and first judge event is not necessarily first client content.
- `internal/gateway/stream_terminal.go:148-168` — `terminalFinalizer.Submit` keeps first-terminal-wins settlement/admission callbacks; do not attach a second billing owner to output timing.
- `internal/gateway/service.go:147-165` — cache hit ends trace with zero timings before client encoding. The HTTP recorder must also cover cache hits without enabling production semantic cache.
- Normal JSON, stream text, tool stream, buffered/Fusion, error/cancel and no-content paths need one shared output contract, with a bounded final record after the response path terminates.

## 6. Existing storage, ownership and bounded-query gaps

- `internal/controlstate/types.go:142-156` — `UsageRecord` has APIKey/provider/model/tokens/duration, but no request ID, route stages, TTFT/TTLT or output outcome.
- `internal/gateway/service_settlement.go:46-64` generates a new Usage UUID unrelated to request ID; completed billing records are not complete request diagnostics.
- `internal/controlstate/repository.go:74-81` — Usage repository only writes; Audit supports target List and timestamp purge, not bounded metadata-by-request lookup.
- SQLite `repository.go:791-814` / PostgreSQL `repository.go:753-772` Audit List lack LIMIT; using audit metadata as an unrestricted request store would not meet bounded-query requirements.
- `internal/controlstate/audit.go:11-24` — `SafeAuditMetadata` is a shallow exact-key denylist, not a safe diagnostic serializer; use explicit allowed fields and bounded codes instead of raw nested maps/errors/upstream bodies.
- `internal/hotstate/hotstate.go:31-42,109-113` — `ByteCache`, `AtomicLimiter`, `NamespacedKey` offer DI/TTL/shared-namespace primitives; none supplies tenant authorization or total diagnostic capacity.
- `internal/hotstate/local.go:24-41,139-156` stores byte data in an unbounded map; expired reads return miss without deleting keys. Local TTL alone is not a capacity/retention implementation.
- `internal/hotstate/redis.go:158-170` provides namespaced GET/SET+TTL across nodes, but no diagnostic admission cap, per-record byte limit, listing/index policy, or bounded publication queue.
- Final D-05 chooses optional/default-off node-local memory only, bounded by TTL/count/total bytes/field and attempt sizes; nonblocking discard/eviction reports finite reasons. Generic ByteCache is only an interface analog, not the selected unbounded implementation.
- `internal/app/app.go:233-252` chooses Redis/local health stores; diagnostics must independently report current-node scope, expiry and restart loss. Redis diagnostic persistence, global lookup/replication and automatic cross-node ID routing are deferred.

Verbatim primitive interface — `internal/hotstate/hotstate.go:32-36`:

```go
type ByteCache interface {
	GetBytes(ctx context.Context, key string) ([]byte, error)
	SetBytes(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
```

## 7. Administrative access, limits and verification seams

- `internal/http/middleware/adminauth.go:11-37` provides global bearer AdminAuth, denies missing configuration, but sets no administrator/tenant identity and applies no rate limiter.
- Final D-07 reuses that global administrator boundary without JWT/RBAC expansion; query/probe/TestConnection touchpoints need explicit schema and rate/concurrency limits, while background probe capacity stays separate from data-plane resources.
- `internal/http/router.go:32-45,87-89` shows authenticated admin groups; Provider GET/test routes currently also inherit `RequireWritable`, unlike read-only topology. Place diagnostics reads according to actual read/write behavior.
- `internal/admission/controller.go:61,112,139` uses `AtomicLimiter` for data-plane key/provider rules; this does not rate-limit admin queries or test actions automatically.
- `internal/hotstate/local.go:166` / `redis.go:190` — `CheckAndIncrement` is reusable for configured fixed-window control-plane limits; validate keys/bounds and expose backend failure explicitly.
- `internal/http/handlers/admin_providers.go:308-329` currently emits raw unknown error text; diagnostic responses should use allowlisted codes without upstream response text/URLs/credentials.
- `internal/http/handlers/chat_stream_test.go:111,125,138,153,169,181,203` gives completed/error/write-failure/cancel/tool transport cases; `health/prober_test.go:32,86,120` gives probe scheduling seams.
- `internal/app/live_timing_test.go:88,98,118,141,201` provides observer, handler, transport trace and JSON evidence shapes; `live_transport_observation_test.go:156` checks sanitized observations.
- Final D-09 requires two independent new local HTTP/SSE evidence runs, query/auth/TTL/capacity/current-node/duplicate-ID cases and named PASS; both outer hard cap and Go `-timeout 60s` apply. No tests ran for this map.
- The final scope locks ephemeral local retention and global admin access; exact finite bounds/field names remain plan choices. No Console, general RBAC platform, MCP or cache-production scope is implied.
