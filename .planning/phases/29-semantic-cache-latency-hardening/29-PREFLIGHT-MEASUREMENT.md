# Phase 29 isolated measurement preflight (2026-09-26)

Status: measurement only. No Wave 2 behavior or production cache configuration was changed. The figures below are **conditional**, not a production acceptance result: the test harness supplies Gemini embeddings through direct REST and maps `:` to `_` in Qdrant collection names. Current application wiring cannot do the former, and current collection names fail against this Qdrant instance.

Production cache-off and an empty production allowlist remain mandatory. They were not independently inspected on a production host; only the supplied isolated test host was accessed.

## Environment and workload

- Code revision: `c43a4a6876d6a32419f2255bbcd8ae76f4907b0e`. Gateway and HTTP client ran in-process on Windows x64 (16 logical CPUs, 31.25 GiB RAM, 13.62 GiB free at inventory). This is not a gateway deployment on the test host.
- Vector backend: Qdrant 1.18.2 in `veloxmesh-test-qdrant` on `192.168.234.129` (Linux host, 4 logical CPUs, 3.3 GiB RAM, 2.6 GiB available, 3.8 GiB swap). gRPC traffic used a temporary SSH tunnel. Cache repository and API-key store were isolated in-memory SQLite per run.
- Main model: `oc/big-pickle` from local `SANS_PRIMARY_DEFAULT_MODEL`. Embedding model: `gemini/gemini-embedding-2-preview` from local `GEM_EMBEDDING`; a real response established 3072 dimensions. No key values or response text are in these artifacts.
- Each run created a new non-admin API key in its test database and read its actual ID into the `phase29-static-faq` allowlist. The fixed request set in `tests/integration/phase29_measure_test.go` is 20 distinct static FAQ questions plus one exact repeat (21 requests). Fixed system prompt, `faq-v1`, one user turn, text only, non-streaming, `temperature=0`, `max_tokens=256`.
- Warmup per run: one distinct sentinel full gateway request, excluded from measurements. A real Gemini dimension probe also preceded each run and was excluded. Each measured client timer started before HTTP send and ended after `io.ReadAll(response.Body)` reached EOF. No response-header timer was used.
- Offered load: 1 or 2 requests/second, at most 4 in-flight requests, identical request sequence for cache-off and cache-on. The low-hit configuration used cosine threshold 0.99999; actual hits were 1/21 per run (4.76%). New test identities isolated each run's cache scope.
- Percentiles are nearest-rank over pooled raw samples from two runs per condition. Actual RPS = completed responses / sum of per-run wall times, including scheduled pacing and tail completion. `at_ms` is integer-millisecond completion offset, so concurrency reconstructed from raw timestamps needs a 1 ms boundary tolerance; the sender also enforces a hard cap of 4.

## Full-response results

| Target RPS | Mode | Runs | Responses | Actual RPS (run values) | Peak in flight (runs) | Hits | Errors | P50 ms | P95 ms | P99 ms |
| ---: | --- | ---: | ---: | --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | Off | 2 | 42 | 0.942 (0.898, 0.989) | 4, 3 | 0 | 0 | 1318.04 | 2718.40 | 4486.91 |
| 1 | Low hit | 2 | 42 | 1.016 (1.006, 1.026) | 3, 3 | 2 | 0 | 1779.82 | 2530.94 | 4011.92 |
| 2 | Off | 2 | 42 | 1.565 (1.646, 1.492) | 4, 4 | 0 | 0 | 1194.18 | 3638.52 | 7570.25 |
| 2 | Low hit | 2 | 42 | 1.595 (1.530, 1.666) | 4, 4 | 2 | 0 | 1768.59 | 3704.29 | 4794.88 |

Four concurrent full requests completed without errors in both 2 RPS repeats, but the system did **not** sustain 2 RPS at that cap. The only repeatedly attained offered rate was 1 RPS. Main-model tail variance dominates the 21-request runs; lower cache-on P99 must not be attributed to a 4.76% hit rate.

## Cache-on stage latency

Each cell is `P95 / P99 ms (samples)`. Embedding samples combine read and write calls; the current service makes one lookup embedding per request and another embedding per miss. Repository read combines candidate-list and candidate-fetch calls. All listed stage operations succeeded in valid runs.

