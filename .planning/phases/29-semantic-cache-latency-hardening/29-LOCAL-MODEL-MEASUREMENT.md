# Phase 29 — local embedding verification, 2026-10-01

The requested local model works through the **real application, production adapter, authenticated gateway and Qdrant**. A real negative case exposed and drove a correctness fix. The corrected local-model semantic flow, embedding fault fallback, queue burst and full backend tests pass. **Phase 29 remains incomplete:** the low-hit latency gate fails, sustainable complete-response capacity is not established, and this run verifies one distinct embedding model. Production cache remains off and the production allowlist remains empty. No production settings were edited.

## Environment and provenance

The user-selected embedding model is `text-embedding-embedder_collection`. A real response verified **768 finite dimensions**; dimension is then supplied through test configuration, never hardcoded in the implementation. The first local HTTP probe took approximately 89 ms. The configured primary remains `oc/big-pickle`, selected from the existing environment configuration; `temperature=0`, `max_tokens=256`, fixed FAQ system prompt, single-turn text and nonstreaming responses throughout each experiment.

Topology: HTTP client and `App.New()` gateway run on test host `192.168.234.129`; disk SQLite WAL, Redis and Qdrant use that host's loopback. The existing OpenAI-compatible embedding adapter calls a loopback SSH reverse tunnel to the user's Windows service at `127.0.0.1:1234`. Primary traffic still uses the existing configured HTTPS provider. This is a temporary two-machine test topology, not a claim about the eventual production network path.

Resource snapshots:

| Component | Observed resources |
| --- | --- |
| Test host | Linux 7.0.0-27-generic, x86_64, 4 CPUs, **2534 MiB RAM**, 3861 MiB swap, no swap in use at initial inspection |
| Embedding host | AMD Ryzen 9 8945HS, 16 logical CPUs, 32001 MiB RAM; initial free RAM about 11760 MiB |
| Local GPU | NVIDIA GeForce RTX 4060 Laptop GPU, 8188 MiB VRAM; snapshot used 561 MiB |
| Test dependencies | Existing Redis Stack, Qdrant 1.18.2 and PostgreSQL/pgvector; Redis PONG and PostgreSQL readiness confirmed |
| Later dependency memory | Redis 100.7 MiB, Qdrant 914.9 MiB, PostgreSQL 83.96 MiB; container memory limit 2.475 GiB |

These are snapshots, not peak RSS. Qdrant retains historical isolated collections, so its later memory value includes earlier runs and is not the allocation cost of a single burst. Volumes and evidence are preserved.

| Artifact | Committed source | Linux binary SHA-256 |
| --- | --- | --- |
| Initial local baseline, before semantic-input fix | `cf6ae510d3d7e03773112ff41c9810e0e2a74526` | `b2f6dd1d53e9e40a31128f285874eb8ad9c516f3b630b753bddd7aa1c3b6455c` |
| Initial real semantic/fault/burst acceptance | `fc8be4d6` | `4aefed78d1335f05fa92bf959a3af97669a0763aa4b9ddff4370aaf56a6be592` |
| Corrected semantic flow and repeated baseline | `c80be553` | `b773d9ae57cc77f551ef3d867d281057341c35e4e200216783c0ff01d8e6983e` |
| Independent repository/hit-path samples | `757bd1755977e943dd094bc1492acf406deb5b28` | `0793cdea12f777246b42519bfddc2aff6cd301667ab0701ef686fd049745b4d4` |

Raw results are under `measurements/local-embedding-20261001/`. There are **1044 timed client attempts**: 624 initial diagnostic attempts, 320 after the correctness fix, and 100 independent hit-path attempts. Gateway acceptance requests and failed warmups are additional observations in retained logs, not invented JSONL samples. UTC log dates are October 2; the local Vancouver execution date is October 1.

## Correctness: failure first, then shared-path repair

The initial real-model test used a three-topic static FAQ and an experimental 0.92 similarity threshold. A paraphrase hit correctly, but the refund-window negative also hit the trial answer. The failing log is retained as `on-acceptance-r2.log`. A separate preceding attempt failed on the primary's 10-second client deadline (`on-acceptance.log`); neither failure is hidden.

The root cause was the input representation: embedding the complete messages included the identical, comparatively long FAQ system prompt in every vector. Both gateway lookup and enqueue now embed **only the eligible user question**. Eligibility already restricts this to one fixed plain-text user turn; system prompt, authenticated API key, use case, knowledge version, primary model and generation settings remain in the opaque cache scope. A new `embedding-input=user-text-v1` scope component prevents entries using the old representation from being reused. No model names or dimensions were added to runtime source.

Corrected real gateway flow (`on-acceptance-fixed.log`): primary miss → bounded observation of actual asynchronous persistence → paraphrase **1/1 hit**, identical answer hash and 55.782 ms complete response → different-answer negatives **2/2 miss** → close the old fixture and construct `faq-v2` using the **same API key, model and database** → first new-version request misses the old answer immediately. This proves the test fixture's version isolation; it does not define a production atomic publishing procedure or a broad semantic-quality guarantee.

