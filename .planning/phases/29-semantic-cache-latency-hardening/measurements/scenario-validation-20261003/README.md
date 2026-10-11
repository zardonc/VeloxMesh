# Phase 29 scenario validation evidence

Results: [scenario report](../../29-SCENARIO-VALIDATION-RESULTS-20261003.md). Policy: [preregistered diagnostic plan](../../29-SCENARIO-VALIDATION-PLAN-20261003.md).

All requests use the real application, real Redis/SQLite/Qdrant and live models. Local Node and Go embedding probes bypass the application for comparison. Network faults forward actual dependency bytes, with one response delay or a connection cut. No synthetic model/component replies are used.

## Replay

From the repository root in Git Bash, use workspace Go caches, then build:

```bash
export GOCACHE='C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/diagnostic-cache'
export GOTMPDIR='C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/diagnostic-build'
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c -o .tmp/phase29-diagnostic-20261003/app-linux.test ./internal/app
go build -o .tmp/phase29-diagnostic-20261003/warmed-runner.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
```

`run.mjs` reads untracked `.env.local`, passes credentials to the runner through its normal environment handling and stdin, and writes redacted logs. It takes provider, model, comma-separated tests, output label, count, interval-ms, and client concurrency. Set `SHIP_RUNNER`, `SHIP_BINARY`, and `SHIP_RESULTS_ROOT` explicitly; use a new output directory for a replay to preserve these artifacts.

`suite.mjs` additionally requires `SHIP_LOCAL_MODEL`, `DIRECT_EMBEDDING_BASE_URL`, `PHASE29_MODEL`, `PHASE29_SECOND_EMBEDDING_MODEL`, and `SHIP_SANS_MODEL`. Each remote backend test has both Go and outer process 60-second deadlines. The small SANS batches use the same FAQ/temperature/token budget and 12-second client deadlines. The suite records individual failures in `suite-progress.jsonl`; its process exit alone does not mean every cell passed.

`local-go.mjs` runs the same tagged Go embedding tests directly on Windows, with a 60-second process deadline per test. It takes the local Windows test executable, output directory, concurrency, and optional `reverse` order. For stable controls, explicitly set `PHASE29_EMBEDDING_PARALLEL_WARMUP=16`; warmup results remain separate from measured samples.

Supplemental controls were executed after the initial suite: `supplemental-c4`, `direct-facts`, `sans-facts`, `observer-reverse-c4`, `host-go-*`, `vm-go-warm-*`, `final-boundaries`, and `final-chain-checks`. Their exact binaries and preserved SHA values are listed in `manifest.json`.

## Recalculate

```bash
UV_CACHE_DIR='C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/uv-cache' uv run python .planning/phases/29-semantic-cache-latency-hardening/measurements/scenario-validation-20261003/analyze.py
```

`summary.json` includes every recorded top-level test result and request-level timing, not just successful cohorts. Warmup/probe failures are retained in their raw logs; measured-client latency summaries do not pretend those samples exist. `profile-summary.json` derives VM CPU/steal from counter differences and STW pauses from the standalone execution trace. Go pprof/trace tools can inspect the retained profiles; `analyze-profile.py` accepts the parsed trace text as its argument.

The two earliest exploratory batches preceded binary retention; their exact hashes are unavailable. Their relevant chain checks were rerun using the final retained binary. No production-source changes, merge, or production cache enablement occurred in this task.
