# Phase 19: SLA Waiting-Time Promotion - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md - this log preserves the alternatives considered.

**Date:** 2026-07-05T13:43:48-07:00
**Phase:** 19-SLA Waiting-Time Promotion
**Areas discussed:** Policy thresholds, Queue promotion mechanics, Priority and quota ceiling, Audit and metrics evidence

---

## Policy Thresholds

| Option | Description | Selected |
|--------|-------------|----------|
| Tenant + model class + request kind | Matches requirements and uses safe structured fields. | yes |
| Tenant only | Simpler, but too blunt for mixed workloads. | |
| Global defaults plus tenant overrides | Convenient fallback, but broader than the selected shape. | |
| Other | Freeform policy shape. | |

**User's choice:** Tenant + model class + request kind.
**Notes:** Rule dimensions must come from trusted identity/config and safe structured fields.

| Option | Description | Selected |
|--------|-------------|----------|
| No promotion | Missing rules do nothing; disabled-by-default behavior stays conservative. | yes |
| Use a global fallback threshold | Convenient, but could affect every tenant. | |
| Use tenant-only fallback | Less broad than global, but still wider than selected policy. | |
| Other | Freeform fallback behavior. | |

**User's choice:** No promotion.
**Notes:** No matching SLA rule means the task keeps normal scheduler/FIFO order.

| Option | Description | Selected |
|--------|-------------|----------|
| Fail validation when promotion is enabled | Bad enabled rules are loud; disabled configs remain harmless. | yes |
| Skip invalid rules and keep serving | Forgiving, but easy to miss broken SLA coverage. | |
| Disable all promotion on any invalid rule | Safe, but one bad rule disables unrelated tenants. | |
| Other | Freeform validation behavior. | |

**User's choice:** Fail validation when promotion is enabled.
**Notes:** Planning should use existing scheduler config validation patterns.

---

## Queue Promotion Mechanics

| Option | Description | Selected |
|--------|-------------|----------|
| Update the existing queue score | Smallest change; works with Redis ZSET and memory heap semantics. | yes |
| Add a dedicated Promote queue API | Clearer intent, but more code across queue backends. | |
| Pop and reinsert promoted tasks | Explicit, but race-prone. | |
| Other | Freeform queue behavior. | |

**User's choice:** Update the existing queue score.
**Notes:** Reuse Redis `ZAdd` and memory duplicate `Push`/`heap.Fix`.

| Option | Description | Selected |
|--------|-------------|----------|
| Bounded pre-pop threshold check | No continuous dynamic sorting; only eligible SLA-expired tasks get a one-time score update before selection. | yes |
| Enqueue-only static score | Preserves strict no-reordering, but weakens SLA promotion. | |
| Periodic promotion scan | More complete, but reintroduces dynamic queue work. | |
| Other | User asked what "evaluate promotion" meant. | |

**User's choice:** Bounded pre-pop threshold check.
**Notes:** User recalled the prior no-dynamic-re-ranking decision. Clarified that this is a narrow threshold-triggered exception, not continuous queue sorting.

| Option | Description | Selected |
|--------|-------------|----------|
| Bounded candidate window | Keeps promotion cheap and predictable. | yes |
| Entire queue scan | More complete, but violates simplicity at scale. | |
| Only current min item | Cheapest, but can miss another eligible task nearby. | |
| Other | Freeform scan boundary. | |

**User's choice:** Bounded candidate window.
**Notes:** Do not scan the whole queue by default.

| Option | Description | Selected |
|--------|-------------|----------|
| Pop normally | Promotion is opportunistic; normal order remains authoritative. | yes |
| Expand the window once | Better discovery, but adds another tuning knob. | |
| Skip popping and retry later | Can stall throughput. | |
| Other | Freeform fallback. | |

**User's choice:** Pop normally.
**Notes:** No eligible task in the window means normal scheduler/FIFO behavior continues.

---

## Priority And Quota Ceiling

| Option | Description | Selected |
|--------|-------------|----------|
| No, reorder only within resolved priority | Safest; respects existing priority resolver and high-priority quota. | yes |
| Yes, but only up to tenant max-priority and quota | Makes SLA promotion a priority escalation path. | |
| Only low to normal, never normal to high | Adds special-case rules. | |
| Other | Freeform ceiling. | |

**User's choice:** No, reorder only within resolved priority.
**Notes:** SLA promotion does not move a task into a higher priority class.

| Option | Description | Selected |
|--------|-------------|----------|
| Do not promote beyond resolved priority | Record a sanitized block outcome; do not borrow quota. | yes |
| Promote only if high-priority quota is available | Uses existing quota logic, but turns SLA into escalation. | |
| Allow tenant-specific override | Flexible, but more config and misconfiguration risk. | |
| Other | Freeform quota behavior. | |

**User's choice:** Do not promote beyond resolved priority.
**Notes:** Record `blocked_by_priority_or_quota` when SLA cannot be met inside the resolved priority boundary.

| Option | Description | Selected |
|--------|-------------|----------|
| Never | Prompt text has zero priority influence. | yes |
| Only safe extracted request kind | Allowed as rule key, but not prompt urgency. | |
| Allow model-estimated complexity | Useful for scheduling, but too close to untrusted content influencing priority. | |
| Other | Freeform signal boundary. | |

**User's choice:** Never.
**Notes:** Promotion can use trusted config/headers and safe structured dimensions only.

---

## Audit And Metrics Evidence

| Option | Description | Selected |
|--------|-------------|----------|
| Aggregated metrics plus sanitized audit on promotion/block only | Enough operator evidence without per-check noise. | yes |
| Audit every eligibility check | Maximum traceability, but noisy and risky. | |
| Metrics only, no durable audit | Simpler, but weaker for explaining SLA behavior. | |
| Other | Freeform evidence level. | |

**User's choice:** Aggregated metrics plus sanitized audit on promotion/block only.
**Notes:** Do not write durable audit events for every eligibility check.

| Option | Description | Selected |
|--------|-------------|----------|
| Policy ID, tenant ID/class, model class, request kind, priority, outcome | Enough to operate; excludes sensitive content. | yes |
| Only task class and outcome | Safest cardinality, but weak tenant diagnosis. | |
| Include task/request ID in durable audit only | More traceability, but higher sensitivity/cardinality. | |
| Other | Freeform field set. | |

**User's choice:** Policy ID, tenant ID/class, model class, request kind, priority, outcome.
**Notes:** Exclude prompts, API keys, auth headers, provider payloads, embeddings, semantic-cache payloads, and raw task text.

| Option | Description | Selected |
|--------|-------------|----------|
| promoted, not_eligible, blocked_by_priority_or_quota, disabled, error | Enough to debug behavior without high-cardinality details. | yes |
| promoted vs not_promoted only | Simpler, but weak diagnostics. | |
| Include detailed block reasons per rule | Richer, but more label/cardinality risk. | |
| Other | Freeform outcome buckets. | |

**User's choice:** promoted, not_eligible, blocked_by_priority_or_quota, disabled, error.
**Notes:** Outcome buckets should remain low-cardinality.

## Agent's Discretion

- Exact config names, default bounded-window size, and metric names.

## Deferred Ideas

- None.
