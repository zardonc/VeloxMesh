---
phase: 04-10
slug: streaming-rate-limits-cache-and-cost
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-22
---

# Phase 04-10 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| provider response -> settlement | Provider usage metadata becomes a credit charge. | provider usage data |
| settlement -> API key balance | Gateway mutates durable credit balances. | credit charges |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-04-10-01 | Tampering | concurrent balance deduction | mitigate | Full-mode PostgreSQL settlement uses transaction plus `SELECT ... FOR UPDATE` row lock. | closed |
| T-04-10-02 | Repudiation | settlement records | mitigate | Persist status, timestamps, provider, model, tokens, rates, credits, and deficit per D-35/D-38. | closed |
| T-04-10-03 | Information Disclosure | usage records | mitigate | Store usage counts and safe IDs only; never prompts or provider payloads per D-42. | closed |
| T-04-10-04 | Denial of Service | missing usage/rate | mitigate | Record unsettled status without arbitrary deduction or panics per D-37. | closed |
| T-04-10-SC | Tampering | package installs | accept | No package-manager dependencies are added. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-10-01 | T-04-10-SC | Expected phase behavior, no external packages needed. | Agent | 2026-06-22 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-22 | 5 | 5 | 0 | gsd-secure-phase |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-22
