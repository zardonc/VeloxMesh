# Phase 22: Documentation, .env.example & UAT - Context

**Gathered:** 2026-07-06T15:13:32-07:00
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 22 completes the v7.6 Scheduler 1.0 wrap-up by making the implemented scheduler/config/admin behavior understandable, safely configurable, and verifiable by operators.

This phase covers README quick-path updates, a focused Scheduler 1.0 operator runbook, curated `.env.example` and structured JSON examples, Qdrant/pgvector semantic-neighbor guidance, and UAT evidence for scheduler enable/disable, degradation, semantic neighbors, admin APIs, config/example safety, disabled-by-default startup, and real-provider checks.

This phase does not add new scheduler runtime capabilities, new admin APIs, Admin Console UI, automatic rollout decisions, scheduler-owned queueing, scheduler-owned vector lookup, or new provider behavior.

</domain>

<decisions>
## Implementation Decisions

### Operator Runbook
- **D-01:** Split documentation: keep the README as the quick path and add one focused Scheduler 1.0 operator runbook for deployment, degradation, config, and UAT details.
- **D-02:** Optimize the runbook as a balanced operator guide: production-minded first, with local commands only where they prove behavior.
- **D-03:** Degradation coverage should use scenario playbooks for Scheduler down, Redis unavailable, Qdrant unavailable, ONNX predictor unhealthy, and admin API validation failure.
- **D-04:** The runbook should include grouped env/JSON config reference plus short copy/paste examples for local, scheduler-enabled, and semantic-neighbor setups.
- **D-05:** UAT guidance should be hybrid: manual steps plus references to existing unit/integration tests. Add only a tiny helper if it removes repeated command glue.

### Config Examples
- **D-06:** Keep `.env.example` curated. Include essential local and scheduler-facing vars inline, but move advanced scheduler detail to the runbook and component examples.
- **D-07:** Add one full structured config example so operators can see the complete nested shape, while keeping focused component examples for scheduler, cache, and heuristic overrides.
- **D-08:** Examples must reference placeholder environment variable names such as `OPENAI_PRIMARY_API_KEY`; never inline secret-shaped provider credentials or passwords.
- **D-09:** Document Qdrant and pgvector parity with one concise comparison table plus short examples for both backends.

### UAT Evidence
- **D-10:** Phase 22 UAT must prove roadmap paths plus config/docs safety: scheduler enable/disable, degradation, semantic neighbors, admin APIs, examples parse/load, disabled-by-default startup, and secret-safe examples.
- **D-11:** Produce a separate UAT report file with a checklist containing command, expected result, actual result, and notes.
- **D-12:** Full real-provider UAT should use `.env.local`. Prefer non-Gemini provider resources for routine real-provider checks; reserve Gemini resources for Gemini-specific scenarios.
- **D-13:** Failed UAT checks should include command output, root cause notes, and whether the issue is blocking or non-blocking.

### Planner Discretion
- Planner may choose exact doc filenames, section ordering, table columns, UAT report filename, and whether a tiny helper script or make target is worth adding.
- Keep the Phase 22 diff documentation-heavy. Reuse existing examples, tests, and command patterns before adding new helpers.
- Do not add new runtime behavior just to make docs nicer.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Planning
- `.planning/ROADMAP.md` - Phase 22 scope: docs, `.env.example`, operator runbook, Qdrant/pgvector parity, Scheduler purpose/enablement, and UAT.
- `.planning/REQUIREMENTS.md` - v7.6 requirements and out-of-scope boundaries.
- `.planning/PROJECT.md` - Project-level constraints: disabled-by-default optional systems, sensitive-payload boundaries, SQLite-first default path, and real-provider UAT resource notes.
- `.planning/phases/21-observability-admin-apis-tooling/21-CONTEXT.md` - Admin API, training export, quality rollup, heuristic config, and semantic-neighbor config decisions that docs/UAT must cover.
- `.planning/phases/20-config-unification-scheduler-core-hardening/20-CONTEXT.md` - Nested config, `.env.example`, `config.json.example`, component config files, executor concurrency, Redis locks, QueueGuard observability, and semantic-neighbor startup/config decisions.
- `.planning/phases/19-sla-waiting-time-promotion/19-CONTEXT.md` - SLA promotion degradation, safe audit/metrics, and priority-safety precedent.

