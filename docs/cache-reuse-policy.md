# Trusted answer reuse policy

`cache.use_cases[].reuse_mode` explicitly selects answer reuse for an existing trusted profile. Profile matching still requires the configured API-key identity, target model and exact system prompt. Client-supplied labels cannot grant eligibility.

| Mode | Behavior |
| --- | --- |
| Omitted or `disabled` | Bypass answer caching and call the primary provider. |
| `exact` | Reuse only the complete identical user question within the same opaque scope and lifetime. |
| `semantic` | Explicitly authorize the existing embedding/vector candidate lookup for that profile. |

Enabled cache configuration rejects unknown values. Overlapping API-key/model/system profiles are rejected so profile order cannot silently choose a policy. Existing profiles without a mode now bypass answer caching; enabling semantic reuse requires a deliberate policy choice.

Use `disabled` or `exact` for policies, prices, deadlines, negation and personal eligibility. Exact mode hashes the full question bytes and opaque scope with SHA-256; it does not trim whitespace, normalize case, or accept paraphrases. Stored answers remain in the relational cache repository and obey expiry, read/write bounds, and existing zero-usage hit behavior. Exact-only cache startup and requests do not require embedding or vector services. Existing cache bounds and TTL remain required.

The scope includes identity, use case, knowledge version, target model, fixed settings, embedding representation and reuse policy version. This change isolates prior cache scopes. Changing policy or knowledge version cannot read an old scope; restoring a prior version may read its still-valid entries, so knowledge-version rollback is an explicit data decision.

Tool requests, streaming, route overrides, administrators and unsupported message/settings shapes keep their existing cache bypass. Embedding memoization stores vectors only and never grants permission to reuse an answer. Semantic mode is experimental until a business-approved same-answer dataset establishes both safe reuse and useful recall. Open-ended chat is not an automatically safe cache domain.

Production cache remains disabled with an empty allowlist. This change supplies policy controls and isolated test evidence, not a production configuration.

## Accepted integration budget — 2026-10-08

The project owner accepted the following candidate budget for merging the
default-off implementation. Compare complete-response P95 with **each** matched
cache-off endpoint (before and after), using the same scheduled load:

| Scenario | Required bounds |
| --- | --- |
| Semantic miss and low-hit | P95 ≤ 1.25 × off **and** P95 increase ≤ 40 ms |
| Exact miss | P95 increase ≤ 10 ms |
| Semantic and exact hits | P95 ≤ 60 ms **and** P95 ≤ 0.5 × off |
| Application residual | P95 ≤ 10 ms and P99 ≤ 15 ms |

Residual is computed per request as handler time minus provider-completion and
cache-read time; it is not pure CPU time. Latest validation used 3 matched blocks,
36 windows and 3,600 successful requests at about 8 RPS, concurrency capped at 4,
with embedding memoization off. All nine scenario checks and paired-bootstrap
interval checks passed. Semantic miss/low-hit P95 was 119.954/117.114 ms; the
before/after ratios were 1.097/1.191 and 1.153/1.150.

The original 1.05 target still fails and remains recorded. Its failure has not
been proved to be a hardware physical limit. Integration acceptance does not
establish semantic answer quality, seed correctness, long-duration capacity,
empty-collection reclamation, or resolution of historical upstream EOFs.
Production activation requires a separate decision and evidence for those gates.
See the [current verification](../.planning/phases/29-semantic-cache-latency-hardening/29-VERIFICATION.md)
for source identities, raw evidence and the limits of this three-block sample.