Embedding fault (`on-fault-fixed.log`): the native HTTP adapter successfully probes the real model through a transparent proxy, then the proxy injects HTTP 503. Lookup/store errors are observed, reason counters include `lookup/embedding_error` and `store/embedding_error`, and the ordinary primary response returns **HTTP 200, cache hit false**, complete-response time 3783.793 ms. Real vector faults, forced shutdown cancellation and cross-model cases retain deterministic coverage; no new live two-model result is claimed.

## Matched-schedule low-hit measurements

Corpus: the numbered-plan trial FAQ fixture in `phase29_live_test.go`. Each plan has its own trial length; every 21st request repeats the preceding plan. Runs use 64 requests, 650 ms scheduling interval (**target 1.538 RPS**), and maximum four client requests in flight. Potential exact repeats are 3/64 = 4.69%; observed corrected-run hit rates are 1.56%, 0%, 1.56%. The threshold is 0.99999 for this load corpus, separately from the semantic-quality fixture's 0.92.

Warmup is a dimension probe followed by a real gateway request for sentinel plan 999; warmup client latency is excluded. **A limitation is retained explicitly:** its asynchronous write may finish after sampling begins, contributing up to one warmup store operation per on-run. The operation samples are consequently diagnostic, not a clean steady-state-only distribution. A future release-grade runner must observe warmup persistence before starting its recorder. No HTTP header timing substitutes for completion: client timing ends after the body reaches EOF, including read errors.

Order: off/on, on/off, off/on; tests run serially, each with `-test.timeout 60s`. Nearest-rank quantiles below include only successfully completed HTTP 200 responses; failed attempts and their durations remain in JSONL and `summary.json`. Achieved RPS includes foreground drain; average concurrency is summed client durations divided by foreground duration.

| Condition | OK / attempts | Actual attempt / success RPS | Peak / average concurrency | Hits | P50 ms | P95 ms | P99 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| Off r1 | 62 / 64 | 1.251 / 1.212 | 4 / 3.312 | 0 | 1924.353 | 5979.132 | 9813.718 |
| On r1 | 61 / 64 | 1.246 / 1.187 | 4 / 3.358 | 1 | 1795.951 | 6832.858 | 8146.200 |
| Off r2 | no timed samples | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable |
| On r2 | 60 / 64 | 1.399 / 1.311 | 4 / 3.299 | 0 | 1472.185 | 3565.431 | 5361.321 |
| Off r3 | 64 / 64 | 1.491 / 1.491 | 4 / 2.612 | 0 | 1408.763 | 3807.286 | 4592.183 |
| On r3 | 59 / 64 | 1.286 / 1.186 | 4 / 3.636 | 1 | 1766.925 | 5751.333 | 8993.873 |

Off r2 completed its embedding dimension probe but its **primary warmup timed out**, before client sampling. Its empty JSONL is excluded and its failure log is retained. The two available same-schedule P95 ratios are **1.142784** and **1.510612**, both above `1.05`. Round 3 P50 also increases approximately 25.4%. Each 64-attempt slice still places P99 near its maximum; repetition does not make these individual tail estimates robust. The on-runs have 12 failed clients and off-runs have two; failed-attempt tails are about 10 seconds. Different achieved load and upstream tails prohibit a causal attribution of the entire increase to caching, but equally prohibit approving the cache parameters from these data.

Before the correctness fix, three higher-load pairs and four scheduled 1 RPS pairs were also attempted. The lower-rate experiment produced six nonempty 40-request files; two off warmups timed out. On actual RPS ranged 0.833–0.997 and off 0.851–0.889. These old-format observations are retained for diagnosis, **not pooled with the corrected implementation's gate**. No continuous long steady-state run or stable end-to-end concurrency capacity has been established. A maximum client ceiling of four is not a sustainable-capacity result. Lowering load did not eliminate the primary deadline failures even with caching off.

## Independent operation samples

Corrected low-hit on-runs combined; successful operations unless stated otherwise. Counts include the warmup-overlap limitation above. `store_total` includes embedding, repository store and vector insertion. Failed lookup embedding samples have separate all-attempt P95/P99 of 100.694/101.689 ms; 19/192 embedding calls fail at the experimental read budget. Seven of 173 vector searches fail and are retained, rather than silently counted as successful retrievals.

| Operation | Successful n | P95 ms | P99 ms | Failed n |
| --- | ---: | ---: | ---: | ---: |
| Lookup embedding | 173 | 99.218 | 100.196 | 19 |
| Store embedding | 181 | 67.820 | 112.628 | 0 |
| Vector retrieval | 166 | 2.542 | 3.648 | 7 |
| Repository read, low-hit | 2 | 0.412 | 0.412 | 0 |
| Repository write | 181 | 0.455 | 1.131 | 0 |
| Vector insertion | 181 | 4.398 | 33.065 | 0 |
| Complete store | 181 | 79.012 | 116.630 | 0 |

