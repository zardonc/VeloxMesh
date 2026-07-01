# Phase 10: Advanced Routing & Observability - Research

**Researched:** 2026-06-30
**Domain:** Go gateway routing, SQLite runtime config, OpenTelemetry traces, Prometheus metrics
**Confidence:** MEDIUM

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Add a balanced `composite-score` routing strategy that considers latency, pending requests, error rate, cost, and health status.
- **D-02:** Cost is only a tie-breaker between otherwise similar candidates. Reliability, speed, and load stay ahead of savings.
- **D-03:** Unhealthy providers are hard excluded, matching current router behavior. Degraded providers remain eligible but receive a score penalty.
- **D-04:** Use guarded z-score normalization only when candidate count and variance are sufficient. For tiny or flat candidate pools, fall back to min/max or neutral scoring.
- **D-05:** Use existing round-robin behavior until a provider/model has enough successful live request samples to be scored confidently.
- **D-06:** If every candidate's composite score is below the configured threshold, trigger the existing provider fallback chain before returning failure.
- **D-07:** Stale latency, error, and load metrics become neutral instead of helping or hurting. Health status still applies.
- **D-08:** "Warm enough" means a small minimum count of successful live requests per provider/model. Planners should choose a conservative default and make it configurable.
- **D-09:** Traces should capture the request lifecycle: routing decision, selected provider, strategy, score summary, TTFT, TPOT, E2E latency, cache hit/miss, provider outcome, and fallback reason where applicable.
- **D-10:** Prometheus labels must stay low-cardinality: `provider`, `model`, `strategy`, `status`, `cache_result`, and `error_category`. Do not use request, user, API-key, or raw identity labels.
- **D-11:** Score breakdown visibility is trace-attribute only for the selected provider. Do not emit per-candidate metrics by default.
- **D-12:** Apply strict sanitization to all observability data. Do not record raw prompts, auth headers, API keys, sensitive provider payloads, or user identifiers. Request IDs are allowed only when generated and safe.
- **D-13:** Introduce composite routing as an opt-in `composite-score` strategy beside existing `round-robin` and `least-latency`; do not change default runtime behavior.
- **D-14:** Store routing strategy, weights, thresholds, warm-up count, stale window, preset name, and optional overrides in SQLite-backed routing config with hot reload.
- **D-15:** Expose a conservative default preset plus optional explicit overrides for advanced operators.
- **D-16:** Invalid or risky routing config must be rejected at validation. Runtime should retain the last known good routing config rather than clamping silently or accepting unsafe values.

### the agent's Discretion
- Keep the first implementation small: one opt-in strategy, one conservative preset, and the narrowest config schema needed to support safe overrides.
- Reuse `internal/routing.HealthAwareRouter`, `health.Store` snapshots, existing fallback behavior, and the current observability interface shape rather than introducing a parallel routing engine.

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within Phase 10 scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ROUT-01 | Gateway can route provider requests through a Composite Score Router. | Add `composite-score` inside `HealthAwareRouter.SelectExcluding`; keep default strategies unchanged. [VERIFIED: codebase grep] |
| ROUT-02 | Gateway can score routing candidates using latency, pending requests, error rates, cost, and health bonus signals. | Use `health.ProviderSnapshot` for latency/load/error/health and `controlstate.ProviderModelRate` for cost tie-breaks. [VERIFIED: codebase grep] |
| ROUT-03 | Gateway can normalize routing signals with z-score normalization before weighting. | Implement guarded z-score with candidate-count and variance gates, fallback to min/max or neutral. [CITED: 10-CONTEXT.md] |
| OBS-01 | Operators can inspect OpenTelemetry traces for TTFT, TPOT, end-to-end latency, and cache-hit behavior. | Use OpenTelemetry Go spans around gateway request lifecycle and stream event timing. [CITED: /open-telemetry/opentelemetry-go] |
| OBS-02 | Operators can scrape Prometheus histogram metrics for routing and request timing. | Replace/extend `observability.Metrics` with Prometheus histograms and counters using low-cardinality labels only. [VERIFIED: Go module registry] |
</phase_requirements>

