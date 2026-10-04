# Availability investigation evidence

The report is [29-AVAILABILITY-INVESTIGATION-20261004.md](../../29-AVAILABILITY-INVESTIGATION-20261004.md). Use a new output directory for every replay. Existing raw failures, baseline sources, binaries and prior policy-execution fingerprints remain historical evidence.

| Directory | Meaning |
| --- | --- |
| `baseline/` | Four pre-fix production sources; exact executed Redis observer source is retained as `.txt`. |
| `baseline-network/` | Real delayed Redis reads: matrix PASS, expected dependency-category HTTP RED at 60/100ms. |
| `routes-red/` | Initial route fixture missing Combo.Name; retained test setup failure. |
| `routes-corrected-red/` | Corrected fixture compiled against saved pre-fix production sources with Go overlay; five routes RED. |
| `green/` | Eleven real selected health, HTTP, routing, recovery, Redis and protected protocol tests PASS. |
| `final-backend/` | Full backend plus three real Phase 29 acceptance tests PASS; two unrelated Plan4 opt-in skips. |
| `workload/`, `workload-readout.json` | One 100-request off/on pair; zero HTTP failures, two initial Qdrant NotFound lookup errors retained. |
| `direct-replay/` | Five 32-request direct upstream windows plus eight separately labelled intentional cancellations. |
| `vm-diagnostics*.log` | Earlier stopped-container / missing-remote-rg diagnostic errors, not warm application evidence. |
| `redis-workload-observation.json` | Bounded startup/first-GET observation, sanitized SLOWLOG metadata; not continuous warm-window monitoring. |
| `summary.json`, `manifest.json`, `credential-audit.log` | Counts, explicit RED/GREEN statuses, source/binary/evidence hashes and credential-value audit. |

From the repository root in Git Bash, compile a fresh tagged Linux binary and the existing isolated runner:

```bash
export GOCACHE="$PWD/.tmp/go-cache"
mkdir -p .tmp/availability-replay
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -tags phase29preflight -c -o .tmp/availability-replay/app.test ./internal/app
go build -o .tmp/availability-replay/runner.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
```

The runner reads untracked `.env`/`.env.local`, validates the existing SSH host key, sends credentials through stdin, temporarily starts only the existing test containers and restores their initial state. Do not copy configuration into artifacts. Use one runner at a time. The VM endpoint below is the existing reverse SSH forward; the Windows service uses port 1234.

```bash
export SHIP_BINARY=.tmp/availability-replay/app.test
export SHIP_ARTIFACTS=.planning/phases/29-semantic-cache-latency-hardening/measurements/availability-replay/green
export SHIP_PROVIDER=LOCAL
export SHIP_LOCAL_BASE_URL=http://127.0.0.1:11234/v1
export SHIP_LOCAL_MODEL=qwen2.5-0.5b-instruct
export SHIP_LOCAL_API_KEY=local-test
export PHASE29_MODEL=text-embedding-embedder_collection
export PHASE29_EMBEDDING_BASE_URL=http://127.0.0.1:11234/v1
export PHASE29_READ_TIMEOUT=100ms
export PHASE29_READ_CONCURRENCY=4
export PHASE29_WRITE_WORKERS=2
export PHASE29_QUEUE_CAPACITY=32
export PHASE29_WRITE_TIMEOUT=2s
export PHASE29_SHUTDOWN_GRACE=1s
export PHASE29_PROVIDER_PROTECTION='{"sans-primary":{"first_byte_timeout":"5s","first_content_timeout":"5s","stream_idle_timeout":"5s","total_timeout":"30s","max_inflight":4}}'
export SHIP_TESTS=TestLiveHealthSnapshotDelayMatrix,TestLiveRoutingHealthDependencyError,TestLiveHealthDependencyRoutes,TestLiveHealthDependencyHealthyPeer,TestLiveStreamSettlement,TestLiveStreamCancellation,TestLiveBufferedStream,TestLiveFusionStream,TestLiveRedisProviderIsolation,TestLiveRedisConcurrentSnapshots,TestLiveRedisStalePublication
.tmp/availability-replay/runner.exe
```

All values above are explicit isolated test candidates. Each backend process has both a 60-second Go timeout and an outer hard bound. Each selected test must emit its named PASS without SKIP; no matching tests is failure. Unset `SHIP_TESTS` and choose another artifact root to run the full backend and the runner's three default acceptance tests.

For the bounded workload pair, retain the same environment and use another fresh root:

```bash
export SHIP_ARTIFACTS=.planning/phases/29-semantic-cache-latency-hardening/measurements/availability-replay/workload
export SHIP_TESTS=TestLiveCacheLoadOff,TestLiveCacheLoadOnWithoutMemo
export PHASE29_COUNT=100
export PHASE29_INTERVAL_MS=125
export PHASE29_CLIENT_CONCURRENCY=4
.tmp/availability-replay/runner.exe
```

`workload-readout.py` evaluates its own containing root. To evaluate a replay, copy it there, preserving the sibling `policy-execution-20261003/evaluate.py` parser dependency, then run `UV_CACHE_DIR="$PWD/.tmp/uv-cache" uv run --no-project <replay-root>/workload-readout.py`. This is deliberately one diagnostic pair, not six-block release validation. Future continuous monitoring must outlive both warm windows; the current observer stops after first GET.

Run the upstream-only replay against the already-running local model after setting `PHASE29_UPSTREAM_API_KEY` in the process environment. Do not paste a credential into a retained command/log:

```bash
node .planning/phases/29-semantic-cache-latency-hardening/measurements/availability-investigation-20261004/upstream-replay.mjs http://127.0.0.1:1234/v1 qwen2.5-0.5b-instruct .planning/phases/29-semantic-cache-latency-hardening/measurements/availability-replay/direct
```

The script only accepts explicit local loopback HTTP inputs, enforces 55s total / 4s request bounds, captures bounded sanitized failure bodies and socket reuse, closes agents and separates intentional cancellations from errors. It does not unload a shared model or call a paid provider.

The exact Redis observer source used for retained measurements is `baseline/redis-observation-executed.txt`; its binary is hashed in the manifest. The reusable `redis-observation.go` extracts output handling and accepts a new output path; this refactor occurred after measurement and is not falsely presented as the executed source. Build it with `go build -o .tmp/availability-replay/observer.exe <evidence-root>/redis-observation.go`, then run alongside a single runner as `.tmp/availability-replay/observer.exe <new-output.json>`. It is read-only, validates known_hosts and retains errors. The 55s bound closes its SSH client; SLOWLOG arguments are discarded before serialization.

`summarize.mjs` regenerates this fixed root's summary and manifest only. Before regeneration, run `SHIP_ARTIFACTS=<evidence-root> <runner> audit`; audit reports matching paths/counts without exposing credential values. Recheck all listed hashes after the final inventory. Prior policy-execution artifacts must not be regenerated against this newer source.
