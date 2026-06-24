---
phase: 04-02
slug: streaming-rate-limits-cache-and-cost
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-21
---

# Phase 04-02 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| durable repositories -> runtime snapshot | Database state becomes live data-plane provider state. | internal config state |
| Admin reload -> data-plane routing | Reload changes active provider, routing, fallback, and probing behavior. | internal state |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-04-02-01 | Tampering | runtime activation | mitigate | Publish snapshots only after durable provider and routing validation succeeds per D-10 and D-11. | closed |
| T-04-02-02 | Information Disclosure | readyz/prober errors | mitigate | Expose provider IDs and safe statuses only; never API keys, auth headers, raw prompts, or upstream payloads per D-42. | closed |
| T-04-02-03 | Denial of Service | missing config | mitigate | Durable mode fails closed with actionable not-ready/no-provider errors per D-08. | closed |
| T-04-02-SC | Tampering | package installs | accept | No package-manager dependencies are added. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-02-01 | T-04-02-SC | No third-party dependencies are added, avoiding supply-chain tampering. | System | 2026-06-21 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-21 | 4 | 4 | 0 | gsd-security-auditor |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-21