## Summary

Phase 10 should extend the current router and observability seams rather than introduce new engines. `internal/routing.HealthAwareRouter.SelectExcluding` already owns model eligibility, route overrides, combo routing, health filtering, and fallback exclusion; composite routing belongs as one more strategy branch there. `health.ProviderSnapshot` already exposes EWMA latency, pending requests, successes/failures, status, and freshness timestamps, so the scoring work should be a small scoring helper plus tests, not a new state store. [VERIFIED: codebase grep]

The durable config path also already exists. `controlstate.RoutingConfig`, `RoutingRepository`, SQLite migrations/repository methods, `RuntimeProviderManager`, and `App.ReloadProviders` are the correct seams for strategy, weights, thresholds, warm-up count, stale window, preset name, and overrides. The important missing piece is validation: invalid composite config must fail activation/save and preserve the existing runtime snapshot. [VERIFIED: codebase grep]

Observability should be a minimal production implementation behind the current `observability.Metrics` package plus OpenTelemetry span helpers in the gateway service path. Do not emit per-candidate Prometheus metrics, raw prompts, user IDs, API keys, auth headers, or provider payloads. The low-cardinality Prometheus labels are exactly `provider`, `model`, `strategy`, `status`, `cache_result`, and `error_category`. [CITED: 10-CONTEXT.md]

**Primary recommendation:** Add `composite-score` as an opt-in strategy inside `HealthAwareRouter`, persist its safe config in SQLite routing config, and instrument the existing gateway request/streaming paths with sanitized OTel spans plus Prometheus histograms.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|--------------|----------------|-----------|
| Composite provider selection | API / Backend | Database / Storage | The Go gateway owns provider selection; SQLite only stores config. [VERIFIED: codebase grep] |
| Health and live scoring signals | API / Backend | Redis hot state | `health.Store` owns snapshots; Redis store is hot state and not durable truth. [VERIFIED: codebase grep] |
| Routing config persistence | Database / Storage | API / Backend | SQLite `RoutingRepository` is authoritative; runtime manager activates validated snapshots. [VERIFIED: codebase grep] |
| OpenTelemetry traces | API / Backend | External collector | Gateway creates sanitized spans; collectors/exporters are external. [CITED: /open-telemetry/opentelemetry-go] |
| Prometheus metrics | API / Backend | External scraper | Gateway exposes counters/histograms; Prometheus scrapes low-cardinality series. [VERIFIED: Go module registry] |

## Project Constraints (from AGENTS.md)

