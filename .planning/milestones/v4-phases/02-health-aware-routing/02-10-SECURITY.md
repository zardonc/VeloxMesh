---
phase: 2.10
slug: adapter-conformance-test-harness
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-17
updated: 2026-06-17
mode: plan-time-STRIDE
---

# Phase 2.10 - Security

Per-phase security contract: plan-time STRIDE threat register, mitigation evidence, accepted risks, and audit trail.

`02-10-PLAN.md` included a parseable `<threat_model>` block with four mitigation threats and one accepted package-install control note. This audit verifies the planned mitigations against the implemented adapter conformance harness, adapter tests, UAT evidence, and command output.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Adapter tests to fake upstream | Adapter tests send provider-shaped requests to deterministic local `httptest.Server` instances or SDK base URLs. | Fake model names, fake prompts, fake API keys, provider-shaped fake responses |
| Adapter to returned error | Provider-native or fake-upstream failures are mapped into gateway-safe structured errors. | Provider error category, sanitized message, HTTP status |
| Production packages to `adaptertest` | Production code must not depend on test-only conformance helpers. | Test-only harness types and assertions |
| Future adapter authors to harness contract | New adapters use local fake upstream behavior and the shared harness before registration. | Adapter-specific test setup, expected provider-neutral outcomes |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-2.10-01 | Information Disclosure | Adapter error mapping | mitigate | Harness secret-safety checks reject returned errors containing fake API keys, authorization headers, raw prompts, raw upstream bodies, or sensitive provider-native payload fragments. | closed |
| T-2.10-02 | Tampering | `adaptertest` import boundary | mitigate | Verification checks `adaptertest` imports outside adapter tests and the helper package. | closed |
| T-2.10-03 | Denial of Service | Accidental live provider calls in tests | mitigate | Harness and adapter tests use deterministic fake servers/SDK base URLs, plus scope grep/review searches for accidental live calls. | closed |
| T-2.10-04 | Spoofing | Fake credentials in tests | mitigate | Tests use fake credentials only and do not require real provider credentials or external network access. | closed |
| T-2.10-SC | Tampering | Package installs | accept | Phase 2.10 did not add npm, pip, cargo, or Go module dependencies; no package-install legitimacy gate was required. | closed |

## Threat Verification

| Threat ID | Evidence |
|-----------|----------|
| T-2.10-01 | `AssertSecretSafeError` rejects forbidden substrings in returned errors (`internal/providers/adaptertest/harness.go:179`); each conformance spec supplies forbidden fake secret/header substrings (`internal/providers/openai/adapter_test.go:219`, `internal/providers/anthropic/adapter_test.go:240`, `internal/providers/gemini/adapter_test.go:260`); `RunConformance` applies secret-safety checks to every configured error case (`internal/providers/adaptertest/harness.go:162`); adapter error mapping returns structured `GatewayError` values with generic messages (`internal/providers/openai/adapter.go:127`, `internal/providers/anthropic/adapter.go:177`, `internal/providers/gemini/adapter.go:182`). |
| T-2.10-02 | `adaptertest` references are confined to adapter test files, helper package files, and helper documentation; `rg -n "adaptertest" internal cmd tests` found no production imports outside `internal/providers/adaptertest` and `*_test.go`; generic runtime packages do not import provider-specific adapters (`rg -n "internal/providers/(openai\|anthropic\|gemini)" internal/http internal/gateway internal/routing internal/config` returned no matches during UAT). |
| T-2.10-03 | Current conformance tests create deterministic local fake upstreams with `httptest.NewServer` (`internal/providers/openai/adapter_test.go:194`, `internal/providers/anthropic/adapter_test.go:215`, `internal/providers/gemini/adapter_test.go:235`); `adaptertest` README requires fake upstream behavior and explicitly forbids live network calls or credentials (`internal/providers/adaptertest/README.md:11`); scope grep for `http.Get(`, `http.Post(`, `models.list`, and `live provider` found no new live-call implementation paths. |
| T-2.10-04 | Conformance specs use fake keys such as `test-key` and assert they do not leak (`internal/providers/openai/adapter_test.go:219`, `internal/providers/anthropic/adapter_test.go:240`, `internal/providers/gemini/adapter_test.go:260`); no real provider credentials are required by `go test ./internal/providers/adaptertest ./internal/providers/openai ./internal/providers/anthropic ./internal/providers/gemini`; the README guides future authors to use fake upstreams and no credentials. |
| T-2.10-SC | `go.mod` and `go.sum` were not changed for this phase, and no package-install commands were required. |

## Summary Threat Flags

No `02-10-SUMMARY.md` artifact exists, and therefore no `## Threat Flags` section was available to parse. UAT found no gaps or issues.

## Accepted Risks Log

| Risk ID | Threat | Rationale | Owner | Review |
|---------|--------|-----------|-------|--------|
| T-2.10-SC | Package-install legitimacy gate not applicable | Phase 2.10 added test harness code and tests only, with no new package dependencies or install steps. | engineering | Revisit if future adapter harness work adds dependencies. |

## Residual Low Risk

The harness verifies error secret-safety only for adapter error cases included in each adapter's conformance spec. This is acceptable for Phase 2.10 because each current adapter includes deterministic fake upstream cases for reachable categories, and future adapter authors are instructed to define all deterministic success/error/health cases before registration.

## Verification

| Command | Result |
|---------|--------|
| `go test ./internal/providers/adaptertest ./internal/providers/openai ./internal/providers/anthropic ./internal/providers/gemini` | pass |
| `go test ./...` | pass |
| `rg -n "adaptertest" internal cmd tests` | reviewed; no production imports outside helper package and adapter tests |
| `rg -n "sk-\|Authorization:\|Bearer \|raw prompt\|raw upstream\|api[_-]?key" internal/providers internal/providers/adaptertest README.md` | reviewed; matches are fake fixtures, adapter header-setting code, README placeholders, or negative assertions |
| `rg -n "http\\.Get\\(\|http\\.Post\\(\|models\\.list\|live provider\|Admin API\|Admin Console\|postgres\|redis\|streaming\|tool calling\|multimodal\|rate limit\|semantic cache\|cost governance" internal tests README.md` | reviewed; matches are deferred README notes, existing capability flags, or existing error categories |

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-17 | 5 | 5 | 0 | Codex security auditor |

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-17
