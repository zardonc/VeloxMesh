---
status: resolved
trigger: "Scheduler score result semantics mark healthy scorer results as fallback failures, and scorer/predictor breakers are not thread-safe."
created: 2026-07-10
updated: 2026-07-10
---

# Debug Session: Scheduler Score Breaker

## Symptoms

- Healthy heuristic scheduler results can carry `reason=structured`, `rule`, or `fallback`.
- Gateway client maps `reason` into `FallbackReason`, counts non-empty values as failures, and can open the breaker.
- ONNX scorer marks normal ONNX results with `FallbackReason=onnx`.
- Gateway scorer and Python predictor breaker state machines are mutable and used from concurrent calls.

## Current Focus

- hypothesis: Score-result metadata is overloaded, and duplicated breaker state machines lack synchronization.
- test: Real TCP scheduler/predictor paths should keep healthy scores usable; concurrent breaker use should not corrupt state.
- expecting: Healthy classification metadata must not populate `FallbackReason`; breaker methods must be locked.
- next_action: resolved; monitor for future proto toolchain availability before adding a wire-level classification_source field.

## Evidence

- timestamp: 2026-07-10
  observation: `heuristic.ScoreCalculator` writes `classification.Source` into `ScoreResult.FallbackReason`.
- timestamp: 2026-07-10
  observation: `GRPCScorer.Score` records failure for any result with non-empty `FallbackReason`.
- timestamp: 2026-07-10
  observation: `breaker` and `clientBreaker` duplicate mutable ring-buffer logic without a mutex.

## Resolution

root_cause: `FallbackReason` mixed healthy classification/source metadata with actual fallback and failure reasons, while duplicated breaker implementations mutated shared window/open-state fields without synchronization.
fix: Added `ScoreResult.ClassificationSource`, kept `FallbackReason` reserved for real fallback/failure reasons, made new scheduler services emit proto `reason` only for fallback reasons, and added legacy client parsing for old `reason=structured|rule|fallback|onnx` responses. Replaced duplicated scheduler/predictor breakers with one mutex-protected `scheduler.Breaker` that supports injected time and a single half-open probe slot. Race verification also exposed and fixed `WorkerProcess.Ensure` reading `exec.Cmd.ProcessState` while `cmd.Wait()` writes it.
verification: `go test -timeout 60s ./internal/scheduler ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler/heuristic ./cmd/scheduler`; `PATH="/c/Soft/1A-Coding/w64devkit/bin:$PATH" go test -race -timeout 60s ./internal/scheduler ./internal/scheduler/predictor`; `go test -timeout 60s ./...`; `go build ./...`; `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v`.
files_changed: internal/scheduler/breaker.go, internal/scheduler/client.go, internal/scheduler/types.go, internal/scheduler/heuristic/score.go, internal/scheduler/heuristic/server.go, internal/scheduler/onnx/scorer.go, internal/scheduler/onnx/server.go, internal/scheduler/predictive/server.go, internal/scheduler/predictor/breaker.go, internal/scheduler/predictor/process.go, internal/scheduler/intake.go, internal/observability/prometheus.go, scheduler regression tests.

## Follow-up: Bug 3/4/5 Verification

source: Phase 26 planning docs only contain explicit numbered scheduler bugs 1-3. The actionable post-bug1/2 item is Bug 3: ONNX quality alerts must use sample windows instead of single samples. The same plan also treats executor empty-queue waiting and rollout alert retention as required regression coverage. No local planning/doc source with explicit Bug 4, Bug 5, or Bug 6 numbering was found.
result: Current code already contains the long-term Phase 26 fixes: quality sample window config/admin runtime control, windowed ONNX quality alerts, executor empty-queue/running-task regression coverage, and rollout alert retention cap coverage. Bug 6 was not implemented.
verification: `go test -timeout 60s ./internal/scheduler -run "TestPredictionQuality|TestRolloutController|TestSynchronousRunner|TestExecutorRaceCondition" -count=1 -v`; `go test -timeout 60s ./internal/config ./internal/http/handlers -run "TestScheduler|TestAdminScheduler" -count=1 -v`; `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/http/handlers -count=1`; `go test -timeout 60s ./internal/scheduler/predictor ./cmd/scheduler -count=1`; `go test -timeout 60s ./internal/scheduler/onnx ./internal/scheduler/predictive -count=1`; `PATH="/c/Soft/1A-Coding/w64devkit/bin:$PATH" go test -race -timeout 60s ./internal/scheduler ./internal/scheduler/predictor`; `go test -timeout 60s ./...`; `go build ./...`; `git diff --check`; `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v`.

## Follow-up: Operator-Reported Bug 3/4/5 Fix

source: User clarified Bug 3/4/5 as Prometheus breaker state staleness, incomplete ONNX/predictive deployment runbook contract, and missing predictive anomaly metrics wiring. User clarified Bug 6 is the documented multi-node runtime propagation limitation and should remain documentation-only.
fix: Scheduler intake now refreshes `gateway_circuit_breaker_state` after scoring; weighted heuristic/ONNX scorers aggregate child breaker states for metrics so any open child reports `open`. The scheduler process now registers observability metrics on its Prometheus registry and passes them into `predictive.NewScorer`, so predictive anomaly/OOD counters are emitted. The runbook now separates Gateway, Scheduler process, and Python predictor worker env contracts and includes status health proof.
verification: `go test -timeout 60s ./internal/scheduler -run "TestTaskIntakeRecordsGRPCBreakerStateMetric|TestGRPCScorer|TestWeightedScorer" -count=1 -v`; `go test -timeout 60s ./cmd/scheduler -run "TestSchedulerServiceUsesPythonONNXWorkerSmoke|TestSchedulerServiceONNX" -count=1 -v`; `go test -timeout 60s ./internal/config ./internal/scheduler ./internal/http/handlers ./cmd/scheduler ./internal/scheduler/onnx ./internal/scheduler/predictive -count=1`; `PATH="/c/Soft/1A-Coding/w64devkit/bin:$PATH" go test -race -timeout 60s ./internal/scheduler ./internal/scheduler/predictor`; `go test -timeout 60s ./...`; `go build ./...`; `git diff --check`; `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v`.
