# Requirements: VeloxMesh

**Defined:** 2026-06-15
**Core Value:** Client applications can call one OpenAI-compatible gateway endpoint and reliably reach the right LLM provider through a low-latency, observable, provider-agnostic routing layer.

## v1 Requirements

### Gateway Foundation

- [x] **GW-01**: Developer can run a Go/Chi gateway binary locally.
- [x] **GW-02**: Gateway exposes `GET /healthz` for liveness.
- [x] **GW-03**: Gateway exposes `GET /readyz` for readiness.
- [x] **GW-04**: Gateway protects data-plane endpoints with static development bearer-token auth.
- [x] **GW-05**: Gateway assigns and propagates `X-Request-ID`.

### OpenAI-Compatible Chat

- [x] **CHAT-01**: Client can call `POST /v1/chat/completions` with OpenAI-compatible non-streaming chat JSON.
- [x] **CHAT-02**: Gateway validates malformed chat requests and invalid roles.
- [x] **CHAT-03**: Gateway rejects `stream: true` until streaming is intentionally implemented.
- [x] **CHAT-04**: Gateway returns successful provider responses in OpenAI-compatible `chat.completion` shape.
- [x] **CHAT-05**: Gateway returns structured errors for validation and provider failures.

### Provider Foundation

- [x] **PROV-01**: Gateway defines a provider adapter interface with `ID`, `Models`, `Complete`, and `HealthCheck`.
- [x] **PROV-02**: Gateway includes one OpenAI-compatible provider adapter.
- [x] **PROV-03**: Gateway includes a provider registry and exposes configured models through `/v1/models`.
- [ ] **PROV-04**: Gateway can load multiple provider definitions from static config.
- [ ] **PROV-05**: Gateway can register and list multiple OpenAI-compatible provider adapters.
- [ ] **PROV-06**: Gateway can add native Anthropic and Gemini adapters without changing HTTP handlers.

### Routing And Health

- [x] **ROUTE-01**: Gateway has a routing boundary between request handling and provider calls.
- [x] **ROUTE-02**: Gateway supports default provider routing and `X-Route-To` override for the configured provider.
- [ ] **ROUTE-03**: Gateway tracks in-memory provider health including EWMA latency, pending count, failures, and status.
- [ ] **ROUTE-04**: Gateway supports round-robin routing across routable providers.
- [ ] **ROUTE-05**: Gateway supports least-latency routing when latency samples exist.
- [ ] **ROUTE-06**: Gateway avoids unhealthy providers and returns a structured error when no provider is routable.

### Observability And Operations

- [x] **OBS-01**: Gateway uses structured `slog` logging.
- [x] **OBS-02**: Gateway records minimal in-process request metrics hooks.
- [ ] **OBS-03**: Gateway logs or records selected provider, routing strategy, provider health status, latency, pending count, and outcome.
- [ ] **OPS-01**: Project documents the active Go version baseline.
- [ ] **OPS-02**: Existing tests pass under the active Go version baseline.

## v2 Requirements

### Native Providers

- **NPROV-01**: Anthropic adapter uses the official Anthropic Go SDK where practical.
- **NPROV-02**: Gemini adapter evaluates the official Google Gen AI Go SDK before implementation.
- **NPROV-03**: Provider-native responses normalize into OpenAI-compatible internal `LLMResponse`.

### Control Plane And Storage

- [x] **CTRL-01**: Admin API can manage provider configuration.
- [x] **CTRL-02**: PostgreSQL/SQLite stores durable provider, API key, routing, usage, audit, idempotency, and provider-secret records.
- [x] **CTRL-03**: Redis stores hot provider health/probe state, data-plane auth-cache hot state, and config-change notifications when configured.

### Advanced Gateway Features

- **STRM-01**: Gateway supports SSE streaming proxy.
- **RATE-01**: Gateway enforces rate limits.
- **CACHE-01**: Gateway supports semantic cache.
- **COST-01**: Gateway tracks usage and cost.
- **CB-01**: Gateway supports circuit breaker and fallback-chain behavior.

## Out of Scope

| Feature | Reason |
|---------|--------|
| TypeScript/Node gateway implementation | Gateway architecture and current implementation are Go-first. |
| PostgreSQL/Redis in Phase 1/2.1 | Static and in-memory behavior should be proven before persistence/control state. |
| Admin Console in current milestone | Gateway data-plane and routing layer are higher priority. |
| SSE streaming in current milestone | Non-streaming chat path must be stable first. |
| Tool calling and multimodal normalization in first native adapter phase | Text chat normalization is the safer first vertical slice. |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| GW-01 | Phase 1 | Complete |
| GW-02 | Phase 1 | Complete |
| GW-03 | Phase 1 | Complete |
| GW-04 | Phase 1 | Complete |
| GW-05 | Phase 1 | Complete |
| CHAT-01 | Phase 1 | Complete |
| CHAT-02 | Phase 1 | Complete |
| CHAT-03 | Phase 1 | Complete |
| CHAT-04 | Phase 1 | Complete |
| CHAT-05 | Phase 1 | Complete |
| PROV-01 | Phase 1 | Complete |
| PROV-02 | Phase 1 | Complete |
| PROV-03 | Phase 1 | Complete |
| ROUTE-01 | Phase 1 | Complete |
| ROUTE-02 | Phase 1 | Complete |
| OBS-01 | Phase 1 | Complete |
| OBS-02 | Phase 1 | Complete |
| PROV-04 | Phase 2.1 | Pending |
| PROV-05 | Phase 2.1 | Pending |
| ROUTE-03 | Phase 2.1 | Pending |
| ROUTE-04 | Phase 2.1 | Pending |
| ROUTE-05 | Phase 2.1 | Pending |
| ROUTE-06 | Phase 2.1 | Pending |
| OBS-03 | Phase 2.1 | Pending |
| OPS-01 | Phase 2.2 | Pending |
| OPS-02 | Phase 2.2 | Pending |
| PROV-06 | Phase 2.3 | Pending |
| CTRL-01 | Phase 3 | Complete |
| CTRL-02 | Phase 3 | Complete |
| CTRL-03 | Phase 3 | Complete |

**Coverage:**
- v1 requirements: 26 total
- Mapped to phases: 26
- Unmapped: 0

---
*Requirements defined: 2026-06-15*
*Last updated: 2026-06-19 after Phase 3 durable control state UAT completion*
