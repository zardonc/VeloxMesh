---
phase: 29-semantic-cache-latency-hardening
plan: "01"
subsystem: gateway-cache
tags: [go, semantic-cache, api-key-auth, sqlite]
requires:
  - phase: 28-tool-calling-protocol-completion
    provides: tool-protocol bypass contract
provides:
  - Trusted, default-off static FAQ cache eligibility with opaque versioned scopes
  - Configured embedding model and dimension validation
affects: [29-02, 29-03, semantic-cache]
actuals:
  tokens: 10193
  tasks: 2
  commits: 2
  plan_head_before: 4b18a99c9e7e816bd6d5995cda7d00db80998ebe
tech-stack:
  added: []
  patterns: [server-owned cache profile, opaque cache scope]
key-files:
  created: []
  modified:
    - internal/cache/semantic.go
    - internal/gateway/service.go
    - tests/integration/semantic_cache_test.go
key-decisions:
  - "Only configured non-admin API-key IDs may use the test-only static FAQ profile."
  - "Knowledge version and embedding identity participate in an opaque cache scope."
requirements-completed: [CACHE-F01]
coverage:
  - id: D1
    description: Trusted FAQ cache profile bypasses development, admin, tool, and unprofiled requests.
    requirement: CACHE-F01
    verification:
      - kind: integration
        ref: tests/integration/semantic_cache_test.go#TestSemanticCache_CacheHeaders
        status: pass
      - kind: unit
        ref: internal/gateway/service_tool_test.go#TestNoToolsWithoutTrustedProfileBypassesSemanticCache
        status: pass
    human_judgment: false
  - id: D2
    description: Configured model, vector dimension, malformed-choice rejection, and version scope isolation are enforced.
    requirement: CACHE-F01
    verification:
      - kind: unit
        ref: internal/cache/semantic_test.go#TestSemanticCacheUsesConfiguredEmbeddingModel
        status: pass
      - kind: unit
        ref: internal/cache/semantic_test.go#TestSemanticCacheKnowledgeVersionChangesScope
        status: pass
    human_judgment: false
duration: 1h
completed: 2026-09-26
status: complete
---

# Phase 29 Plan 01: Eligibility, Configuration, Isolation Summary

**A default-off, server-owned FAQ cache path now uses a database-authenticated non-admin key and opaque versioned scope, with no production enablement.**

## Performance

- **Duration:** 1h
- **Tasks:** 2
- **Files modified:** 12
- **Verification:** `go test -timeout 60s ./internal/config ./internal/cache ./internal/gateway ./tests/integration`, `go test -timeout 60s ./tests/integration -run TestSemanticCache -count=1`, and `go build ./...`

## Accomplishments

- Added explicit embedding model, dimension, TTL, threshold, candidate limit, and trusted FAQ profile configuration.
- Bound cache lookup/store to a hashed identity covering authenticated key ID, use case, FAQ version, target model, fixed prompt, deterministic generation settings, and embedding identity.
- Enforced zero cache I/O for tool traffic and profile-less keys; a SQLite integration test creates the allowed non-admin key in its isolated test database.
- Rejected wrong vector dimensions and malformed stored choices; cache hits retain zero-token Usage without upstream settlement.

## Task Commits

1. **Task 1: Write red gateway and configuration contract tests** - `0bc740e` (`test`)
2. **Task 2: Implement the narrow eligible path with existing components** - `f471376` (`feat`)

## Decisions Made

- Cache eligibility never reads a client-controlled use case or knowledge-version selector.
- `faq-v1` and `faq-v2` derive distinct opaque scopes, so old entries cannot match after a version change.
- Existing legacy configurations without a trusted profile remain inert: application wiring does not construct a cache service.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Compatibility] Profile-less legacy cache settings are inert**
- **Found during:** Task 2
- **Issue:** Existing deployment fixtures enable the legacy cache flag without a trusted profile, model, or bounds.
- **Fix:** Preserved their startup compatibility while preventing cache service construction until a complete profile is configured.
- **Files modified:** `internal/config/config_validation.go`, `internal/app/semantic_cache.go`
- **Verification:** Full config and gateway test suite passed.

## Issues Encountered

- `CACHE-F01` is named by the plan but is absent from the current requirements registry, so it could not be marked complete automatically.

## User Setup Required

None. Production cache remains disabled; no `.env.local` values were read or written.

## Next Phase Readiness

29-02 can add measured lookup and worker bounds on top of the immutable scope contract. No deployment or real-provider validation was performed.

## Self-Check: PASSED

- Confirmed `internal/cache/semantic.go` and `tests/integration/semantic_cache_test.go` exist.
- Confirmed task commits `0bc740e` and `f471376` exist in repository history.
