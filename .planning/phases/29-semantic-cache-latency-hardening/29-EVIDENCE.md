# Phase 29 evidence — 2026-10-01

Phase 29 is **not complete**. Wave 1 and offline Wave 2 are implemented. The requested local embedding model now passes the corrected real gateway semantic/version flow, embedding fault fallback and real queue burst; full backend tests pass. A real false hit was repaired by embedding only the isolated user question and versioning that representation. Corrected low-hit P95 ratios still fail at **1.143 / 1.511**; sustainable load, final parameters and the second real-model gate remain open. Production cache remains disabled and its allowlist remains empty; no production configuration was changed.

Latest local-model method, code/binary provenance, all operation quantiles, raw sample counts and concrete review candidates: [29-LOCAL-MODEL-MEASUREMENT.md](29-LOCAL-MODEL-MEASUREMENT.md). The sections below preserve the earlier SANS diagnostic history and are not the latest local-model measurements.

## Real application repairs

Committed implementation: `1d866f1` (native Gemini embedding) and `77124712dfba563a9f745eb2efedcf601abf9f3d` (offline Wave 2, application wiring and measurement tools). These commits are the starting point for the next approved live rerun, not the revision used by the earlier diagnostic binary.

- Gemini now implements the existing `providers.EmbedAdapter` through the installed SDK. Batch ordering, malformed responses, invalid input and upstream errors have regression coverage. The former preflight-only REST implementation has been removed. A real call to the configured Gemini model returned 3072 dimensions through the production adapter.
- Qdrant collections now use `semantic_cache_<opaque digest>` without the forbidden colon; the measurement wrapper no longer changes collection names.
- Durable providers are activated before semantic cache construction. A red startup test previously produced `provider registry not ready`; it now passes. Embedding provider ID joins model and dimension in the authorization/version identity. Trusted profile slices and queued usage identities are copied.

## Provenance and method

The pre-Wave-2 baseline ran on **192.168.234.129**, not a manually assembled gateway: `App.New()` → authenticated HTTP chat handler → normal gateway/settlement → disk SQLite WAL repository → configured Qdrant gRPC adapter. The HTTP client and gateway ran on the same test host using loopback, as did Redis and Qdrant. Upstream primary and embedding traffic used the configured SANS HTTPS endpoint. This topology has not been confirmed as the final production deployment topology.

Machine: Linux `7.0.0-27-generic`, x86_64, 4 CPUs, 3350 MiB RAM, 3861 MiB swap; no swap in use at inspection. Dependency readiness: Redis PONG plus Search/JSON modules, PostgreSQL `pg_isready`, Qdrant 1.18.2. Post-run container RSS was approximately Redis 101 MiB, Qdrant 492 MiB, PostgreSQL 88 MiB. These are snapshots, not burst peak-memory measurements.

Dependency image identities:

- Redis Stack: `sha256:798ab84d9f266936b034ab11c4d04a2b8e4b441884c5aa7d17ac951eefdf742a`
- Qdrant: `sha256:75eab8c4ba42096724fdcfde8b4de0b5713d529dde32f285a1f86fdcb2c9e50c`
- PostgreSQL/pgvector: `sha256:ccc6e83d6e35e931dc7c5def2022729d5a6c370318d099181995567ff1fb4d6b`

Baseline code provenance: parent `3aa4bfd1de5bc1dfccd9639f97c2501b48703e90` plus then-uncommitted native-adapter, collection, startup-order and measurement-instrumentation repairs. The compiled Linux test binary SHA-256 is `53403f0028008a775ca96c9c946cdba0b4cd5c9b399f8a146640673191bdc076`; it was retained in the private remote test directory. **This is not an immutable source commit baseline**, and does not include the later Wave 2 behavior. A release-grade rerun must build from the recorded committed source revision.

Primary: configured `SANS_PRIMARY_DEFAULT_MODEL` (`oc/big-pickle`), fixed system prompt, `temperature=0`, `max_tokens=256`, single-turn plain text, nonstreaming. Corpus is the numbered-plan trial FAQ defined in `internal/app/phase29_live_test.go`: plan N has an N-day trial; requests 1–64 ask the corresponding plan. Every 21st request repeats the preceding plan; the maximum intended hit rate is 3/64 = 4.69%. The positive/negative semantic-quality corpus is separate; this exact-repeat load corpus does not prove paraphrase quality.

Each run creates a non-admin API key in the isolated SQLite database and reads its actual ID back by hash before constructing the allowlist. The fixed `phase29-static-faq` use case and `faq-v1` are server fixtures. Models and dimensions come from configuration and a real dimension probe. Credentials are supplied through stdin and ephemeral test configuration, never through tracked files or log output.

Warm-up: a real gateway request for sentinel plan 999, including synchronous store in this pre-Wave-2 binary; warm-up samples are excluded. Requested load: 2 RPS, 500 ms scheduling interval, maximum four HTTP requests in flight, 64 requests per condition. Client timing ends after the complete body is consumed, not at headers. Actual RPS includes the drain of foreground requests and is reported separately from the requested rate. Three paired rounds were scheduled, but only round 1 completed; round 2 stopped at a failed embedding dimension probe. No successful repeats or longer steady-state capacity are claimed.

