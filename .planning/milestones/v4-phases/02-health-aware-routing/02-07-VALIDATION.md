---
phase: 2.7
slug: provider-adapter-capability-contract
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-06-16
updated: 2026-06-16
---

# Phase 2.7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go test |
| **Config file** | `go.mod` |
| **Quick run command** | `go test ./internal/providers/... ./internal/routing ./internal/gateway` |
| **Full suite command** | `go test ./...` |
| **Estimated runtime** | ~2 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/providers/... ./internal/routing ./internal/gateway`
- **After every plan wave:** Run `go test ./...`
- **Before `$gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~2 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2.7-01-01 | 01 | 1 | Provider-neutral capability types and adapter contract | T-2.7-01 / T-2.7-04 | Capability metadata contains no provider-native SDK details and is copy-safe | unit | `go test ./internal/providers/...` | yes | green |
| 2.7-01-02 | 01 | 2 | OpenAI-compatible adapter reports conservative capabilities | T-2.7-05 | Adapter reports text chat only, `Streaming=false`, `ToolCalling=false` | unit | `go test ./internal/providers/openai` | yes | green |
| 2.7-01-03 | 01 | 2 | Anthropic adapter reports conservative capabilities | T-2.7-05 | Adapter reports text chat only, `Streaming=false`, `ToolCalling=false` | unit | `go test ./internal/providers/anthropic` | yes | green |
| 2.7-01-04 | 01 | 2 | Gemini adapter reports conservative capabilities | T-2.7-05 | Adapter reports text chat only, `Streaming=false`, `ToolCalling=false` | unit | `go test ./internal/providers/gemini` | yes | green |
| 2.7-01-05 | 01 | 2 | Registry exposes capability lookup/listing in stable order | T-2.7-03 / T-2.7-04 | Unknown providers cannot synthesize metadata; returned metadata is copied | unit | `go test ./internal/providers` | yes | green |
| 2.7-01-06 | 01 | 3 | All `ProviderAdapter` mocks compile after contract change | — | Test doubles cannot silently omit capability reporting | compile/unit | `go test ./internal/...` | yes | green |
| 2.7-02-01 | 02 | 1 | Router exposes provider-neutral capabilities | T-2.7-04 | Router returns copy-safe metadata from registry | unit | `go test ./internal/routing` | yes | green |
| 2.7-02-02 | 02 | 1 | Gateway service exposes provider-neutral capabilities | T-2.7-04 | Service returns copy-safe metadata without provider-specific imports | unit | `go test ./internal/gateway` | yes | green |
| 2.7-02-03 | 02 | 2 | `/readyz` includes compact secret-safe capability summaries | T-2.7-01 | Readiness omits API keys, base URLs, raw prompts, raw bodies, and SDK-native details | integration | `go test ./tests/integration -run "Ready"` | yes | green |
| 2.7-02-04 | 02 | 2 | `/v1/models` remains OpenAI-compatible | — | Model items do not include capability or provider-native fields | integration | `go test ./tests/integration -run "Models"` | yes | green |
| 2.7-02-05 | 02 | 2 | HTTP/gateway/routing remain provider-agnostic | — | No imports of concrete provider adapter packages outside app wiring | grep | `rg "internal/providers/(openai\|anthropic\|gemini)" internal/http internal/gateway internal/routing` | yes | green |

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Audit 2026-06-16

| Metric | Count |
|--------|-------|
| Gaps found | 5 |
| Resolved | 5 |
| Escalated | 0 |

### Gaps Filled

- Added direct `Capabilities()` tests for OpenAI-compatible, Anthropic, and Gemini adapters.
- Expanded registry tests to verify capability metadata copy-safety, not just model copy-safety.
- Added router capability accessor coverage with deterministic order and copy-safety assertions.
- Added gateway service capability accessor coverage with deterministic order and copy-safety assertions.
- Re-ran readiness/model integration checks and provider-specific import grep to confirm Plan 2 coverage.

### Verification Evidence

- `go test ./internal/providers/...` passed.
- `go test ./internal/routing ./internal/gateway` passed.
- `go test ./tests/integration -run "Ready|Models"` passed.
- `gofmt -l .` returned no files.
- `go vet ./...` passed with no output.
- `go test ./...` passed for all packages.
- `rg "internal/providers/(openai|anthropic|gemini)" internal/http internal/gateway internal/routing` returned no matches.

---

## Validation Sign-Off

- [x] All tasks have automated verification
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all missing references
- [x] No watch-mode flags
- [x] Feedback latency < 5 seconds
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-06-16
