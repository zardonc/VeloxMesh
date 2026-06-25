# Phase 2.2: Go Version Baseline for Official Provider SDKs - Context

**Gathered:** 2026-06-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2.1 completed health-aware routing across multiple OpenAI-compatible providers. The original Phase 2.2 provider-adapter scope is being moved to Phase 2.3 because the Anthropic adapter should use Anthropic's official Go SDK where practical.

Phase 2.2 is now a foundation phase: confirm and standardize the Go version baseline required by official provider SDKs, especially Anthropic's official SDK, and prove the existing gateway still builds and tests under that baseline.

</domain>

<decisions>
## Implementation Decisions

### Phase Split
- **D-01:** Move native Anthropic/Gemini provider adapter implementation from Phase 2.2 to Phase 2.3.
- **D-02:** Phase 2.2 should only handle Go version/toolchain readiness and verification for official SDK adoption.
- **D-03:** Phase 2.3 will implement native provider adapters and OpenAI-compatible response normalization.

### Go Baseline
- **D-04:** Anthropic's official Go SDK currently requires Go 1.24+.
- **D-05:** Current `go.mod` already declares `go 1.26.1`, which satisfies Anthropic SDK's Go 1.24+ requirement.
- **D-06:** Phase 2.2 should validate that the local developer/CI toolchain can actually run this module version, not only that `go.mod` declares it.
- **D-07:** If CI, README, Makefile, Dockerfile, or other developer setup docs still mention Go 1.22+, update them to the selected baseline.
- **D-08:** Prefer a single project-wide Go baseline. Do not add build tags or split module tricks just to support older Go versions.

### Verification
- **D-09:** Phase 2.2 must run the existing test suite after Go baseline confirmation:
  - `go version`
  - `go mod tidy`
  - `go test ./...`
- **D-10:** If dependency resolution is needed and network access is restricted, rerun with approved network access rather than silently skipping verification.
- **D-11:** If the local sandbox blocks `go` process execution, record that as an environment blocker and let the next execution step run the same commands in a permitted shell.

### Official SDK / Reference Rule
- **D-12:** User preference is to use Anthropic's official SDK implementation where possible.
- **D-13:** Phase 2.3 should start from the official SDK, not from a hand-written Anthropic HTTP adapter, unless implementation discovers a concrete blocker.
- **D-14:** If a concrete SDK blocker appears in Phase 2.3, document it with evidence and ask before falling back to local HTTP implementation.

### the agent's Discretion
The planner/executor may choose whether Phase 2.2 is a docs-only verification phase or includes small config/tooling edits, depending on what `README.md`, CI, Makefile, and local scripts currently say about Go version.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase Context
- `.planning/phases/02-health-aware-routing/02-CONTEXT.md` - Phase 2 locked decisions for health-aware routing, static provider config, registry behavior, and deferred scope.
- `.planning/phases/02-health-aware-routing/02-01-PLAN.md` - Completed Phase 2.1 implementation plan and package boundaries.
- `.planning/phases/02-health-aware-routing/02-03-CONTEXT.md` - Follow-up native provider adapter context after migration.

### Gateway Architecture
- `C:\Users\inthe\IdeaProjects\Notes-sur-l-IA\Projects\Agent-gateway\gateway-architecture.md` - Source architecture. Relevant sections: Provider Adapter System, Request Processing Pipeline, API Design, Provider Health Tracking.

### Current Code Integration Points
- `go.mod` - Current module declares `go 1.26.1`; verify local and CI toolchains match.
- `go.sum` - May change if `go mod tidy` normalizes dependencies.
- `README.md` - Update any stale Go version instructions.
- `Makefile` - Verify existing commands work under the selected Go baseline.
- `.github/workflows/*` - If present, update Go setup version to match the selected baseline.
- `Dockerfile` / `docker-compose.yml` - If present, update Go image/toolchain references.

### Provider References
- `https://github.com/anthropics/anthropic-sdk-go` - Official Anthropic Go SDK reference. Note current README requirement: Go 1.24+.
- `https://platform.claude.com/docs/en/api/messages` - Anthropic Messages API reference for request/response shape and content blocks.
- `https://github.com/googleapis/go-genai` - Official Google Gen AI Go SDK repository.
- `https://pkg.go.dev/google.golang.org/genai` - Go package documentation for Gemini SDK.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `go.mod`: Already declares `go 1.26.1`, so this phase likely focuses on validation and cleanup rather than a large upgrade.
- Existing Go tests: The current suite is the regression safety net for the toolchain baseline.

### Established Patterns
- Keep dependency/toolchain changes small and verify with `go test ./...`.
- Do not mix provider feature implementation into baseline/toolchain validation.

### Integration Points
- Confirm local `go version` is compatible with `go.mod`.
- Run `go mod tidy` and inspect whether it changes `go.mod` or `go.sum`.
- Run `go test ./...`.
- Update docs/CI/tooling only where stale Go version references exist.

</code_context>

<specifics>
## Specific Ideas

- User prefers using Anthropic's official SDK implementation.
- User requested migrating the provider-adapter work from 02-02 to 02-03.
- User requested Phase 02-02 handle Go version update and tests first.

</specifics>

<must_build>
## Must Build In Phase 2.2

- Confirm selected Go baseline supports Anthropic official SDK.
- Verify `go.mod` Go version is intentional and compatible with local/CI tooling.
- Update stale Go version references in docs, CI, Makefile, Dockerfile, or scripts if found.
- Run `go mod tidy`.
- Run `go test ./...`.
- Record any environment blocker if tests cannot execute.

</must_build>

<deferred>
## Deferred Ideas

- Native Anthropic adapter using official SDK: Phase 2.3.
- Native Gemini adapter: Phase 2.3.
- Provider response normalization: Phase 2.3.
- Any provider feature work beyond dependency/toolchain readiness.

</deferred>

<success_criteria>
## Success Criteria

- Go baseline is documented and compatible with Anthropic official SDK requirements.
- Existing code builds and tests under the selected Go version.
- Stale Go 1.22 references are removed or updated where relevant.
- Phase 2.3 can proceed with Anthropic official SDK adoption without reopening Go version questions.
- No provider adapter implementation is added in Phase 2.2.
</success_criteria>

---

*Phase: 2.2-Go Version Baseline for Official Provider SDKs*
*Context gathered: 2026-06-15*
