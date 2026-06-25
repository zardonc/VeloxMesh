# Phase 6: Model Combo Feature (RR, Fusion, Capability-based routing) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-06-25
**Phase:** 6-Model Combo Feature (RR, Fusion, Capability-based routing)
**Areas discussed:** Combo definition surface, Combo-as-model naming, Fusion behavior, Capability filtering and fallback

---

## Combo Definition Surface

| Option | Description | Selected |
|--------|-------------|----------|
| Durable control state/Admin API | Persist combos as gateway configuration and manage them through backend/admin surfaces. | ✓ |
| Static config only | Define combos only in local/static config. | |
| Static seed plus durable runtime | Use static config as seed and durable state as runtime source. | |

**User's choice:** Combos need to be persisted as configuration in the system.
**Notes:** Backend automatically assigns requests to target providers according to the configured algorithm. For users, a combo is a new model.

---

## Combo-as-Model Naming

| Option | Description | Selected |
|--------|-------------|----------|
| Expose combos in `/v1/models` | Combo names appear as available models alongside provider-backed models. | ✓ |
| Hidden virtual models | Combo names are callable but not listed. | |
| Provider-only model list | `/v1/models` remains provider-backed only. | |

**User's choice:** `/v1/models` needs to return combos as model entries.
**Notes:** Combo names should be usable as normal model names by data-plane clients.

---

## Fusion Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Parallel panel plus judge | Query all combo models in parallel, then have a judge synthesize one answer. | ✓ |
| First successful response | Query multiple models and return the first acceptable response. | |
| Best simple response | Query multiple models and choose a response using a simple local heuristic. | |

**User's choice:** Query all models in parallel, then a judge synthesizes one answer.
**Notes:** This is best quality but most expensive: each request bills all panel models plus the judge, N+1 calls. The judge is user-configurable and must be an already-connected model in the current AI gateway.

---

## Capability Filtering and Fallback

| Option | Description | Selected |
|--------|-------------|----------|
| Capacity auto-switch | Send image/PDF/audio requests to a combo model that supports the needed capability first. | ✓ |
| All-members-required | Fail if any combo member cannot satisfy the request capability. | |
| Best-effort remaining | Filter unsupported members and continue with any eligible members. | |

**User's choice:** Add capacity auto-switch and ordered fallback.
**Notes:** Capacity auto-switch sends image, PDF, or audio requests to a model that supports them first. If a call fails, select the next provider in combo order.

---

## the agent's Discretion

- Choose the smallest durable schema/API shape that supports the locked combo behavior.
- Reuse existing router, registry, provider capability, and control-state patterns.

## Deferred Ideas

None.
