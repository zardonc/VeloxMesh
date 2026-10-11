# Replay and evidence boundaries

Run from the repository root in Git Bash. Credentials remain in untracked `.env.local`; the runner reads them, verifies the existing SSH host key, and sends runtime test inputs through stdin. Do not put credentials into this directory.

Build tagged real tests and runner using workspace Go caches. Every VM test has a Go60s and outer60s deadline. Runner starts only its existing isolated Redis/Qdrant/PostgreSQL containers, creates loopback/reverse forwarding, waits for real readiness, then stops owned services and verifies their stopped state.

```bash
export SHIP_RESULTS_ROOT=.planning/phases/29-semantic-cache-latency-hardening/measurements/your-new-run
export SHIP_BINARY=.tmp/your-new-run/app-linux.test
export SHIP_RUNNER=.tmp/your-new-run/runner.exe
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -tags phase29preflight -c -o "$SHIP_BINARY" ./internal/app
go build -o "$SHIP_RUNNER" ./.planning/phases/29-semantic-cache-latency-hardening/measurements/acceptance-20261001/runner
node .planning/phases/29-semantic-cache-latency-hardening/measurements/followup-20261003/suite.mjs quality
node .planning/phases/29-semantic-cache-latency-hardening/measurements/followup-20261003/suite.mjs cloud
node .planning/phases/29-semantic-cache-latency-hardening/measurements/followup-20261003/suite.mjs performance
```

Inspect the explicitly selected model names in the suite against current `.env.local`; OR/Gemini/SANS are excluded this round. The cloud preflight must pass before protocol/business calls. No parallel runner instances: they share test-only container ports. Use a fresh result directory to preserve this round's failures. Copy analysis scripts there before recomputing because their `ROOT` is their containing directory.

`verified-stable-1..6` are the actual four-condition comparison (100/condition/block,125ms arrival,C4 ceiling). The evaluator requires four observed memo hits in every memo window and complete request/stage coverage. `stable-1..3` did not activate memo and are excluded; interrupted later dispatches in `progress.jsonl` did not run application tests. `red-memo` is the pre-fix failure; the nonexistent component name is retained and subsequently rejected by the runner. Quality conditions stay distinct even when their target chat model is the same.

`evaluation.json` preserves ordinary/cold-miss/hit/bypass/saturated measurements, original1.05 verdict and proposed scenario budgets. Joint six-block bootstrap is exploratory. `summary.json` inventories all raw logs including excluded failures; its raw PASS count must not be used as a current acceptance count. `manifest.json` records exclusions and hashes, and keeps the unsafe prefix candidate failed. Zero positive recall cannot be counted as approved quality merely because a no-unsafe-hit test completed.

`host-resource.mjs` takes explicit API,embedding,chat,output arguments. It sends independent 125ms schedules for embedding/chat, preserves dispatch lag, uses only actual APIs, and starts no server. `backend-timing.mjs` reads an explicitly named existing model-server log and exports only timing/task/slot numbers, no request/answer bodies. Its two frozen wall-time windows match this run; adjust them for a future run. Backend wall-time seconds cannot correlate exact gateway request IDs.

Use `uv run --no-project python analyze.py`, `evaluate.py`, `finalize.py` with a workspace UV cache. Check hashes and copied scripts, rerun credential audit after generating reports, and retain the runner's final stopped-container output. The scripts do not merge or enable production cache. Formal scenario SLOs and safe semantic policy remain pending.
