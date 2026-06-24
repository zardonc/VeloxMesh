---
phase: 04-12
slug: streaming-rate-limits-cache-and-cost
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-22
---

# Phase 04-12 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| user request -> embeddings | Request text is embedded only when semantic cache is enabled. | request text for cosine similarity |
| cache hit -> client response | Stored response may bypass provider call. | cache response payloads |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-04-12-01 | Information Disclosure | cache hit scope | mitigate | Scope cache by safe API-key identity and model; no cross-key hits. | closed |
| T-04-12-02 | Tampering | cached response | mitigate | Store only after successful provider response and settlement path; include expiry/enabled checks. | closed |
| T-04-12-03 | Repudiation | cache hits | mitigate | Record observable cache hit/miss metadata without guessing provider token usage. | closed |
| T-04-12-04 | Denial of Service | similarity search | mitigate | Bound candidate count and use in-process cosine over scoped candidates. | closed |
| T-04-12-SC | Tampering | package installs | accept | No package-manager dependencies are added. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-12-01 | T-04-12-SC | Expected phase behavior, no external packages needed. | Agent | 2026-06-22 |

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