- Do not use `git restore`, `git stash`, `git checkout`, or `git worktree` unless explicitly requested. [VERIFIED: AGENTS.md]
- Use ES modules for JS/TS; Go work should follow existing Go package conventions. [VERIFIED: AGENTS.md]
- Prefer immutable/new objects; avoid mutation-heavy APIs where practical. [VERIFIED: AGENTS.md]
- Inject implementations through parameters/interfaces; avoid `new`-style construction in business logic. [VERIFIED: AGENTS.md]
- Keep functions under 50 lines, files under 500 lines, nesting at most 3, positional parameters at most 3, and cyclomatic complexity at most 10. [VERIFIED: AGENTS.md]
- Validate all external inputs at boundaries with schema or structured validation. [VERIFIED: AGENTS.md]
- Prevent SQL injection, XSS, CSRF, and add rate limiting on endpoints. [VERIFIED: AGENTS.md]
- Do not hardcode secrets or configuration into git-managed source. [VERIFIED: AGENTS.md]
- Use `pnpm` for package-manager work; Go tests must keep backend unit-test timeout at 60 seconds. [VERIFIED: AGENTS.md]
- Prefer `rg` for search and Git Bash for shell commands where possible. [VERIFIED: AGENTS.md]

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `math`, `time`, `sync/atomic` | Go 1.26.1 project baseline | Composite scoring math, stale-window checks, existing RR counter | Already in use; avoids new dependencies for routing math. [VERIFIED: go.mod] |
| `internal/routing` | local | Strategy switch and routing decision metadata | Existing router seam already owns provider selection. [VERIFIED: codebase grep] |
| `internal/health` | local | Live provider snapshots | Exposes latency, pending, failures, successes, status, and timestamps. [VERIFIED: codebase grep] |
| `internal/controlstate` + SQLite repository | local | Durable routing config | Existing authoritative config repository and hot reload path. [VERIFIED: codebase grep] |
| `go.opentelemetry.io/otel` | v1.44.0 | Trace API, tracer provider access, attributes | Official OpenTelemetry Go module; registry version verified with `go list -m -versions`. [VERIFIED: Go module registry] |
| `go.opentelemetry.io/otel/sdk` | v1.44.0 | Tracer provider/export pipeline setup | Official SDK companion module. [VERIFIED: Go module registry] |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | v0.69.0 | Optional HTTP middleware instrumentation | Official OTel contrib HTTP instrumentation. [VERIFIED: Go module registry] |
| `github.com/prometheus/client_golang` | v1.23.2 | Prometheus counters, histograms, `/metrics` handler | Canonical Go Prometheus client module; registry version verified. [VERIFIED: Go module registry] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | align with v1.44.0 OTel core | OTLP trace export | Only if runtime config enables OTLP export; otherwise no-op provider is enough. [CITED: /open-telemetry/opentelemetry-go] |
| `github.com/prometheus/client_golang/prometheus/promhttp` | v1.23.2 | Expose scrape handler | Add `/metrics` only when metrics are enabled/configured. [VERIFIED: Go module registry] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Stdlib scoring helper | External stats/math package | Not needed; z-score/min-max are a few lines and easier to audit. [ASSUMED] |
| Prometheus Go client | OTel metrics only | Prometheus scrape requirement is explicit; OTel metrics can wait. [CITED: 10-CONTEXT.md] |
| Manual request spans only | `otelhttp` middleware only | Middleware cannot capture routing score, cache, fallback, TTFT, or TPOT alone. Use manual spans for gateway lifecycle. [CITED: /open-telemetry/opentelemetry-go] |

**Installation:**

```bash
go get go.opentelemetry.io/otel@v1.44.0 go.opentelemetry.io/otel/sdk@v1.44.0 go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.69.0 github.com/prometheus/client_golang@v1.23.2
```

**Version verification:** `go list -m -versions` succeeded for the modules above after setting `GOPATH`, `GOMODCACHE`, and `GOCACHE` under the workspace to avoid sandbox writes to the user module cache. [VERIFIED: Go module registry]

## Package Legitimacy Audit

> The GSD legitimacy seam supports npm/PyPI/crates only, not Go modules. Go module existence and latest versions were verified with `go list -m -versions`; package legitimacy still relies on official project provenance. [VERIFIED: local tool output]

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `go.opentelemetry.io/otel` | Go module | unknown | unknown | github.com/open-telemetry/opentelemetry-go | OK | Approved; official docs and module registry verified. |
| `go.opentelemetry.io/otel/sdk` | Go module | unknown | unknown | github.com/open-telemetry/opentelemetry-go | OK | Approved; official docs and module registry verified. |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | Go module | unknown | unknown | github.com/open-telemetry/opentelemetry-go-contrib | OK | Approved; official source and module registry verified. |
| `github.com/prometheus/client_golang` | Go module | unknown | unknown | github.com/prometheus/client_golang | OK | Approved; canonical Prometheus Go client module verified on registry. |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

## Architecture Patterns

### System Architecture Diagram

