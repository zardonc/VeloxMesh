# Phase 10: Advanced Routing & Observability - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-30
**Phase:** 10-advanced-routing-observability
**Areas discussed:** Composite score policy, Cold-start and fallback behavior, Observability surface, Configuration and rollout

---

## Composite Score Policy

| Question | Options Presented | Selected |
|---|---|---|
| How should the router prioritize the scoring signals? | Reliability first; Latency first; Balanced default; Other | Balanced default |
| How should cost behave inside that balanced score? | Tie-breaker cost; Weighted cost; Budget-aware cost; Other | Tie-breaker cost |
| How should the router handle provider health inside the score? | Hard exclude unhealthy, penalize degraded; Score everything except hard failures; Two-lane routing; Other | Hard exclude unhealthy, penalize degraded |
| How should z-score normalization handle tiny candidate pools? | Guarded z-score; Always z-score; No normalization fallback; Other | Guarded z-score |

**User's choice:** Balanced scoring with cost as tie-breaker, hard exclusion of unhealthy providers, degraded-provider penalty, and guarded z-score behavior.
**Notes:** Preserve existing unhealthy exclusion and avoid cost-driven quality regressions.

---

## Cold-Start And Fallback Behavior

| Question | Options Presented | Selected |
|---|---|---|
| When provider metrics are missing or stale, what should the router do? | Round-robin until warm; Health-probe seeded; Static priority seed; Other | Round-robin until warm |
| When all candidates are weak, what should happen? | Best available; Fallback chain; Fail fast; Other | Fallback chain |
| How should stale metrics be treated? | Stale becomes neutral; Stale gets penalized; Stale triggers warm-up mode; Other | Stale becomes neutral |
| What should count as warm enough for composite scoring? | Minimum successful requests; Minimum age window; Either live requests or probe data; Other | Minimum successful requests |

**User's choice:** Use round-robin warm-up until enough successful live requests exist, neutralize stale metrics, and use fallback chains below score threshold.
**Notes:** Keep traffic moving without over-trusting sparse or stale signals.

---

## Observability Surface

| Question | Options Presented | Selected |
|---|---|---|
| What should traces capture for each routed request? | Routing decision only; Request lifecycle; Deep score breakdown; Other | Request lifecycle |
| What Prometheus labels are allowed? | Low-cardinality only; Add route/account scope; Debug-only rich labels; Other | Low-cardinality only |
| Should score breakdowns be visible anywhere? | Trace attributes only; Debug sampled traces; Structured logs; Other | Trace attributes only |
| How should sensitive observability fields be handled? | Strict sanitization; Hashed identity labels; Configurable redaction; Other | Strict sanitization |

**User's choice:** Request lifecycle traces, low-cardinality Prometheus labels, selected-provider score summary in traces only, strict sanitization.
**Notes:** No raw prompts, auth headers, API keys, user/API-key labels, or sensitive provider payloads.

---

## Configuration And Rollout

| Question | Options Presented | Selected |
|---|---|---|
| How should composite routing be introduced? | Opt-in strategy; Default for new config; Replace least-latency; Other | Opt-in strategy |
| Where should score weights and thresholds live? | Static config only; Routing config in SQLite; Built-in defaults only for Phase 10; Other | Routing config in SQLite |
| How much tuning surface should Phase 10 expose? | Named conservative preset only; Editable weights and thresholds; Preset plus override; Other | Preset plus override |
| How should invalid or risky routing config be handled? | Reject at validation; Clamp values; Fallback to round-robin; Other | Reject at validation |

**User's choice:** Add opt-in `composite-score`, persist config in SQLite, provide conservative preset plus overrides, reject invalid config and retain last known good.
**Notes:** No default behavior change for existing deployments.

---

## the agent's Discretion

- Choose exact conservative default weights, warm-up sample count, stale window, threshold ranges, and validation bounds during planning.
- Keep implementation narrow and reuse existing router, health-store, control-state, and observability seams.

## Deferred Ideas

None.
