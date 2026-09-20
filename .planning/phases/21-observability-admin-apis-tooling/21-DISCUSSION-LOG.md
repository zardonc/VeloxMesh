# Phase 21: Observability, Admin APIs & Tooling - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-05T22:21:21-07:00
**Phase:** 21-Observability, Admin APIs & Tooling
**Areas discussed:** Status endpoint contract, SLA rules admin API, Training sample export, Operator tuning/tooling

---

## Status Endpoint Contract

| Question | Option | Description | Selected |
|---|---|---|---|
| Status route | New `/admin/v1/scheduler/status` | Keeps rollout GET/PATCH compatible and gives queue/executor/breaker health its own endpoint. | yes |
| Status route | Expand `/admin/v1/scheduler/rollout` | Fewer routes, but mixes rollout controls with operational status. | |
| Status route | Both routes return status | Convenient, but more compatibility surface to maintain. | |
| Unavailable pieces | Partial status with `warnings` | Operators still get queue/executor/breaker data if rollups or repo reads fail. | yes |
| Unavailable pieces | Fail the whole request | Cleaner error semantics, less useful during incidents. | |
| Unavailable pieces | Omit unavailable fields | Shortest response, but ambiguous for clients. | |
| Rollup selection | Latest 100 by bucket time | Simple and matches current repo limit style. | |
| Rollup selection | Client-controlled `limit`, default 100 | Useful for tools, with validation and a default. | yes |
| Rollup selection | Fixed latest 20 | Lighter response, less useful for debugging. | |
| Health detail | Per-component summary | `queue.depth`, `executor.slots_used/slots_total`, and breaker state per scorer when available. | yes |
| Health detail | Flat minimal fields | Simpler but less precise for heuristic/ONNX split. | |
| Health detail | Raw breaker counters/timestamps | Useful for debugging, but more internal surface. | |

**User's choice:** New status route, partial responses with warnings, `limit` query parameter defaulting to `100`, and per-component status summaries.
**Notes:** Status area was considered clear enough after these decisions.

---

## SLA Rules Admin API

| Question | Option | Description | Selected |
|---|---|---|---|
| Update shape | Replace whole in-memory rule set | Simplest and atomic; operators submit desired current rules. | yes |
| Update shape | Patch individual rules by `policy_id` | Friendlier for small edits, more validation/merge surface. | |
| Update shape | Append-only adds plus clear endpoint | Safer from accidental overwrite, awkward for corrections. | |
| Restart behavior | Revert to config-file rules | Matches runtime-updatable in-memory requirement; no hidden persistence. | yes |
| Restart behavior | Persist to control-state | Survives restart, but expands Phase 21 beyond in-memory. | |
| Restart behavior | Opt-in persistence later | Punts persistence cleanly. | |
| Invalid submission | Reject whole replacement | Atomic and avoids mixed old/new rule state. | yes |
| Invalid submission | Accept valid rules and report rejected ones | Flexible, easy to misread operationally. | |
| Invalid submission | Keep old rules and return warnings with 200 | Nonstandard for a write endpoint. | |
| Audit payload | Counts and safe rule keys only | Old/new counts plus policy IDs and safe dimensions. | yes |
| Audit payload | Full submitted rules | More reproducible, but larger and riskier. | |
| Audit payload | Counts only | Safest, but harder to reconstruct operator intent. | |

**User's choice:** Atomic full replacement, in-memory lifetime only, reject invalid replacements, audit counts and safe rule keys only.
**Notes:** Route and exact response shape are planner discretion within the existing admin scheduler namespace.

---

## Training Sample Export

| Question | Option | Description | Selected |
|---|---|---|---|
| Export format | JSON list with metadata | Easy for operators/tools; includes `items`, `count`, filters used, and warnings. | default |
| Export format | NDJSON stream | Better for huge exports, more plumbing. | optional |
| Export format | CSV download | Convenient for spreadsheets, worse for nested feature/label fields. | |
| Filters | Optional `start`, `end`, `task_type`, `limit` | Easy admin use, bounded by default. | yes |
| Filters | Require `start` and `end` | Safer for large stores, more typing. | |
| Filters | Require `start`, `end`, and `task_type` | Strictest, annoying for broad debugging. | |
| Fields | Split `features` and `labels` objects | Mirrors training data shape while keeping raw prompts/embeddings out. | yes |
| Fields | Flat safe row | Simpler for scripts, noisier and less self-describing. | |
| Fields | Minimal labels plus sample IDs only | Safest, but not enough to train/debug. | |
| Safety | Include low-cardinality fields only | Include provider class, model/request kind, priority, coverage; exclude tenant/user IDs and raw text. | yes |
| Safety | Include tenant IDs too | Useful for tenant analysis, but higher privacy risk. | |
| Safety | Strip provider class too | Safest, but loses useful completion-label context. | |
| Bounds | Default 1000, max 10000 | Enough for tooling, bounded for admin API safety. | yes |
| Bounds | Default 100, max 1000 | Safer and smaller, less useful for export. | |
| Bounds | No max when admin authenticated | Flexible, risky on large stores. | |

**User's choice:** Support JSON and NDJSON, with JSON default. Use optional filters, split `features` and `labels`, low-cardinality fields only, default `1000`, max `10000`.
**Notes:** Exclude tenant/user IDs, raw task text, raw prompts, embeddings, semantic-cache payloads, provider payloads, auth headers, API keys, and secrets.

---

## Operator Tuning/Tooling

| Question | Option | Description | Selected |
|---|---|---|---|
| Embedding model config | Dedicated scheduler setting | `scheduler.semantic_neighbors_embedding_model` / `SCHEDULER_SEMANTIC_NEIGHBORS_EMBEDDING_MODEL`, defaulting to current model. | yes |
| Embedding model config | Reuse semantic cache model config | Fewer knobs, but couples different features. | |
| Embedding model config | Require explicit model | Clear, but more config friction. | |
| `ListByIDs` behavior | Return found samples only, preserving requested order | Simple for semantic-neighbor hydration; missing IDs reduce coverage. | yes |
| `ListByIDs` behavior | Error on any missing ID | Stricter, but vector metadata drift breaks enrichment. | |
| `ListByIDs` behavior | Return unordered DB order | Easiest SQL, but caller must sort. | |
| Heuristic config | Override base latency and model multipliers only | Matches requirement and keeps file small. | yes |
| Heuristic config | Override full heuristic config | More power, wider validation surface. | |
| Heuristic config | Template only, no loader | Not enough for OBS-06. | |
| SchedulerType attribution | Populate everywhere `ScoreResult` is produced | FIFO, heuristic, ONNX, predictive, fallback/merged paths all set it before quality evidence. | yes |
| SchedulerType attribution | Only fix gRPC/proto decode path | Smallest patch, may leave other paths unattributed. | |
| SchedulerType attribution | Fill missing type at quality-recording time | Masks upstream gaps. | |

**User's choice:** Dedicated semantic-neighbor embedding model setting, ordered found-only `ListByIDs`, narrow heuristic override loader plus template, and full SchedulerType attribution at every producer.
**Notes:** Tuning area was considered clear enough after these decisions.

---

## Agent Discretion

- Exact JSON field names, helper names, route constants, warning text, template file location, and validation error text.
- Exact route for SLA rules inside the admin scheduler namespace.
- Exact mechanism for requesting NDJSON, with preference for the smallest existing handler pattern.

## Deferred Ideas

None - discussion stayed within phase scope.