```text
Client request
  -> Chi/OpenAI-compatible handler
  -> gateway.Service
     -> semantic pipeline request stage
     -> semantic cache lookup
        -> cache hit: trace cache_result=hit, return sanitized cached response
        -> cache miss:
           -> RuntimeProviderManager.SelectExcluding
              -> Registry model eligibility
              -> health.Store snapshots
              -> hard exclude unhealthy/excluded providers
              -> if composite-score and warm enough:
                   guarded normalize latency/load/error/health
                   apply weights, degraded penalty, cost tie-breaker
                   if best score below threshold: return no healthy/score error for fallback loop
                 else:
                   round-robin cold-start behavior
           -> admission + circuit breaker
           -> provider adapter call or stream
           -> health.EndRequest + settlement + cache store
           -> Prometheus metrics + OpenTelemetry span attributes/events
```

### Recommended Project Structure

```text
internal/
├── routing/
│   ├── router.go              # Strategy branch and RoutingDecision extension
│   ├── composite.go           # Small scoring helper, no provider calls
│   └── composite_test.go      # Normalization, warm-up, stale, threshold tests
├── controlstate/
│   ├── types.go               # RoutingConfig fields for composite settings
│   ├── validation.go          # Reject invalid/risky config
│   └── migrations/sqlite/     # Add nullable routing config columns
├── observability/
│   ├── metrics.go             # Interface plus safe event shape
│   ├── prometheus.go          # Real Prometheus implementation
│   └── tracing.go             # Span helper functions and sanitized attrs
└── gateway/
    └── service.go             # Lifecycle span boundaries, TTFT/TPOT/E2E hooks
```

### Pattern 1: Composite Scoring Helper

**What:** Keep scoring as a pure helper that accepts immutable candidate snapshots/config and returns the selected provider ID plus selected-provider score summary. [VERIFIED: codebase grep]

**When to use:** Only inside `HealthAwareRouter` when strategy is exactly `composite-score` and the pool is warm enough. [CITED: 10-CONTEXT.md]

**Example:**

```go
// Source: local pattern from internal/routing/router.go
type CompositeResult struct {
	ProviderID string
	Score      float64
	Reason     string
}

func ScoreComposite(candidates []CompositeCandidate, cfg CompositeConfig, now time.Time) (CompositeResult, bool) {
	// Return false when warm-up, tiny pool, flat variance, or threshold rules require fallback behavior.
}
```

### Pattern 2: Guarded Normalization

**What:** Use z-score only when candidate count meets the configured minimum and variance is non-zero above epsilon; otherwise use min/max for varied small pools or neutral scores for flat/stale signals. [CITED: 10-CONTEXT.md]

**When to use:** For latency, pending requests, and error rate. Health is categorical; cost is tie-break only. [CITED: 10-CONTEXT.md]

**Example:**

```go
// Source: Phase 10 locked decision D-04
func normalizedSignal(values []float64, cfg NormalizationConfig) []float64 {
	if len(values) >= cfg.MinZScoreCandidates && variance(values) > cfg.MinVariance {
		return zScores(values)
	}
	if hasRange(values, cfg.MinVariance) {
		return minMaxScores(values)
	}
	return neutralScores(len(values))
}
```

### Pattern 3: Sanitized Tracing

**What:** Start one request lifecycle span, add routing/cache/provider events, and record errors with `span.RecordError(err)` plus explicit error status. OpenTelemetry docs state `RecordError` records an exception event and status must be set separately. [CITED: /open-telemetry/opentelemetry-go]

**When to use:** In non-streaming and streaming gateway paths, with streaming TTFT captured on first emitted token/chunk and TPOT calculated only from safe timing/count data. [CITED: 10-CONTEXT.md]

**Example:**

```go
// Source: Context7 /open-telemetry/opentelemetry-go
ctx, span := tracer.Start(ctx, "gateway.chat_completion")
defer span.End()

span.SetAttributes(
	attribute.String("llm.provider", decision.ProviderID),
	attribute.String("llm.model", req.Model),
	attribute.String("routing.strategy", decision.Strategy),
)
if err != nil {
	span.RecordError(err)
	span.SetStatus(codes.Error, errorCategory)
}
```