| Stage | 1 RPS, 2 runs | 2 RPS, 2 runs |
| --- | ---: | ---: |
| Embedding | 449.06 / 533.24 (82) | 490.91 / 1731.83 (82) |
| Vector search | 4.20 / 7.15 (42) | 3.64 / 4.42 (42) |
| Repository read | 1.12 / 2.04 (42) | 1.13 / 1.65 (42) |
| Repository write | 0.50 / 0.55 (40) | 0.50 / 0.50 (40) |
| Vector insert | 6.36 / 8.25 (40) | 5.72 / 6.26 (40) |

At 2 RPS, 1/82 embedding operations exceeded 750 ms (1731.83 ms); none did at 1 RPS. The harness did not distinguish whether that outlier was a lookup or a store. Thus any read cutoff estimated from this combined distribution is provisional.

## Candidate bounds requiring approval

| Parameter | Candidate | Evidence and consequence |
| --- | ---: | --- |
| Cache-read deadline | **750 ms** | Above 1 RPS combined embedding P99 (533.24 ms) plus observed vector/repository tails; at 2 RPS at least one embedding would exceed it. A timed-out read must fall through to the primary model, not fail the request. |
| Max concurrent cache reads | **4** | Four full requests completed without errors in both 2 RPS repeats; this is an upper test boundary, not proof of 2 RPS sustainable throughput. |
| Async write workers | **2** | At 2 RPS, actual misses were about 1.5-1.6/s. Two workers provide room above the 0.49 s embedding P95 plus sub-7 ms vector insert, subject to a longer-tail recheck. |
| Write queue capacity | **32** | Bounded short-burst buffer; no unbounded memory or request blocking. This capacity itself was not load-tested because Wave 2 async writes do not exist yet. |
| Shutdown drain grace | **15 s** | Allows roughly 30 sequential P95-sized writes across two workers; remaining items after the deadline are dropped and counted. This is not a full-queue P99 drain guarantee. |

Burst policy proposal: enqueue without waiting; if the queue is full or shutdown has begun, drop the newest write, increment a privacy-safe drop counter, and continue the foreground response. Drain accepted writes for at most 15 s on shutdown, then count and discard the remainder. The user is the parameter approval owner for this checkpoint; the production FAQ publisher and atomic version-switch owner remain unassigned. **Do not enable a production allowlist before both ownership and version-switch procedure are settled.**

## Blockers and raw data

1. Native `internal/providers/gemini` implements chat completions but not `providers.EmbedAdapter`. The test-only adapter used the real Gemini REST endpoint and model ID; production application startup with this Gemini provider would not build a semantic-cache service.
2. Current `vectorCollection` produces names like `semantic_cache:<digest>`; Qdrant rejected create-collection with `collection name cannot contain \":\" char`. The test-only vector wrapper replaced the colon with an underscore. The unswizzled cache path cannot be used to approve these parameters.
3. The first cache-off request set completed but its raw output path was wrong, so it is excluded. An earlier cache-off JSONL lacks usage settlement and is excluded. An earlier cache-on JSONL captured failed repository/vector operations and is excluded. These diagnostic files are retained. The live `faq-v1` -> `faq-v2` gateway switch was not measured here; only the existing isolated scope test covers that change.
4. The earlier GPT embedding configuration returned `model_not_found`; following the user correction, only `gemini/gemini-embedding-2-preview` completed a real embedding and full-gateway measurement. The original two-embedding-model acceptance item remains open until another valid provider/model pair is supplied.

Valid raw JSONL (all samples, durations, statuses, cache headers, no request/response bodies):

- `measurements/gemini-off-mapped-1.jsonl`, `measurements/gemini-off-mapped-2.jsonl`
- `measurements/gemini-low-hit-mapped-1.jsonl`, `measurements/gemini-low-hit-mapped-2.jsonl`
- `measurements/gemini-off-2rps.jsonl`, `measurements/gemini-off-2rps-2.jsonl`
- `measurements/gemini-low-hit-2rps.jsonl`, `measurements/gemini-low-hit-2rps-2.jsonl`

Excluded diagnostic raw files: `measurements/gemini-off-1.jsonl`, `measurements/gemini-low-hit-1.jsonl`. The 8 valid files contain 168 complete-response samples. No production deployment or cache enablement occurred.

Verification: `go vet -tags phase29preflight ./tests/integration` and focused `go test -timeout 60s -run '^TestSemanticCache_' ./tests/integration` passed; `go test -timeout 60s ./internal/cache` passed. The full `./tests/integration` package timed out at the mandated 60 seconds with many Redis connection waits because Redis was not started for this preflight. The temporary SSH tunnel was closed and `veloxmesh-test-qdrant` was returned to its initial stopped state.