Since low-hit traffic yields only two repository reads, a separate **100-request, 100% exact-hit** gateway run supplies an explicitly different sample. All 100 responses succeed and hit: complete-response P50/P95/P99 **16.559/24.475/54.641 ms**, actual 56.046 RPS, concurrency one, duration 1.784 seconds. Component P95/P99: lookup embedding **21.853/51.576 ms**, vector search **2.002/2.574 ms**, repository read **0.360/0.408 ms**, each n=100 with zero errors. This short warm hit-path sample is neither the low-hit gate nor a sustained 56-RPS capacity claim.

From overlapping operation intervals in the corrected low-hit runs, observed embedding peak concurrency is at most two reads, two writes and **four combined**. The configured maximum of six combined operations was not continuously saturated, so stable capacity at that maximum remains unproven.

## Concrete isolation candidates and burst behavior

These values are **review candidates for isolation only**, not approved final production settings:

| Parameter | Concrete value | Evidence and limit |
| --- | ---: | --- |
| Total read cutoff | **100 ms** | Real paraphrase flow works, but 19/192 lookup embeddings and seven searches fail under this budget; complete-response ratios fail. Keep experimental, do not approve as final. |
| Maximum concurrent reads | **4** | Deterministic admission limit verified; only peak two active read embeddings observed live, so sustained capacity four remains unproved. |
| Write workers | **2** | Two live store embeddings overlap; all observed completed stores succeed. |
| Queue capacity | **32** | A 128-candidate real-dependency burst accepts at most 32 queued plus two active workers. |
| Independent write timeout | **2 s** | Observed complete-store P99 is 116.630 ms; this timeout bounds faults rather than guaranteeing queue completion. |
| Shutdown grace | **1 s** | Corrected burst closes in 656.187 ms; forced cancellation/drop paths pass deterministic tests. Deployment stop deadline is still unknown. |

Real burst (`on-burst-fixed.log`): **128 candidates, 34 accepted, 94 dropped newest**, enqueue loop 0.050 ms; all 34 accepted stores complete before close returns. A post-close enqueue is rejected. HeapAlloc before/after is 2,160,536/5,053,568 bytes; this includes adapter/network/storage allocation and is not queue-only memory or peak RSS. This component burst intentionally constructs non-sensitive cache candidates directly; it is not represented as 128 primary gateway completions.

Policy: never wait for a full optional queue; drop the newest candidate. Reject enqueue after close starts. At grace expiry, cancel workers and discard remaining queue entries, with separate `shutdown_cancelled` and `shutdown_drop` counters. Guarantee bounded optional work, not lossless caching. **Approval owner for isolation parameters is the requesting user.** Production FAQ publisher, atomic cutover owner and deployment stop deadline are unresolved; production enablement requires separate approval.

## Checks and reproduction

After the shared-path fix: full `go test -timeout 60s ./...` **passes**, with Redis/Qdrant/PostgreSQL available (integration package 15.889 s); focused `-race` cache/gateway regression **passes**; build-tagged app compilation and `go vet` **pass**. Local acceptance tests are opt-in, skipped in ordinary CI. No frontend files changed.

Recompute all retained samples without credentials:

```bash
node scripts/phase29-analyze.mjs .planning/phases/29-semantic-cache-latency-hardening/measurements/local-embedding-20261001
```

Build the Linux runner from the recorded revision with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -tags phase29preflight ./internal/app`. Set the embedding endpoint/model, cache bounds, mode, client concurrency, count, interval and private output path explicitly. Supply existing primary credentials, optional local authentication placeholder and Qdrant credentials via a JSON environment map on stdin. Do not write those values into tracked configuration or command logs. Use a loopback SSH reverse tunnel for the user-owned local model.

Runner selections are `TestPhase29LiveMeasure`, `TestPhase29LocalGatewayAcceptance`, `TestPhase29LocalEmbeddingFault`, `TestPhase29LocalQueueBurst`, and `TestPhase29LocalHitReadMeasure`; each invocation uses a hard 60-second timeout. The static fixture's actual non-admin API key is created in and read back from the isolated database before allowlisting. Models/dimensions are configured/probed per run. Other API keys remain outside that allowlist.

Remaining release gates: stable representative primary/network/repository load with clean warmup and longer sustained evidence; passing `1.05` low-hit gate; second distinct real-model full lifecycle and live cross-model/fault cases; numeric owner approval and final human review. One local model cannot close the original two-model requirement. The other model visible in the local server's inventory was **not selected or counted as tested**.

Cleanup: the three isolated containers were verified stopped; their volumes, opaque vector entries, private test binaries and redacted raw results remain recoverable. The task's loopback SSH tunnel and temporary transport helpers were removed. The user's local embedding service was not stopped. A scan against credential values from the existing environment found no matches in retained samples/logs.