### Anti-Patterns to Avoid

- **Parallel router engine:** Do not bypass `HealthAwareRouter`; it would duplicate health filtering, overrides, combo behavior, and fallback exclusion. [VERIFIED: codebase grep]
- **Cost as a weighted primary signal:** The locked decision says cost is tie-breaker only. [CITED: 10-CONTEXT.md]
- **Per-candidate Prometheus labels:** Provider/model/strategy/status/cache_result/error_category are the only allowed labels; per-request or per-user labels create cardinality risk. [CITED: 10-CONTEXT.md]
- **Silent config clamping:** Reject invalid/risky routing config and retain last known good. [CITED: 10-CONTEXT.md]
- **Raw observability payloads:** Never record prompts, provider payloads, auth headers, API keys, sensitive content, user IDs, or API-key IDs. [CITED: 10-CONTEXT.md]

## Recommended Plan Slices

| Slice | Scope | Files | Verification |
|-------|-------|-------|--------------|
| 10-01 Composite router core | Add config structs, score helper, strategy branch, decision score summary | `internal/routing/*`, `internal/health/store.go` only if a model-keyed sample count is needed | `go test ./internal/routing ./internal/health` |
| 10-02 SQLite routing config | Add migrations, repository read/write fields, validation, activation preservation | `internal/controlstate/*`, sqlite migrations/tests, app reload tests | `go test ./internal/controlstate ./internal/app` |
| 10-03 Prometheus metrics | Implement real metrics registry and `/metrics` handler wiring with approved labels | `internal/observability/*`, HTTP router wiring | `go test ./internal/observability ./internal/http/...` |
| 10-04 OpenTelemetry traces | Add trace setup and gateway lifecycle span/events for non-streaming and streaming | `internal/observability/tracing.go`, `internal/gateway/service.go` | `go test ./internal/gateway ./tests/integration` as available |
| 10-05 End-to-end safeguards | Integration tests for opt-in strategy, fallback threshold, cache hit/miss labels, sanitization | targeted integration tests | `go test ./...` under 60-second backend timeout where feasible |

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Trace API/export pipeline | Custom trace structs/exporter | OpenTelemetry Go | Standard propagation, spans, attributes, errors, and collectors. [CITED: /open-telemetry/opentelemetry-go] |
| Prometheus exposition | Manual text endpoint | `client_golang/prometheus` + `promhttp` | Avoid malformed exposition and label registration bugs. [VERIFIED: Go module registry] |
| Routing engine | New strategy framework | Existing `HealthAwareRouter` switch | Current router already centralizes eligibility, override, combo, health, and fallback exclusion. [VERIFIED: codebase grep] |
| Durable config bus | New config store | Existing SQLite `RoutingRepository` and hot-state config events | SQLite is authoritative; Redis/hotstate is notification/acceleration only. [VERIFIED: codebase grep] |
| Statistics package | External math dependency | Go stdlib helper functions | Composite normalization needs only mean, variance, min/max. [ASSUMED] |

**Key insight:** The phase is mostly seam completion. The lazy, durable implementation is a pure scoring helper plus real telemetry adapters behind existing routing/controlstate/observability surfaces.

## Common Pitfalls

### Pitfall 1: Cold-Start Bias

**What goes wrong:** A provider with one lucky low-latency request wins too often.
**Why it happens:** Composite scoring trusts sparse samples.
**How to avoid:** Require configurable minimum successful live requests per provider/model; use round-robin until all eligible candidates are warm enough or score only warm candidates with a conservative fallback policy. [CITED: 10-CONTEXT.md]
**Warning signs:** New provider wins immediately after startup because `EWMALatency` is non-zero.

### Pitfall 2: Stale Metrics Become False Confidence