## Full-response results — diagnostic, not approval

Successful complete-response quantiles use nearest rank. Failed attempts are excluded from these quantiles and retained separately in raw data and `summary.json`; failures are not silently converted into complete responses. Every condition's P99 is still essentially its maximum because sample counts remain small.

| Model / condition | Successful / attempted | Actual attempt RPS | Observed peak / average concurrency | Hits | P50 ms | P95 ms | P99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Liquid / off | 64 / 64 | 1.988 | 4 / 2.988 | 0 | 1405.563 | 2427.351 | 3057.152 |
| Liquid / low hit | 63 / 64 | 1.728 | 4 / 3.614 | 0 | 1809.452 | 3415.618 | 4773.275 |
| NVIDIA / off | 64 / 64 | 1.556 | 4 / 2.475 | 0 | 1401.048 | 2358.848 | 9641.379 |
| NVIDIA / low hit | 62 / 64 | 1.663 | 4 / 3.471 | 0 | 1679.129 | 3317.548 | 4313.887 |

Observed P95 ratios are approximately **1.407** and **1.406**, both above `1.05`. P50 increases are approximately **28.7%** and **19.8%**. Actual delivered load differs despite identical scheduling/concurrency settings, and the provider failed heavily, so these ratios are diagnostic rather than a valid stable-load release comparison. Three low-hit client attempts timed out at approximately 10 seconds. Their samples are retained, including the attempt-level P99 around 10 seconds.

Stable concurrency and sustainable embedding capacity are **not established**. Observed concurrency 4 is an enforced client ceiling, not proven provider capacity. A low hit rate caused by failed lookups is not evidence of healthy cache operation.

## Operation measurements

Successful operation samples only; raw data and analysis also retain failed-operation counts and durations. Lookup and store embedding calls are separate. `store_total` includes embedding, repository storage and vector insertion. All numbers are milliseconds.

| Operation | Liquid n | Liquid P95 / P99 | NVIDIA n | NVIDIA P95 / P99 |
| --- | ---: | ---: | ---: | ---: |
| Lookup embedding | 13 | 604.416 / 604.416 | 3 | 627.520 / 627.520 |
| Store embedding | 20 | 1119.271 / 2183.455 | 0 | unavailable |
| Vector retrieval | 10 | 2.163 / 2.163 | 3 | 3.779 / 3.779 |
| Repository read | 10 | 0.941 / 0.941 | 3 | 0.641 / 0.641 |
| Repository write | 20 | 0.455 / 0.793 | 0 | unavailable |
| Vector insertion | 20 | 9.114 / 26.612 | 0 | unavailable |
| Complete store | 20 | 1122.403 / 2186.063 | 0 | unavailable |

Liquid lookup embedding failed 51/64 times and store embedding 43/63 times; NVIDIA lookup failed 61/64 times and store embedding 62/62 times. Three Liquid vector searches failed while its collection was not yet populated. The old synchronous path returned ordinary primary responses through most cache failures, but the remaining successful samples are insufficient to derive a healthy tail distribution.

## Provider diagnosis and model verification

Initial production-adapter smoke calls succeeded:

| Provider / configured model | Observed dimension | Initial single-call ms |
| --- | ---: | ---: |
| sans-primary / `openrouter/liquid/lfm-2.5-embedding-350m:free` | 1024 | 1269 |
| sans-primary / `openrouter/nvidia/nemotron-3-embed-1b:free` | 2048 | 478 |
| native Gemini / `gemini/gemini-embedding-2-preview` | 3072 | 376 |

After sustained load, both SANS models repeatedly returned `provider_rate_limit`. A separate sanitized HTTP diagnostic confirmed upstream HTTP **429**, `Retry-After: 300`, and **`free-models-per-day`**. The upstream suggested adding credits to raise the daily allowance. No credits were purchased and no account settings were changed. The retry header is not proof that the exhausted daily allowance will recover in five minutes. These models are valid configurations; the remaining issue is external test quota, not a model-name typo.

## Explicit experimental bounds — not final approved values

| Parameter | Isolation experiment | Basis / limitation |
| --- | ---: | --- |
| Total read deadline | **100 ms** | Below roughly 118–121 ms (5% of these off P95 values); this arithmetic is only a starting reference, not an additive percentile proof. Successful embedding calls are usually slower, so this profile may sacrifice most real hits. Must tune jointly with hit quality and full-response latency. |
| Concurrent reads | **4** | User-authorized experimental ceiling; a deterministic burst verifies it is enforced. Sustainable provider capacity is unknown. |
| Write workers | **2** | User-authorized starting point; combined blocking-embedding test verifies at most four reads plus two writes. |
| Write queue capacity | **32** | User-authorized starting point; full queue drops the newest candidate without waiting. Production burst/RSS validation is pending. |
| Independent write timeout | **2 s** | Successful Liquid store P95 is about 1.12 s, but its observed P99 exceeds 2 s. Slow writes may deliberately be discarded; this is not a tail-completion guarantee. |
| Shutdown grace | **1 s** | Conservative finite offline experiment; accepted queued writes need not all drain. Real deployment stop deadline remains unconfirmed; 15 s was not silently retained as an approved value. |

