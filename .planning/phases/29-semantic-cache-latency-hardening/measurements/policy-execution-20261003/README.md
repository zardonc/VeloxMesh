# Phase 29 policy execution evidence

The batch uses the existing isolated VM, test Redis/PostgreSQL/Qdrant containers and the local LM Studio service. Credentials are read from untracked `.env`/`.env.local`, retained in memory and passed through verified SSH stdin. Do not copy these files into the artifact directory. The runner starts only stopped test containers and restores their state when the batch exits.

Build from the repository root in Git Bash:

```bash
export GOCACHE=C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/go-cache
mkdir -p .tmp/policy-execution-20261003
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -tags phase29preflight -c -o .tmp/policy-execution-20261003/app-linux-replay.test ./internal/app
go build -o .tmp/policy-execution-20261003/runner-replay.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
```

Choose a **new** results root for a replay so existing evidence remains intact. These explicit values reproduce the isolated local candidate, not a production configuration:

```bash
export SHIP_RESULTS_ROOT=.planning/phases/29-semantic-cache-latency-hardening/measurements/policy-replay
export SHIP_ARTIFACTS="$SHIP_RESULTS_ROOT/lifecycle"
export SHIP_BINARY=.tmp/policy-execution-20261003/app-linux-replay.test
export SHIP_RUNNER=.tmp/policy-execution-20261003/runner-replay.exe
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
"$SHIP_RUNNER" batch
```

The default batch runs safety, balanced low-hit/pure-miss windows, capacity, protected protocols, full backend regression and exact safety. `SHIP_BATCH_STAGES` can select a comma-separated subset of those named stages, or `final-checks`. One runner batch owns dependency lifetime; do not run batches in parallel. A task-local `PAUSE` file in the results root prevents new child runners after the active one cleans up.

Backend test processes retain a 60-second Go and outer deadline. Dependency startup has a separate bounded 120-second readiness window; the existing Qdrant volume contains historical test collections. Tests must emit the expected named PASS and no SKIP; a zero exit with no matching test is rejected.

Recalculate from completed windows:

```bash
UV_CACHE_DIR=C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/uv-cache uv run --no-project scripts/test_phase29_metrics.py
UV_CACHE_DIR=C:/Users/inthe/IdeaProjects/VeloxMesh/.tmp/uv-cache uv run --no-project .planning/phases/29-semantic-cache-latency-hardening/measurements/policy-execution-20261003/evaluate.py --root "$SHIP_RESULTS_ROOT"
```

`block-selection.json` records original windows and at most one recheck per failed window. Failed attempts remain in separate logs. If a selected window has complete request data but failed HTTP attempts, the evaluator includes its failures and marks it diagnostic; missing/incomplete/skip evidence cannot pass. Whole/miss gates and residual budgets remain separate. Bootstrap intervals are exploratory with six temporal blocks.

This execution's directories:

| Directory/artifact | Meaning |
| --- | --- |
| Root `progress.jsonl`, initial safety/low-hit folders | Initial attempts, including sandbox/startup and test assertion failures. |
| `warm-batch/` | Warm dependency batch: twelve balanced scenario blocks, capacity, known failures and one recheck. |
| `warm-batch/provider-protection-final/` | Final source: six real Provider protection tests. |
| `warm-batch/protected-stream-final/` | Final usage option: SSE/cancellation/buffering/Fusion passed. |
| `final-backend/` | Final complete backend suite and three acceptance regressions, all passed. |
| `evaluation-readout.json`, `warm-batch/evaluation.json` | Window-aware diagnostics; original and candidate AND gates. |
| `summary.json`, `manifest.json` | Per-test statuses, retained HTTP failures and SHA-256 source/binary/evidence inventory. |
| `vm-diagnostics*.log`, `direct-usage-stream.log` | Read-only VM attribution snapshots and actual upstream usage confirmation. |
| `lmstudio-engine-errors.log`, `credential-audit.log` | Bounded time-aligned upstream error excerpts and loaded-credential audit results. |

`summarize.mjs` regenerates the inventory for this fixed evidence root after documentation is finalized. Audit the fixed artifact root with `SHIP_ARTIFACTS=<root> <runner> audit`; it reports matches without exposing credential values. Final cleanup evidence is in `final-backend/lifecycle/runner.log`.

The semantic dataset/recall decision and true isolated model unload/reload test are pending. This batch does not enable production cache, erase old test data, claim model fact accuracy or prove a physical latency lower bound.
