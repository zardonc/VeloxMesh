---
phase: 04-11
slug: streaming-rate-limits-cache-and-cost
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-22
---

# Phase 04-11 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| prompt-derived text -> embeddings provider | User input is sent for embedding only when semantic cache is explicitly enabled. | text chunks / normalized input |
| cache storage -> future responses | Stored response JSON may be returned on semantic match. | response payloads |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-04-11-01 | Information Disclosure | semantic cache storage | mitigate | Store embeddings, safe scope, and response only; never raw prompts, tokens, or provider secrets. | closed |
| T-04-11-02 | Spoofing | cache scope | mitigate | Scope entries by safe API-key identity and model to prevent cross-key hits. | closed |
| T-04-11-03 | Tampering | embedding vectors | mitigate | Repository tests verify encoded vector round-trip and expiry/enabled flags. | closed |
| T-04-11-SC | Tampering | package installs | accept | No package-manager dependencies are added. | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| R-04-11-01 | T-04-11-SC | Expected phase behavior, no external packages needed. | Agent | 2026-06-22 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-22 | 4 | 4 | 0 | gsd-secure-phase |

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-22
