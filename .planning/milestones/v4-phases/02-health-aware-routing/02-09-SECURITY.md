---
phase: 2.9
slug: provider-model-catalog-and-routing-eligibility
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-17
updated: 2026-06-17
mode: retroactive-STRIDE
---

# Phase 2.9 - Security

Per-phase security contract: retroactive STRIDE threat register, mitigation evidence, accepted risks, and audit trail.

No parseable `<threat_model>` block existed in `02-09-PLAN.md`, so this register was built from the implementation files listed in the audit prompt.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Client to HTTP handlers | Public OpenAI-compatible API receives model IDs, chat payloads, auth headers, and optional `X-Route-To`. | Client-controlled model ID, messages, route override, bearer token |
| HTTP handlers to gateway service | Validated request shape becomes internal `LLMRequest`. | Model ID, route override, stream flag, request ID |
| Gateway service to router | Provider selection happens before adapter execution. | Model ID, attempted provider exclusions, routing strategy |
| Router to provider registry/catalog | Eligibility decisions use internal catalog metadata. | Provider IDs, model IDs, provider-neutral capabilities |
| Gateway to provider adapters | Adapter `Complete` is the provider execution boundary. | Provider-selected request only after eligibility and health checks |
| Internal catalog to `/v1/models` | Internal model/provider eligibility is projected into public OpenAI-compatible JSON. | Model IDs only; no provider/capability metadata |
| Static config and adapter metadata to catalog | Catalog is derived at registry construction time. | Static provider config, adapter-declared models, adapter-declared capabilities |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-2.9-01 | Tampering | Model catalog and registry APIs | mitigate | Return defensive copies for model lists, provider slices, and capability metadata. | closed |
| T-2.9-02 | Elevation of privilege | Non-override routing | mitigate | Filter by catalog model and `chat_completions` eligibility before health filtering and strategy selection. | closed |
| T-2.9-03 | Spoofing / elevation of privilege | Strict `X-Route-To` override | mitigate | Verify override provider exists, supports the requested model and operation, and is healthy before execution. | closed |
| T-2.9-04 | Denial of service / elevation of privilege | Fallback routing | mitigate | Re-enter router through `SelectExcluding`, preserving catalog eligibility and excluding failed providers. | closed |
| T-2.9-05 | Repudiation / denial of service | Unknown or unsupported models | mitigate | Return deterministic structured eligibility errors before adapter execution. | closed |
| T-2.9-06 | Information disclosure | `/v1/models` and eligibility errors | mitigate | Public model JSON uses OpenAI-compatible fields only; eligibility error messages are generic gateway errors. | closed |
| T-2.9-07 | Tampering / information disclosure | Deferred feature boundaries | mitigate | Catalog uses static config plus adapter metadata only; no live discovery, Admin CRUD, DB-backed catalog, hot reload, streaming/tools/multimodal implementation, rate limiting, semantic cache, or cost governance was introduced in audited files. | closed |

## Threat Verification

