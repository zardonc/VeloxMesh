---
phase: 04-01
slug: streaming-rate-limits-cache-and-cost
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-21
---

# Phase 04-01 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| durable storage -> routing config | Database records become provider selection and fallback state. | internal config state |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-04-01-01 | Tampering | routing config | mitigate | Validate strategy, default provider, fallback flag, and max_attempts against active providers before activation per D-10. | closed |
| T-04-01-02 | Denial of Service | missing durable config | mitigate | Return typed missing-config errors instead of silently using hard-coded defaults per D-08. | closed |
| T-04-01-SC | Tampering | package installs | accept | No package-manager dependencies are added. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-01-01 | T-04-01-SC | No third-party dependencies are added, avoiding supply-chain tampering. | System | 2026-06-21 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-21 | 3 | 3 | 0 | gsd-security-auditor |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-21
