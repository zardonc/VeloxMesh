---
phase: 2.7
slug: health-aware-routing
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-16
---

# Phase 2.7 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Client -> Gateway (/readyz) | Operational readiness endpoint | Internal provider capabilities state |
| Registry -> Gateway (/v1/models) | Model listing | None (static listing) |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-02-07-01 | Information Disclosure | internal/http/handlers/health.go | mitigate | Manually extract only safe properties (type, streaming, tools) into map; omit sensitive fields. | closed |
| T-02-07-02 | Tampering | internal/providers/registry.go | mitigate | Registry returns `.Clone()` of capabilities to prevent accidental mutation. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

No accepted risks.

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-16 | 2 | 2 | 0 | gsd-security-auditor |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-16
