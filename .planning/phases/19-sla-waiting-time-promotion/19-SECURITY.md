---
phase: 19
slug: sla-waiting-time-promotion
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-05
---

# Phase 19 - Security

Per-phase security contract: threat register, accepted risks, and audit trail.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Operator config -> gateway runtime | SLA rule fields enter SchedulerConfig and are validated before use. | Trusted config: policy ID, tenant selector, model class, request kind, threshold, candidate window |
| Queue backend -> promotion logic | Redis or memory queue exposes bounded task IDs and scores for pre-pop inspection. | Task ID and queue score only |
| Auth context/request -> task snapshot | Promotion stores only trusted identity and safe structured feature fields. | Tenant ID/class and safe TaskFeature fields |
| Promotion decision -> metrics | Runtime promotion outcomes become operator-visible Prometheus labels. | Policy, tenant class, model class, request kind, priority, outcome |
| Promotion decision -> durable audit | Promoted and blocked outcomes become durable control-state audit events. | Policy ID, tenant ID/class, model class, request kind, priority, outcome |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-19-01-01 | Tampering | internal/config/config_validation.go | mitigate | Enabled SLA rules require policy ID, tenant selector, model class, request kind enum, positive threshold, and bounded candidate window. Covered by config validation tests. | closed |
| T-19-01-02 | Information Disclosure | internal/config/config.go | mitigate | SLA rule config contains only tenant ID/class, model class, request kind, threshold, and policy ID; no prompt, payload, API key, auth header, embedding, semantic-cache, or raw task fields were added. | closed |
| T-19-01-03 | Denial of Service | SchedulerConfig | mitigate | SLA promotion is disabled by default and uses a positive candidate-window field before runtime wiring consumes it. | closed |
| T-19-02-01 | Information Disclosure | ResultRegistry task snapshots | mitigate | `ResultRegistry` stores cloned `Task` snapshots containing tenant ID/class and safe `TaskFeature` fields only; tests assert prompt-like fields do not affect promotion. | closed |
| T-19-02-02 | Tampering | SLAPromoter score update | mitigate | Promotion reuses `QueueBackend.Push` score replacement, computes same-priority ordering with `math.Nextafter`, and blocks attempts that would cross higher-priority work. | closed |
| T-19-02-03 | Denial of Service | Queue.PeekMin | mitigate | Memory, Redis, and fallback queues inspect only the configured bounded window and treat limits below one as empty. | closed |
| T-19-02-04 | Elevation of Privilege | Priority handling | mitigate | Promotion matches only trusted tenant ID/class, model class, and request kind; it never reads prompt-derived urgency fields or reruns quota to borrow high priority. | closed |
| T-19-03-01 | Information Disclosure | metrics/audit/logs | mitigate | Metrics omit tenant ID and all sensitive payload fields; audit/log evidence is limited to the allowed evidence key set. | closed |
| T-19-03-02 | Tampering | audit metadata | mitigate | Durable audit metadata is passed through `controlstate.SafeAuditMetadata`, and tests assert the exact allowed key set. | closed |
| T-19-03-03 | Denial of Service | metrics cardinality | mitigate | Prometheus labels are bounded with allow-list/default helpers, and outcome values map to known buckets. | closed |
| T-19-03-04 | Repudiation | durable audit | mitigate | Promoted and blocked_by_priority_or_quota outcomes write durable audit events with policy and tenant evidence; disabled and not_eligible do not create noisy audit records. | closed |

## Accepted Risks Log

No accepted risks.

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-05 | 11 | 11 | 0 | Codex |

## Verification Evidence

- `go test -timeout 60s ./internal/config` - passed
- `go test -timeout 60s ./internal/scheduler ./internal/app` - passed
- `go test -timeout 60s ./internal/scheduler ./internal/observability ./internal/app` - passed
- `go test -timeout 60s ./...` - passed
- `go build ./...` - passed

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-05
