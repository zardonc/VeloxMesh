---
status: complete
phase: 26-scheduler-scoring-backpressure-hardening
source: .planning/phases/26-scheduler-scoring-backpressure-hardening/26-01-PLAN.md, .planning/phases/26-scheduler-scoring-backpressure-hardening/26-03-PLAN.md, .planning/phases/26-scheduler-scoring-backpressure-hardening/26-04-PLAN.md
started: 2026-07-08T19:51:19Z
updated: 2026-07-11T00:51:58Z
---

## Current Test

[testing complete]

## Tests

### 1. External scorer backpressure quick-fails
expected: Gateway-side Scheduler scoring and scheduler-side Python predictor scoring cap external calls with non-blocking concurrency slots. Requests above the cap return fallback immediately instead of waiting behind slow scorer work.
result: pass

### 2. Slow successful scorer calls degrade
expected: Slow but successful external scorer and predictor responses are treated as degraded, recorded against breaker state, and return fallback results rather than blocking intake.
result: pass

### 3. Breaker uses a small failure-rate window
expected: Alternating failure and success still opens the breaker when recent failures exceed the configured small window; one success does not erase recent failures.
result: pass

### 4. Operator docs and config expose fallback policy
expected: Operators can configure scorer max concurrency and slow threshold through env and JSON examples, and the runbook states predictive scoring is an optimization path that should quick-fail to heuristic/FIFO.
result: pass

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Evidence

- `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/scheduler/predictor ./internal/scheduler/onnx ./cmd/scheduler -count=1` passed.
- `go test -timeout 60s ./...` passed.
- `go build ./...` passed.
- `git diff --check` passed with only existing CRLF conversion warnings for `.planning/ROADMAP.md` and `.planning/STATE.md`.
- Real component coverage included local TCP gRPC Scheduler and Python predictor test servers plus ONNX artifact/model scorer tests.

## Additional Evidence: 26-03

- `go test -timeout 60s ./internal/scheduler -run TestTaskIntakeConcurrent -count=1 -v` passed and covered concurrent gateway admission under the hard queue limit.
- `go test -timeout 60s ./internal/scheduler -run SubmittedRequestContext -count=1 -v` passed and covered chat plus stream handlers receiving the executor-provided context.
- `go test -timeout 60s ./internal/scheduler -run TestExtractSafeFeaturesChineseSentenceLengthBucket -count=1 -v` passed.
- `go test -timeout 60s ./internal/http/handlers -run TestAdminSchedulerRollout -count=1` passed with the real admin handler repository path.
- `go test -timeout 60s ./cmd/scheduler -run TestSchedulerServiceUsesPythonONNXWorkerSmoke -count=1 -v` passed using the real local Python ONNX worker process.
- `go test -timeout 60s ./cmd/scheduler -run TestSchedulerServicePredictorBreakerEnvUsesRealTCP -count=1 -v` passed using a real TCP predictor path.
- `PATH=/c/Soft/1A-Coding/w64devkit/bin:$PATH go test -race -timeout 60s ./internal/scheduler ./internal/scheduler/predictor -count=1` passed.
- `go test -timeout 60s ./internal/scheduler ./internal/http/handlers ./cmd/scheduler -count=1` passed after fixing a flaky real TCP scorer test timeout.
- `go test -timeout 60s ./... -count=1` passed.
- `go build ./...` passed.
- `git diff --check` passed with only the expected CRLF warning for `internal/scheduler/client_test.go`.
- `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v` passed and returned HTTP 200 through the real provider integration.

## Additional Evidence: 26-04

- `go test -timeout 60s ./internal/scheduler -run "TestExecutorRunOneUsesRegisteredTaskContext|TestExtractSafeFeaturesUnspacedCJK|TestWeightedScorerONNXFailureFallsBackToHeuristicThenFIFO" -count=1 -v` passed using the real scheduler registry, queue, scorer, and feature extraction paths.
- `go test -timeout 60s ./internal/app -run "TestNewSchedulerQueueExplicitRedisIsNodeScoped|TestAppCloseCancelsLifecycle" -count=1 -v` passed using the real app wiring and `miniredis` Redis protocol component.
- `go test -timeout 60s ./internal/scheduler ./internal/app -count=1` passed.
- `go test -timeout 60s ./... -count=1` passed, including `tests/integration`.
- `go build ./...` passed.
- The local PowerShell PATH did not expose Go, so verification used the existing environment Go binary at `C:\Soft\1A-Coding\go1.26.1.windows-amd64\go\bin\go.exe` with repository-local `GOCACHE=.gocache`.

## Gaps

[none]
