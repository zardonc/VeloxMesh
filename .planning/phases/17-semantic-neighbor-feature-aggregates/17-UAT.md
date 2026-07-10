---
status: complete
phase: 17-semantic-neighbor-feature-aggregates
source:
  - 17-01-SUMMARY.md
  - 17-02-SUMMARY.md
  - 17-03-SUMMARY.md
started: 2026-07-04T21:25:08.5169884-07:00
updated: 2026-07-04T22:01:21-07:00
---

## Current Test

[testing complete]

## Tests

### 1. Feature Contract, Config, And Safe Defaults
expected: Scheduler feature payloads expose bounded semantic aggregate fields, config stays disabled by default, env/JSON overrides validate, and safe extraction defaults to neutral aggregate values.
result: pass
evidence: `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/controlstate/sqlite ./internal/scheduler/onnx ./internal/scheduler/heuristic -run "TestSchedulerConfigDefaults|TestSchedulerSemanticNeighborsConfigEnv|TestSchedulerConfigJSONOverride|TestSchedulerConfigValidation|TestExtractSafeFeaturesSemanticDefaults|TestTaskFeatureProtoMapsSemanticAggregates|TestTrainingSampleCopiesSemanticAggregates|TestSchedulerTrainingSamplesInsertAndListByWindow|TestSchedulerTrainingSamplesListDefaultsLegacyAggregates|TestSchedulerTrainingSampleSchemaExcludesForbiddenFields|TestLoadArtifactReadsSemanticAggregateSupport|TestLoadArtifactDefaultsWithoutSemanticAggregateSupport|TestLoadArtifactRejectsUnknownSemanticFeature|TestUnsupportedArtifactIgnoresSemanticCoverage|TestSupportedArtifactCountsSemanticCoverage|TestScoreCalculatorIgnoresSemanticAggregates"`

### 2. SQLite Durable Training Sample Persistence
expected: Real SQLite migrations add semantic aggregate columns, persisted samples round-trip aggregate values, legacy rows return neutral defaults, and schema columns exclude raw prompt/payload/secret fields.
result: pass
evidence: same Go command, covering `internal/controlstate/sqlite` scheduler training sample tests.

### 3. Offline Export, Training, And Artifact Publishing
expected: Scheduler-training export fills bounded semantic defaults, rejects forbidden fields, training prepares semantic aggregate features, and publishing writes runtime manifests with semantic support metadata.
result: pass
evidence: `uv run --project tools/scheduler_training pytest tools/scheduler_training/tests/test_export_schema.py tools/scheduler_training/tests/test_train_publish.py tools/scheduler_training/tests/test_artifacts.py -q`

### 4. ONNX And Heuristic Runtime Compatibility
expected: ONNX artifacts only count semantic aggregate coverage when manifest metadata opts in, legacy artifacts stay neutral, unknown semantic fields are rejected, and heuristic scoring is invariant when only semantic aggregate fields change.
result: pass
evidence: same Go command, covering `internal/scheduler/onnx` and `internal/scheduler/heuristic` semantic aggregate tests.

### 5. Build And Schema Drift Gate
expected: The repository builds after phase 17 and GSD schema drift reports no blocking drift.
result: pass
evidence: `go build ./...`; `node .codex/gsd-core/bin/gsd-tools.cjs query verify.schema-drift 17`

### 6. Live PostgreSQL Migration And Repository Parity
expected: A real PostgreSQL database applies phase 17 migration `0008_scheduler_training_semantic_aggregates.sql`; scheduler training samples round-trip aggregate values and legacy rows return neutral defaults.
result: pass
evidence: `go test -count=1 -timeout 60s ./internal/controlstate/postgres -run "TestPostgresSchedulerTrainingSamples|TestPostgresRepositoryAccessorsNonNil"`; full phase check `go test -count=1 -timeout 60s ./internal/config ./internal/scheduler ./internal/controlstate/sqlite ./internal/controlstate/postgres ./internal/scheduler/onnx ./internal/scheduler/heuristic ./internal/app ./internal/observability ./internal/testenv`

### 7. Live Semantic-Neighbor Vector Collection
expected: With semantic neighbors explicitly enabled and real embedding/vector dependencies configured, Gateway indexes completed samples and enriches scheduler features from real vector search results without raw prompts, embeddings, payloads, or secrets crossing into Scheduler records.
result: skipped
reason: "Deferred per user instruction: expose/settle the live semantic-neighbor test configuration before the current milestone closes."
deferred_until: current milestone close
exposed_config: `SEMANTIC_CACHE_ENABLED`, `SEMANTIC_CACHE_PROVIDER`, `SEMANTIC_CACHE_VECTOR_STORE`, `SEMANTIC_CACHE_VECTOR_DIMENSION`, `QDRANT_ADDR`, `QDRANT_API_KEY`, `SCHEDULER_SEMANTIC_NEIGHBORS_ENABLED`, `SCHEDULER_SEMANTIC_NEIGHBORS_MIN_COUNT`, `SCHEDULER_SEMANTIC_NEIGHBORS_TASK_TIMEOUT`, `SCHEDULER_SEMANTIC_NEIGHBORS_BATCH_TIMEOUT`
evidence: `rg -n "SCHEDULER_SEMANTIC|SEMANTIC_CACHE|QDRANT|DEV_SERVER_IP|QDRANT_PORT" .env.example internal/config internal/testenv .planning/PROJECT.md`; `DEV_SERVER_IP` + `QDRANT_PORT` derives `QDRANT_ADDR` in `internal/testenv`, while semantic cache/vector enablement remains milestone-end config work.

## Summary

total: 7
passed: 6
issues: 0
pending: 0
skipped: 1
blocked: 0

## Gaps

None confirmed. Live semantic-neighbor vector UAT configuration is deferred to current milestone close.
