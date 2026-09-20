## Scope Coverage

Phase 27 remains internal-only: it adds no external API, SDK, provider, deployment dependency, schema migration, or new endpoint.

| Work package | Priority | Primary scope | Requirement coverage | Validation owner |
| --- | --- | --- | --- | --- |
| 27-01 | P0 | Immutable terminal classification and once-only finalizer | TERM-01, TERM-02, TERM-03, TERM-08 | Gateway/error unit tests |
| 27-02 | P0 | Ordinary, buffered, and Fusion lifecycle closure | TERM-01, TERM-02, TERM-03, TERM-06, TERM-08 | Gateway service tests |
| 27-03 | P0 | Request cancellation and checked SSE emission | TERM-04, TERM-05, TERM-06 | Handler and gateway tests |
| 27-04 | P0 | Usage settlement plus persistence-failure observability | TERM-07, TERM-08 | Settlement tests with fakes |
| 27-05 | P0 | Full regression matrix and execution evidence | TERM-01 through TERM-08 | Unit, service, handler, integration gate |
| 27-06 | P1 | Fusion terminal consistency verification only | TERM-01, TERM-02, TERM-03, TERM-07, TERM-08 | Fusion terminal regressions |

**Ordering:** `27-01 -> 27-02 -> {27-03, 27-04} -> {27-05, 27-06}`. `27-05` is the independent P0 phase gate; `27-06` supplies P1 Fusion consistency evidence in parallel.

**Explicit exclusions:** Fusion aggregation, routing, provider-member execution, and Judge behavior are unchanged by `27-06`; no plan adds hot-path external I/O or a live-provider acceptance dependency.