**What goes wrong:** Old latency/error values keep helping or hurting a provider long after conditions changed.
**Why it happens:** `ProviderSnapshot.LastUpdated` exists but current least-latency selection does not check freshness.
**How to avoid:** Treat stale latency/load/error as neutral; keep health status active. [VERIFIED: codebase grep]
**Warning signs:** Selection remains fixed after long idle periods.

### Pitfall 3: Retry/Fallback Never Triggers on Low Score

**What goes wrong:** Router picks a bad provider even when all scores are below threshold.
**Why it happens:** Selection always returns a provider unless health excludes all.
**How to avoid:** Return an error or no-selection state that the existing fallback loop can interpret without changing default strategies. Keep `SelectExcluding` semantics clear. [VERIFIED: codebase grep]
**Warning signs:** Tests show `FallbackUsed=false` when every candidate is below threshold.

### Pitfall 4: Observability Leaks Identity or Prompts

**What goes wrong:** Request/user/API-key IDs or prompt content appear in spans or labels.
**Why it happens:** Gateway has access to auth identity and serialized request messages near cache lookup.
**How to avoid:** Centralize span/metric attribute creation in `observability` helpers that accept only safe fields. [VERIFIED: codebase grep]
**Warning signs:** Attribute names include `user`, `api_key`, `prompt`, `authorization`, `body`, or `messages`.

### Pitfall 5: Streaming Timing Is Measured Like Non-Streaming

**What goes wrong:** TTFT and TPOT are missing or wrong for streams.
**Why it happens:** Streaming returns before the goroutine consumes provider events.
**How to avoid:** Record TTFT on first successful stream event and E2E/TPOT when the stream channel closes. [VERIFIED: codebase grep]
**Warning signs:** Streaming metrics only contain adapter setup latency.

## Code Examples

### Router Strategy Hook

```go
// Source: internal/routing/router.go
switch r.strategy {
case "composite-score":
	selected, summary, ok := r.selectComposite(healthyProviders, req.Model, time.Now())
	if !ok {
		selected = r.selectRoundRobin(healthyProviders)
		strategyUsed = "composite-score-cold-start-rr"
	}
case "least-latency":
	selected = r.selectLeastLatency(healthyProviders)
}
```

### Config Validation Shape

```go
// Source: internal/controlstate/validation.go pattern
if rc.Strategy != "round-robin" && rc.Strategy != "least-latency" && rc.Strategy != "composite-score" {
	errs = append(errs, FieldError{Field: "strategy", Code: "invalid_strategy"})
}
if rc.Composite != nil && rc.Composite.WarmupSuccesses < 1 {
	errs = append(errs, FieldError{Field: "composite.warmup_successes", Code: "invalid"})
}
```

### Prometheus Labels

