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
fix: Added `ScoreResult.ClassificationSource`, kept `FallbackReason` reserved for real fallback/failure reasons, made new scheduler services emit proto `reason` only for fallback reasons, and added legacy client parsing for old `reason=structured|rule|fallback|onnx` responses. Replaced duplicated scheduler/predictor breakers with one mutex-protected `scheduler.Breaker` that supports injected time and a single half-open probe slot.
verification: `go test -timeout 60s ./internal/scheduler ./internal/scheduler/predictor ./internal/scheduler/onnx ./internal/scheduler/heuristic ./cmd/scheduler`; `go test -timeout 60s ./...`; `go build ./...`; `go test -timeout 60s ./tests/integration -run TestPlan4PostgresSansPrimaryRealProviderSmoke -count=1 -v`. `go test -race -timeout 60s ./internal/scheduler ./internal/scheduler/predictor` was blocked because gcc is not installed.
files_changed: internal/scheduler/breaker.go, internal/scheduler/client.go, internal/scheduler/types.go, internal/scheduler/heuristic/score.go, internal/scheduler/heuristic/server.go, internal/scheduler/onnx/scorer.go, internal/scheduler/onnx/server.go, internal/scheduler/predictive/server.go, internal/scheduler/predictor/breaker.go, internal/scheduler/intake.go, internal/observability/prometheus.go, scheduler regression tests.
