# Memory upgrade controlled experiment

The final experiment is in `final/`. The root and `validated/` retain two interrupted configuration-rejected attempts; never pool them into the final performance results. `PAUSE` files intentionally stop reuse of those output roots. Use a fresh output directory for a new run.

See [the results and limits](../../29-MEMORY-UPGRADE-RESULTS-20261004.md). Product HEAD is recorded in `source-head.txt`; final application hash is in `final-binary.sha256`, and runner/initial binary hashes in `binaries.sha256`. `manifest.json` records the current relevant source hashes and every retained evidence hash. No credentials are retained.

`suite.mjs` registers two six-block scenarios and alternates on/memo inside off-before/off-after brackets. Every window has 100 scheduled requests at 125ms, ceiling four. `internal/app/live_controlled_load_test.go` waits for the seed store and then two seconds before formal measurement. The runner holds existing isolated dependencies warm and restores their initial stopped state. Backend unit tests have both Go and outer 60-second limits. Continuous VM/host observers cover startup separately from measured windows. No compiler, second load generator or inventory probe ran during formal performance windows.

Build in the repository with a writable task-local GOCACHE:

```bash
export GOCACHE="$(pwd)/.tmp/memory-upgrade-controlled-20261004/go-cache"
go vet ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
go build -o .tmp/memory-upgrade-controlled-20261004/runner.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet -tags phase29preflight ./internal/app
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c -o .tmp/memory-upgrade-controlled-20261004/app-linux-final.test ./internal/app
```

The run uses the existing untracked `.env.local` for SSH credentials and verifies `known_hosts`. Export a **new** `SHIP_RESULTS_ROOT`; also set `SHIP_ARTIFACTS` to it and `SHIP_BINARY`/`SHIP_RUNNER` to the binaries above. Supply the same explicit test inputs recorded in `final/test-plan.json`: local primary and embedding service via the VM reverse tunnel, `PHASE29_REUSE_MODE=semantic`, read=100ms, reads=4, workers=2, queue=32, write=2s, close=1s, `SHIP_TELEMETRY=true`, mixed providers disabled. Then execute `runner.exe controlled`. Test auth stays in environment/stdin and is never a command-line credential. The suite rejects the wrong reuse mode and stops before the matrix if its profile smoke fails.

```bash
export PYTHONUTF8=1
export UV_CACHE_DIR="$(pwd)/.tmp/memory-upgrade-controlled-20261004/uv-cache"
uv run --no-project python .planning/phases/29-semantic-cache-latency-hardening/measurements/memory-upgrade-controlled-20261004/evaluate.py
```

Keep `SHIP_RESULTS_ROOT` set to the run being evaluated. Python UTF-8 avoids Windows locale decoding errors. `evaluate.py` uses the existing nearest-rank/window-aware metrics and preserves failed windows; a missing or incomplete window stops evaluation. `evaluation.json` includes original/candidate AND gates, matched miss positions, actual RPS/concurrency/send lag, per-block drift, joint block bootstrap and synchronized resource windows. Outcome events lack request IDs, so their formal aggregates are filtered by recorder time through drain; request stages use exact IDs. VM's first boot-average row is excluded.

Evidence:

- `final/progress.jsonl`, `batch.log`: all stages, errors and verified final cleanup.
- `final/*/LOCAL-*/Test*.log`: actual test output, samples and timings; `*-vm-before/after.log` are telemetry, not extra test invocations.
- `final/named-tests.json`, `full-backend.jsonl` under backend-regression: 69 selected real passes, full backend passes and explicit opt-in skips.
- `final/vmstat.jsonl`, `host-resources.jsonl`: passive continuous measurements; `slow-baseline.json` retains the longest baseline requests and stages.
- `models-before/after.json`, `vm-resources-before/after.log`: model/resource inventory and stopped/no-OOM checks.
- `collections-post-test.json`: authenticated read-only metadata for all 347 collections; collected in a separate post-test startup/cleanup, without collection deletion. The repeatable helper source is `collections.go.txt`; `vm-resources.go.txt` retains the resource probe source. Compile those copied helper sources only outside measurement windows.
- `secret-audit.log`: loaded-credential audit passed.

Limitations are part of the result: fixed waiting did not eliminate paging; four brackets drifted beyond the diagnostic alert; off-first binary upload and primary first-request effects were not fully isolated; new scopes grow the preserved volume. These measurements do not prove RAM upgrade causality, universal physical impossibility, business semantic safety, or production capacity.
