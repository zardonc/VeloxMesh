# Recovery and collection-layout investigation

Read [the Chinese results and next tests](../../29-RECOVERY-INVESTIGATION-20261004.md).

The root retains a failed 120-second recovery attempt: `batch.log`, `run-result.json`, `recovery.jsonl`, `recovery-memory.log`, continuous VM/host observations, loaded model configuration and collection names. No `TestLivePrimary*` window ran. `model-timing.jsonl` is empty because no measured primary request was sent. This is not a passing performance comparison.

`idle/` is a separate preregistered seven-condition, component-only experiment. It contains the plan, every five-second memory snapshot, one-second VM/host observations, condition boundaries and explicit final stopped states. It sent no application/model traffic. Evaluation ends each active interval at its last snapshot, excluding shutdown. Conditions were run once in a fixed order; they locate pressure but do not prove collection-count causality.

`storage-allocated.log` retains the direct host access denial. The `*-readonly-helper.log` files and `storage-file-layout.log` are final stopped-volume metadata obtained with the already-cached PostgreSQL image, no network/pull, read-only root/volume and 128MiB/0.5CPU bounds. Helpers remove themselves on successful exit. The initial size inspection and final file-layout inspection are outside all observed conditions; the retained size files describe the final metadata inspection. No file contents or credentials were copied. `collection-name-comparison.json` checks 347 API names against stopped-volume directories; point counts were not refreshed.

`evaluation.json` retains the failed gate, all seven conditions and the prior run's 14 formal baseline spikes. `storage-layout-summary.json` groups metadata by WAL/file family. `source-head.txt`, `binaries.sha256`, source snapshots and `manifest.json` allow independent verification. `baseline-runner-main.go.txt` is the prior uncommitted runner before this investigation's extraction. Manifest source hashes describe this investigation, not a rewrite of the previous run's fingerprint.

Build only outside measured intervals:

```bash
export GOCACHE="$(pwd)/.tmp/memory-upgrade-controlled-20261004/go-cache"
go vet ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
go build -o .tmp/primary-warmup-investigation-20261004/runner.exe ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go vet -tags phase29preflight ./internal/app
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c -o .tmp/primary-warmup-investigation-20261004/app-linux.test ./internal/app
```

The retained `run.mjs` and `idle.mjs` show the exact local-only inputs. Both refuse an existing batch log. Use a **new output directory** for a repeat and update the wrapper paths; never rerun into this sealed evidence. `runner.exe warmup` uploads once, holds tunnels, checks recovery and runs the balanced matrix only if recovery passes. Each selected backend test has both outer and Go 60-second limits. `runner-idle.exe recovery-diagnose` requires all three existing containers stopped and restores that state. SSH credentials are loaded from untracked `.env.local`, host keys remain verified, and secrets stay out of command arguments/artifacts.

Analysis is repeatable without starting any service:

```bash
export PYTHONUTF8=1
export UV_CACHE_DIR="$(pwd)/.tmp/memory-upgrade-controlled-20261004/uv-cache"
uv run --no-project python .planning/phases/29-semantic-cache-latency-hardening/measurements/primary-warmup-investigation-20261004/evaluate.py
node .planning/phases/29-semantic-cache-latency-hardening/measurements/primary-warmup-investigation-20261004/storage-summary.mjs
```

The final stopped-container/OOM/restart check is `container-state-after.log`. The managed observers and read-only helpers exited. `cleanup-limitations.json` records an older task-owned log subscriber that Windows did not permit us to terminate; do not claim all local processes were cleaned up. Models and volumes remain present. Production cache, original 1.05 gate, candidate status and semantic-quality conclusions are unchanged.
