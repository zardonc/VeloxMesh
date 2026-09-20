# Phase 22: Documentation, .env.example & UAT - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-06T15:13:32-07:00
**Phase:** 22-Documentation, .env.example & UAT
**Areas discussed:** Operator runbook shape, Config examples, UAT evidence details

---

## Operator Runbook Shape

| Option | Description | Selected |
|--------|-------------|----------|
| README section | Shortest path; everything visible in the existing main doc. | |
| Dedicated docs file | Cleaner for deployment/degradation/UAT details; README links to it. | |
| Split | README quick start plus focused runbook. | yes |

**User's choice:** Split docs.
**Notes:** README should stay lean; focused runbook carries deployment, degradation, config, and UAT detail.

| Option | Description | Selected |
|--------|-------------|----------|
| Local operator walkthrough | Practical local commands. | |
| Production deployment guide | Config, failure modes, degradation, rollout/rollback. | |
| Incident/debug reference | Admin status, metrics, quality rollups, export. | |
| Balanced operator guide | Production-minded, with local commands only where they prove behavior. | yes |

**User's choice:** Balanced operator guide.
**Notes:** Optimize for operators without losing local reproducibility.

| Option | Description | Selected |
|--------|-------------|----------|
| Checklist only | Name failure modes and expected fallback. | |
| Scenario playbooks | Scheduler down, Redis unavailable, Qdrant unavailable, ONNX unhealthy, admin validation failure. | yes |
| Full incident drills | Exact setup/teardown commands for each failure. | |

**User's choice:** Scenario playbooks.
**Notes:** Enough detail to guide operations without overbuilding drills.

| Option | Description | Selected |
|--------|-------------|----------|
| Operator-first table | Group env vars and JSON keys by area. | |
| Narrative examples | Explain recommended configs, less exhaustive. | |
| Both | Grouped reference table plus copy/paste examples. | yes |

**User's choice:** Both.
**Notes:** Include examples for local, scheduler-enabled, and semantic-neighbor setups.

| Option | Description | Selected |
|--------|-------------|----------|
| Manual checklist | Operators run commands manually. | |
| Automated smoke target | Add script or make target for local deps. | |
| Hybrid | Manual steps plus existing tests; tiny helper only if useful. | yes |

**User's choice:** Hybrid.
**Notes:** Add only the smallest helper if repeated command glue becomes noisy.

---

## Config Examples

| Option | Description | Selected |
|--------|-------------|----------|
| Keep all scheduler vars inline | Discoverable, but long. | |
| Minimal `.env.example` plus links | Cleaner, less grep-friendly. | |
| Curated inline essentials | Local setup stays readable; advanced vars live in runbook/examples. | yes |

**User's choice:** Curated `.env.example`.
**Notes:** Keep essentials inline and move advanced scheduler details elsewhere.

| Option | Description | Selected |
|--------|-------------|----------|
| Keep current separate files | No new example structure. | |
| Add richer separate files | Expand focused component examples. | |
| Add one full example plus component examples | Complete shape plus focused overrides. | yes |

**User's choice:** One full example plus component examples.
**Notes:** Operators should see both complete nested config and focused overrides.

| Option | Description | Selected |
|--------|-------------|----------|
| Empty values only | Safest but less instructive. | |
| Placeholder env var names only | Reference env vars; never inline secret-like values. | yes |
| Fake-looking local values | Friendlier demos but needs strict tests. | |

**User's choice:** Placeholder env var names only.
**Notes:** Use names like `OPENAI_PRIMARY_API_KEY`; no secret-shaped literals.

| Option | Description | Selected |
|--------|-------------|----------|
| Concise comparison table | When to use Qdrant vs pgvector plus config pointers. | |
| Separate sections | More setup detail. | |
| Table plus short examples | Enough guidance without a database manual. | yes |

**User's choice:** Table plus short examples.
**Notes:** Keep Qdrant/pgvector parity docs compact.

---

## UAT Evidence Details

| Option | Description | Selected |
|--------|-------------|----------|
| Required roadmap paths only | Scheduler enable/disable, degradation, semantic neighbors, admin APIs. | |
| Roadmap plus config docs safety | Also examples parse/load, disabled defaults, secret-safe examples. | yes |
| Full milestone smoke | Include Redis/Qdrant/Postgres-adjacent checks where services exist. | |

**User's choice:** Roadmap plus config/docs safety.
**Notes:** UAT must prove behavior and documentation/config safety.

| Option | Description | Selected |
|--------|-------------|----------|
| Test command list only | Exact commands only. | |
| UAT report file | Checklist with command, expected result, actual result, notes. | yes |
| Inline README/runbook section only | No separate UAT artifact. | |

**User's choice:** UAT report file.
**Notes:** Separate artifact is expected.

| Option | Description | Selected |
|--------|-------------|----------|
| No live services required | Optional checks are gated/skipped. | |
| Local Redis/Qdrant required | Provider credentials optional. | |
| Full real-provider UAT | Use `.env.local`. | yes |

**User's choice:** Full real-provider UAT using `.env.local`.
**Notes:** Prefer non-Gemini provider resources for routine checks.

| Option | Description | Selected |
|--------|-------------|----------|
| Strict pass/fail only | Any failed command blocks Phase 22. | |
| Pass/fail with gated skips | Missing optional services/credentials are skipped with prerequisite. | |
| Diagnostic report | Include output, root cause, blocking/non-blocking classification. | yes |

**User's choice:** Diagnostic report.
**Notes:** Failed checks should expose enough detail to debug root causes.

## Planner Discretion

- Exact doc filenames, section order, table columns, UAT report filename, and whether a tiny helper script or make target is useful.

## Deferred Ideas

None.