```go
// Source: Phase 10 locked decision D-10
var requestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{Name: "veloxmesh_request_duration_seconds"},
	[]string{"provider", "model", "strategy", "status", "cache_result", "error_category"},
)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| One-signal `least-latency` | Guarded composite score with health and fallback | Phase 10 target | Better decisions without changing default behavior. [CITED: 10-CONTEXT.md] |
| Stub metrics | Prometheus histograms/counters and OTel traces | Phase 10 target | Operators can inspect routing and latency behavior. [VERIFIED: codebase grep] |
| Static/env routing strategy only | SQLite routing config with hot reload | Phase 7+ existing seam, Phase 10 extension | Operators can opt in safely and retain last known good config. [VERIFIED: codebase grep] |

**Deprecated/outdated:**
- `priority` routing config validation remains in `ValidateRoutingConfig`, but `HealthAwareRouter` does not implement a `priority` strategy. Planner should either leave it untouched as legacy or remove only if tests show it is dead; Phase 10 should not expand `priority`. [VERIFIED: codebase grep]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | An external stats/math dependency is unnecessary for z-score/min-max scoring. | Standard Stack, Don't Hand-Roll | Low; implementation can still add one if code becomes error-prone, but current math is small. |
| A2 | Publish age/downloads for Go modules are not required for this project if official provenance and module registry versions are verified. | Package Legitimacy Audit | Medium; if planner enforces package-age fields strictly, it should add a manual pkg.go.dev/source audit task. |

## Open Questions (RESOLVED)

1. **Where should provider/model sample counts live?**
   - What we know: `ProviderSnapshot.TotalSuccesses` is provider-level, not explicitly provider/model-level. [VERIFIED: codebase grep]
   - Resolution: Track warm-up by provider/model when the existing request path can identify both values; otherwise start with provider-level `TotalSuccesses` as the compatibility fallback. Any model-keyed state must stay in the routing/health runtime path, not a new durable store.
   - Plan binding: 10-01 owns the minimal runtime sample-count seam and tests cold-start round-robin behavior.

2. **How should low-score selection signal fallback?**
   - What we know: `HandleChatCompletion` retries when provider errors are retryable and `attempted` excludes failed providers. [VERIFIED: codebase grep]
   - Resolution: Add one internal routing outcome/error category for "composite score below threshold" and map it into the existing fallback loop. `round-robin` and `least-latency` keep their current behavior.
   - Plan binding: 10-01 defines the routing signal; 10-05 verifies the fallback path.

3. **Should `/metrics` always be exposed?**
   - What we know: Phase 10 requires Prometheus scrape metrics, but current config shape was not fully audited for an observability enable flag. [VERIFIED: codebase grep]
   - Resolution: Prefer an existing config-gated observability path if present. If no such config exists, add the smallest internal Prometheus endpoint with only the approved low-cardinality labels and no sensitive payload exposure.
   - Plan binding: 10-03 owns endpoint wiring and label allowlist tests.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | Build/test and module verification | yes | project `go 1.26.1`; `go test` succeeded | none |
| SQLite repository | Routing config persistence | yes | `modernc.org/sqlite v1.52.0` in `go.mod` | none |
| Redis Stack | Hot health/config state | optional | `github.com/redis/go-redis/v9 v9.20.1` in `go.mod` | in-memory/local hot state |
| OpenTelemetry collector | Trace export | not required for tests | not probed | no-op/stdout or in-memory test exporter |
| Prometheus server | Scrape validation | not required for tests | not probed | `promhttp` handler unit tests |

**Missing dependencies with no fallback:** none for planning and unit tests.

**Missing dependencies with fallback:**
- OpenTelemetry collector: use no-op or test exporter during automated tests.
- Prometheus server: test the handler/registry without running Prometheus.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` with current module |
| Config file | none |
| Quick run command | `go test ./internal/routing ./internal/health ./internal/controlstate ./internal/observability` |
| Full suite command | `go test ./...` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| ROUT-01 | `composite-score` selects through existing router and leaves default behavior unchanged | unit | `go test ./internal/routing -run TestComposite` | no, Wave 0 |
| ROUT-02 | latency, pending, error rate, health penalty, and cost tie-break are applied in priority order | unit | `go test ./internal/routing -run TestCompositeScoreSignals` | no, Wave 0 |
| ROUT-03 | z-score guard falls back for tiny/flat pools and stale metrics become neutral | unit | `go test ./internal/routing -run TestCompositeNormalization` | no, Wave 0 |
| OBS-01 | traces include safe lifecycle attributes and exclude prompts/identity/secrets | unit/integration | `go test ./internal/observability ./internal/gateway -run TestTrace` | no, Wave 0 |
| OBS-02 | Prometheus metrics expose low-cardinality labels and histograms | unit | `go test ./internal/observability -run TestPrometheus` | no, Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/routing ./internal/health ./internal/controlstate ./internal/observability`
- **Per wave merge:** `go test ./...` if it stays under the required 60-second backend timeout
- **Phase gate:** full suite green plus manual scrape/trace smoke if external collector/scraper is configured

### Wave 0 Gaps

- [ ] `internal/routing/composite_test.go` -- covers ROUT-01, ROUT-02, ROUT-03.
- [ ] `internal/controlstate/routing_config_test.go` -- covers config validation, migration, persistence, and last-known-good activation.
- [ ] `internal/observability/prometheus_test.go` -- covers OBS-02 and label allowlist.
- [ ] `internal/observability/tracing_test.go` or gateway lifecycle tests -- covers OBS-01 sanitization and timing attributes.

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | no new auth | Existing API-key auth remains unchanged. [VERIFIED: codebase grep] |
| V3 Session Management | no | Phase 11 owns BFF/session work. [CITED: ROADMAP.md] |
| V4 Access Control | yes, config mutation paths | Reuse existing admin/controlstate validation and hot reload; do not add unauthenticated config writes. [VERIFIED: codebase grep] |
| V5 Input Validation | yes | Extend `ValidateRoutingConfig` to reject invalid weights, thresholds, windows, warm-up, presets, and overrides. [VERIFIED: codebase grep] |
| V6 Cryptography | no new crypto | Do not change provider secret encryption. [VERIFIED: codebase grep] |
| V7 Error Handling and Logging | yes | Record sanitized error categories only; no raw prompts, secrets, payloads, auth headers, user IDs, or API keys. [CITED: 10-CONTEXT.md] |
| V10 Malicious Code | yes for dependencies | Use official OTel/Prometheus modules only; no unverified observability packages. [VERIFIED: Go module registry] |

### Known Threat Patterns for Go Gateway Observability

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Prompt/API-key leakage in spans or labels | Information Disclosure | Centralize allowlisted telemetry attributes; tests scan exported spans/metrics for forbidden keys. [CITED: 10-CONTEXT.md] |
| Metric cardinality explosion | Denial of Service | Fixed low-cardinality labels only; never label by request/user/API key. [CITED: 10-CONTEXT.md] |
| Unsafe routing config degrades reliability | Tampering | Structured validation rejects risky config and runtime retains last known good. [CITED: 10-CONTEXT.md] |
| SQL injection in config persistence | Tampering | Continue repository parameterized SQL pattern; do not build string SQL from operator inputs. [VERIFIED: codebase grep] |

## Sources

### Primary (HIGH confidence)

- `.planning/PROJECT.md` -- v7.1 milestone scope, architecture constraints, security boundaries.
- `.planning/REQUIREMENTS.md` -- ROUT-01/02/03 and OBS-01/02.
- `.planning/ROADMAP.md` -- Phase 10 scope and deferred Phase 11-13 boundaries.
- `.planning/phases/10-advanced-routing-observability/10-CONTEXT.md` -- locked Phase 10 decisions.
- `internal/routing/router.go` -- router seam and strategy switch.
- `internal/health/store.go`, `internal/health/redis_store.go` -- scoring input snapshots.
- `internal/controlstate/repository.go`, `types.go`, `validation.go`, `runtime.go` -- durable config and activation seams.
- `internal/observability/metrics.go` -- metrics interface to extend.
- `internal/gateway/service.go` -- request, fallback, cache, streaming, and outcome hooks.
- `go.mod` and `go list -m -versions` -- Go module baseline and external module versions.

### Secondary (MEDIUM confidence)

- Context7 `/open-telemetry/opentelemetry-go` -- tracer, span, error recording, and setup examples.
- Go module registry output for OTel and Prometheus modules.

### Tertiary (LOW confidence)

- Assumption that no external statistics package is needed.
- Assumption that Go package legitimacy audit can rely on official repo provenance plus module registry where GSD seam lacks Go ecosystem support.

## Metadata

**Confidence breakdown:**
- Standard stack: MEDIUM -- external OTel docs and Go module versions verified; Prometheus docs were version-verified via Go registry but not deeply queried through Context7.
- Architecture: HIGH -- existing seams verified directly in source files.
- Pitfalls: MEDIUM -- grounded in current code and locked decisions; exact fallback error shape remains an implementation choice.

**Research date:** 2026-06-30
**Valid until:** 2026-07-30 for local architecture; 2026-07-07 for external observability module versions.