### Documentation And Examples
- `README.md` - Current quick start, config overview, scheduler training, and scheduler rollout sections.
- `.env.example` - Current local env example and scheduler/cache/Redis variables.
- `config.json.example` - Current minimal structured config example.
- `config.scheduler.example.json` - Current scheduler component config example.
- `config.cache.example.json` - Current cache/Qdrant/pgvector component config example.
- `config.heuristic.example.json` - Current heuristic override example.
- `tools/scheduler_training/README.md` - Offline scheduler training/export workflow documentation.

### Validation And UAT Entry Points
- `Makefile` - Current `run`, `test`, `fmt`, and `vet` command surface.
- `internal/config/config_test.go` - Existing `.env.example` secret-safety and scheduler-disabled tests.
- `internal/http/handlers/admin_scheduler_test.go` - Admin scheduler API behavior and validation tests.
- `internal/scheduler/semantic_neighbors_test.go` - Semantic-neighbor enrichment, fallback, and config behavior tests.
- `internal/scheduler/client_test.go` - Scheduler scoring/fallback behavior tests.
- `internal/scheduler/queue_fallback_test.go` - Redis-to-memory queue fallback behavior tests.
- `internal/scheduler/quality_test.go` - Quality rollup attribution behavior.
- `internal/scheduler/sla_promotion_test.go` - SLA promotion and degraded/blocked behavior.
- `tests/integration/semantic_cache_test.go` - Semantic cache/vector integration coverage.
- `tests/integration/redis_hotstate_test.go` - Redis-backed hot-state integration coverage.
- `tests/integration/plan4_postgres_smoke_test.go` - PostgreSQL/pgvector-adjacent smoke coverage.
- `scripts/smoke/plan4-postgres.sh` - Existing gated Plan 4 smoke helper.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Existing example files already cover minimal structured config plus scheduler/cache/heuristic component overrides; Phase 22 can enrich them instead of inventing a new config format.
- `internal/config/config_test.go` already tests `.env.example` secret safety and scheduler-disabled defaults; extend these checks if examples become richer.
- Existing scheduler/admin/semantic-neighbor tests provide most UAT evidence without new runtime code.
- The Plan 4 smoke script is a useful pattern for gated external-service UAT.

### Established Patterns
- Optional Scheduler, Redis, Qdrant, semantic cache, ONNX, and semantic-neighbor features stay disabled by default and fail open.
- Documentation and examples must avoid API keys, auth headers, raw prompts, provider payloads, embeddings, semantic-cache payloads, passwords, and secret-shaped fake values.
- Config is moving toward named nested blocks with legacy env compatibility.
- Gateway owns queueing, execution, task state, semantic lookup, SLA promotion, fallback, and sensitive-payload boundaries.
- UAT commands must use a 60-second timeout for backend tests.

### Integration Points
- README should link to the focused Scheduler 1.0 operator runbook rather than carrying all operational detail inline.
- `.env.example` should stay readable and point operators to component examples/runbook for advanced scheduler settings.
- UAT report should connect manual checks to existing tests and any gated real-provider or service-backed checks.

</code_context>

<specifics>
## Specific Ideas

- Runbook shape: production-minded guide with local commands used only as proof points.
- Degradation playbooks: Scheduler down, Redis unavailable, Qdrant unavailable, ONNX predictor unhealthy, admin API validation failure.
- Config examples: curated `.env.example`, one full structured config example, richer component examples, no inline secret-looking values.
- Vector guidance: concise Qdrant vs pgvector comparison table plus short examples.
- UAT report: command, expected result, actual result, notes, command output on failure, root cause, and blocking/non-blocking classification.

</specifics>

<deferred>
## Deferred Ideas

None - discussion stayed within phase scope.

</deferred>

---

*Phase: 22-Documentation, .env.example & UAT*
*Context gathered: 2026-07-06T15:13:32-07:00*