| Threat ID | Evidence |
|-----------|----------|
| T-2.9-01 | `ModelProvider.Clone()` deep-copies capability metadata (`internal/providers/catalog.go:12`); `CapabilitySet.Clone()` deep-copies slices (`internal/providers/capabilities.go:47`); catalog stores cloned capabilities (`internal/providers/catalog.go:56`); `GetAllModels()` copies model IDs (`internal/providers/catalog.go:71`); `EligibleProviders()` returns cloned providers (`internal/providers/catalog.go:77`); registry copies IDs and capability/model lists (`internal/providers/registry.go:59`, `internal/providers/registry.go:103`); copy-safety tests mutate returned data and re-read (`internal/providers/registry_test.go:109`, `internal/providers/registry_test.go:143`, `internal/providers/registry_test.go:164`, `internal/routing/router_test.go:262`, `internal/gateway/service_test.go:105`). |
| T-2.9-02 | Router obtains eligible providers from `registry.EligibleProviders(req.Model, providers.OperationChatCompletions)` before health filtering (`internal/routing/router.go:48`); no eligible provider returns `ErrNoEligibleProvider` (`internal/routing/router.go:50`); only eligible providers enter health filtering (`internal/routing/router.go:53`, `internal/routing/router.go:83`); `ProviderSupports` requires model and operation support (`internal/providers/catalog.go:92`); router tests cover no eligible provider (`internal/routing/router_test.go:224`) and provider-specific model routing is covered by integration tests (`tests/integration/chat_test.go:306`, `tests/integration/chat_test.go:319`). |
| T-2.9-03 | Strict override resolves provider ID first (`internal/routing/router.go:100`), rejects model/operation-ineligible providers with `ErrIneligibleProviderOverride` (`internal/routing/router.go:105`), and rejects unhealthy overrides before execution (`internal/routing/router.go:109`); structured errors are defined in `internal/errors/errors.go:28` and `internal/errors/errors.go:30`; router tests cover unknown and ineligible override behavior (`internal/routing/router_test.go:181`, `internal/routing/router_test.go:233`); service disables fallback for override requests because fallback attempts require `req.RouteOverride == ""` (`internal/gateway/service.go:43`). |
| T-2.9-04 | Gateway fallback calls `SelectExcluding(ctx, req, attempted)` on each attempt (`internal/gateway/service.go:50`); retryable failures add the selected provider to `attempted` (`internal/gateway/service.go:115`); router applies catalog eligibility before exclusions and health filtering (`internal/routing/router.go:48`, `internal/routing/router.go:83`); integration test proves `p1-only` does not fallback to healthy but ineligible `p2` (`tests/integration/chat_test.go:332`, `tests/integration/chat_test.go:346`). |
| T-2.9-05 | Unknown models produce no eligible providers and return `ErrNoEligibleProvider` before `adapter.Complete` is reached (`internal/routing/router.go:48`, `internal/routing/router.go:50`, `internal/gateway/service.go:50`, `internal/gateway/service.go:71`); `no_eligible_provider` is a 400 structured gateway error (`internal/errors/errors.go:27`); router and integration tests cover unknown model rejection (`internal/routing/router_test.go:224`, `tests/integration/chat_test.go:299`). |
| T-2.9-06 | `/v1/models` response types expose only `id`, `object`, `created`, and `owned_by` fields (`internal/http/handlers/models.go:18`); handler populates model items from `service.GetAvailableModels()` and a constant owner, without provider IDs, capabilities, API keys, prompts, or provider-native payloads (`internal/http/handlers/models.go:36`, `internal/http/handlers/models.go:40`, `internal/http/handlers/models.go:44`); model endpoint integration asserts OpenAI-compatible object shape and deduplicated model IDs (`tests/integration/models_test.go:37`, `tests/integration/models_test.go:41`, `tests/integration/models_test.go:45`, `tests/integration/models_test.go:62`); eligibility errors use generic messages (`internal/errors/errors.go:27`, `internal/errors/errors.go:30`) and the chat handler returns `GatewayError` code/message directly for those errors (`internal/http/handlers/chat.go:64`). |
| T-2.9-07 | Provider adapter contract exposes static `Models()` and provider-neutral `Capabilities()` as metadata sources (`internal/providers/adapter.go:13`, `internal/providers/adapter.go:15`, `internal/providers/adapter.go:20`); catalog construction iterates registered adapters and reads `a.Capabilities()` plus `a.Models()` only (`internal/providers/catalog.go:31`, `internal/providers/catalog.go:36`, `internal/providers/catalog.go:45`, `internal/providers/catalog.go:46`); case-insensitive scope grep over audited implementation and tests for deferred features found only existing capability booleans (`Streaming`, `ToolCalling`), upstream 429 fallback tests, and comments, not implementations of live discovery, Admin CRUD, DB-backed catalog, hot reload, rate limiting, semantic cache, or cost governance. |

## Summary Threat Flags

`02-09-SUMMARY.md` has no `## Threat Flags` section. No unregistered threat flags were found.

## Accepted Risks Log

No accepted risks.

## Residual Low Risk

`/v1/models` intentionally exposes configured public model IDs as part of the OpenAI-compatible API contract. This is not treated as internal provider metadata because provider IDs, provider types, capability metadata, API keys, authorization headers, raw prompts, raw upstream bodies, and provider-native payload details are not included in the public model response.

## Verification

| Command | Result |
|---------|--------|
| `go test ./internal/providers ./internal/routing ./internal/gateway ./tests/integration` | pass |

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-17 | 7 | 7 | 0 | Codex security auditor |

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-17
