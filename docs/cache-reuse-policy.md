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
