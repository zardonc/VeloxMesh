---
status: diagnosed
trigger: "Investigate the two VeloxMesh scheduler bugs described by the user: (1) ScoreResult/FallbackReason semantic mixing causes healthy heuristic/ONNX scorer results to be counted as failures and may open the breaker; (2) scheduler GRPCScorer breaker and predictor clientBreaker are not thread-safe. Read only; do not edit. Return a concise root-cause confirmation, affected files, and the minimal invariant tests that should exist after the fix. Do not use mocks in recommendations; prefer real TCP gRPC scheduler/predictor components where practical. Repository: C:\\Users\\inthe\\IdeaProjects\\VeloxMesh."
created: 2026-07-10T00:00:00-07:00
updated: 2026-07-10T00:00:00-07:00
---

## Current Focus
<!-- OVERWRITE on each update - reflects NOW -->

hypothesis: Confirmed. GRPCScorer treats any non-empty ScoreResult.FallbackReason as a failed scheduler result, while healthy heuristic/ONNX scorers also write non-failure semantics into that field; scheduler and predictor breaker structs mutate shared state without synchronization.
test: Source-read confirmation across scheduler client, heuristic/ONNX scorers, predictor client breaker, intake metrics, and current tests.
expecting: Post-fix invariant tests should prove healthy heuristic/ONNX reasons do not open breakers or count as fallback/failure, and concurrent real TCP client calls are race-free under go test -race.
next_action: Return concise root-cause confirmation and minimal invariant tests to user; do not edit product code.

## Symptoms
<!-- Written during gathering, then IMMUTABLE -->

expected: Healthy heuristic/ONNX scorer results should not be counted as scorer failures, and scheduler/predictor breaker state should be concurrency-safe.
actual: User reports ScoreResult/FallbackReason semantic mixing counts healthy heuristic/ONNX scorer results as failures and may open the breaker; scheduler GRPCScorer breaker and predictor clientBreaker are not thread-safe.
errors: none reported
reproduction: Inspect scheduler scoring paths and concurrent breaker use; prefer real TCP gRPC scheduler/predictor components for recommended invariant tests.
started: unknown

## Eliminated
<!-- APPEND only - prevents re-investigating -->

## Evidence
<!-- APPEND only - facts discovered -->

- timestamp: 2026-07-10T00:00:00-07:00
  checked: internal/scheduler/client.go GRPCScorer.Score and merge/fallback helpers
  found: Score marks success=false for every result with non-empty FallbackReason, then records that into the breaker. mergeResults only falls back to FIFO for missing task IDs; otherwise proto reason is copied into ScoreResult.FallbackReason.
  implication: Any scheduler server that uses reason for non-failure metadata can cause GRPCScorer breaker failures even when the RPC succeeded and scores are usable.
- timestamp: 2026-07-10T00:00:00-07:00
  checked: internal/scheduler/heuristic/score.go and internal/scheduler/onnx/scorer.go
  found: Heuristic ScoreCalculator sets FallbackReason to classification.Source, while ONNX scorer sets FallbackReason to "onnx" on normal ONNX scores.
  implication: Healthy heuristic/ONNX scores are encoded in the same field used by GRPCScorer and intake metrics to mean fallback/failure.
- timestamp: 2026-07-10T00:00:00-07:00
  checked: internal/scheduler/intake.go
  found: TaskIntake passes score.FallbackReason as both call reason and classification source; schedulerCallResult maps most non-empty reasons to "fallback".
  implication: The same semantic mixing also pollutes scheduler call telemetry for healthy non-FIFO scoring paths.
- timestamp: 2026-07-10T00:00:00-07:00
  checked: internal/scheduler/client.go breaker and internal/scheduler/predictor/breaker.go clientBreaker
  found: Both breaker types read and mutate events, next, count, failures, and openedAt from Allow/State/Record/reset without a mutex or atomic discipline, while callers can invoke Score/Predict concurrently.
  implication: Concurrent scheduler/predictor client calls can race on breaker state and produce undefined breaker behavior.
- timestamp: 2026-07-10T00:00:00-07:00
  checked: internal/scheduler/client_test.go and internal/scheduler/predictor/python_client_test.go
  found: Existing tests already use real local TCP gRPC servers and cover breaker behavior, busy fallback, and slow fallback, but not the semantic FallbackReason invariant or concurrent race invariants.
  implication: Minimal regression coverage should extend those suites with real TCP tests rather than mocks.

## Resolution
<!-- OVERWRITE as understanding evolves -->

root_cause: ScoreResult.FallbackReason is overloaded for classifier/source metadata and actual fallback/failure reasons, and GRPCScorer/TaskIntake interpret every non-empty value as fallback/failure. Separately, scheduler breaker and predictor clientBreaker share mutable ring-buffer/open-state fields across concurrent Score/Predict calls without synchronization.
fix: Not applied; user requested read-only investigation.
verification: Source-read confirmation only. No product code edited and no tests run.
files_changed: []