These values are supplied explicitly through test configuration or `PHASE29_READ_TIMEOUT`, `PHASE29_READ_CONCURRENCY`, `PHASE29_WRITE_TIMEOUT`, `PHASE29_WRITE_WORKERS`, `PHASE29_QUEUE_CAPACITY`, `PHASE29_SHUTDOWN_GRACE`. Enabled profiles missing any positive bound fail configuration validation. No enabled production defaults were introduced.

Burst/close policy: drop newest work on a full queue; reject writes once closing begins; finish accepted work only within the configured grace, then cancel in-flight worker contexts and discard remaining queued work. Bounded reason counters distinguish `queue_full`, `closed`, `shutdown_drop`, `shutdown_cancelled`, timeout and dependency errors. Prometheus label values are enumerated; no key IDs, prompts, answers, versions or credentials are labels. Errors remain returned with their underlying causes; synchronous fault logging was kept out of the optional foreground cache path to avoid adding blocking log I/O to its deadline.

Approval owner for isolation numeric parameters and final evidence: **the requesting user**. Production FAQ publisher and atomic `faq-v1` → `faq-v2` cutover owner: **unassigned**. Neither production allowlist nor production cache may be enabled before that separate ownership/procedure and release approval exist.

## Offline validation and remaining gates

- Red-to-green tests cover delayed/non-cooperative embedding, admission saturation, nil/nonfinite/zero/wrong-dimension embeddings, dependency errors, malformed vector results, no scan after a vector fault, full/closed queues, write cancellation, bounded shutdown, immutable queued and configured identities, provider/version/model isolation and TTL.
- HTTP response-before-write regression fails with the old synchronous `Store` call and passes with nonblocking `Enqueue`; persistence is observed separately before asserting a hit. The primary is called once, and cache hits keep their existing zero-usage behavior.
- Labeled offline oracle: one positive paraphrase hits; two similar-but-different-answer questions, three cross-key/version/model cases and expiry produce no wrong hits. These hand-assigned vectors do not establish real-model semantic quality.
- `go test -timeout 60s ./...` passes with isolated Redis/Qdrant/PostgreSQL available. The previously failing Redis integration package completed in about 16 seconds. One earlier full run exposed a fragile wall-clock assertion under parallel compilation; it was replaced by a deterministic non-cooperative-dependency case, and the full suite was rerun.
- Focused `go test -timeout 60s -race ... -run 'TestSemanticCache|TestAdapterEmbedding'` and `go vet -tags phase29preflight` pass. No streaming/tool protocol behavior was changed.

Still required for `29-03`: restore sustainable test quota; run both real models through positive/negative gateway cases, async persistence, version/model isolation and fault bypass; repeat longer steady-state measurements from a committed build; verify queue backlog/RSS and shutdown against the actual deployment stop deadline; tune values from those results; prove matched-load low-hit full-response P95 ratio ≤ `1.05`; obtain final human review. Production publisher ownership blocks production enablement, not offline development.

## Retained artifacts and reproduction

Raw diagnostic files in `measurements/app-baseline-20261001/`: four nonempty round-1 JSONL files contain **256 client attempts**, with operation records. `baseline-m0-off-r2.jsonl` is empty because the next dimension probe failed and is explicitly excluded. `summary.json` retains exact operation counts, failures and both success-only and attempt-level quantiles. Earlier conditional preflight artifacts remain unchanged.

Cleanup completed: all three isolated containers were returned to their original stopped state, their volumes were retained, the task's SSH forwarding process was terminated, and temporary credential-reading transport helpers were removed. Remote redacted logs/results and the diagnostic binary remain available; no credentials were retained in them.

Recompute without secrets:

```bash
node scripts/phase29-analyze.mjs .planning/phases/29-semantic-cache-latency-hardening/measurements/app-baseline-20261001
```

The opt-in real application runner is `TestPhase29LiveMeasure` in `internal/app/phase29_live_test.go`, built with `-tags phase29preflight`. Compile for the test host with `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c -tags phase29preflight ./internal/app`. Supply selected model, bounds, `PHASE29_MODE`, interval, count and output path explicitly; pass the required provider credentials as a JSON environment map on stdin. Run each bounded slice with `-test.run TestPhase29LiveMeasure -test.timeout 60s`. The updated runner drains bounded writes before saving operation samples and records foreground duration separately from close/drain duration. Select each SANS model from `.env.local`, probe dimensions again, alternate off/on order across repeats, and do not approve a run with quota faults, mismatched achieved load or missing samples.
