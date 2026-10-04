# Semantic cache embedding representation and short-term reuse

The cache remains opt-in and restricted by its trusted use-case allowlist and explicit [answer reuse policy](cache-reuse-policy.md). These options do not enable caching or change its similarity threshold. Embedding options apply to `semantic` profiles; `exact` profiles use the relational answer cache without embedding.

| Cache field | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `embedding_input_prefix` | `SEMANTIC_CACHE_EMBEDDING_INPUT_PREFIX` | empty | Explicit text prepended to eligible user questions; at most 64 UTF-8 bytes, no NUL. |
| `embedding_memo_capacity` | `SEMANTIC_CACHE_EMBEDDING_MEMO_CAPACITY` | 0 | Maximum in-memory vectors per service, 0 disables reuse, maximum 4096. |
| `embedding_memo_ttl` | `SEMANTIC_CACHE_EMBEDDING_MEMO_TTL` | empty | Required when capacity is positive; positive duration at most one hour. |

A nonempty prefix changes the opaque cache scope, including an input representation version. Returning to an empty prefix restores the existing unprefixed scope. A model service that already inserts a prefix may require no gateway prefix: verify the effective model input first. Changing the model file or pooling behind an unchanged provider/model name also requires a new model identifier or knowledge version; the gateway cannot discover that change automatically.

Memoization stores only successful, validated embedding vectors, keyed by a hash of account/use-case scope, target model and exact question. Each service has separate storage, and outward vectors are copied. TTL does not extend on reads; eviction uses LRU order. Failed or cancelled calls are not retained. Concurrent cold requests may each call the real embedding provider; there is no shared cancellation or single-flight wait. Capacity bounds memory; expired vectors are rejected on reads and storage is released with the service.

Repeated inputs can avoid embedding I/O within this lifetime. New questions and paraphrases still call the real provider, so memoization does not establish a faster cold-miss SLO. Answer lookup still queries the vector and relational stores, validates expiry and billing, and obeys the existing read deadline. A memo hit never authorizes an answer-cache hit by itself. `embedding_memo/hit` and `embedding_memo/miss` outcomes distinguish actual vector reuse.

Do not lower similarity thresholds globally to compensate for latency. Validate frozen candidates on independent same-answer and different-answer questions, including numbers and negation. A prefix and threshold that work on one embedding model are not a policy for all models. Test budgets and production SLOs are separate; quota-exhausted providers must be recorded as skipped rather than passing.
