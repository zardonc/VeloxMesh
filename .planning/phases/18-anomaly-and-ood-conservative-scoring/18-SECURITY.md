---
phase: 18
slug: anomaly-and-ood-conservative-scoring
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-05
---

# Phase 18 - Security

Per-phase security contract: threat register, accepted risks, and audit trail.

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Control-state samples -> offline tooling | Safe scheduler training samples are exported into Python training/publish tooling. | Bounded scalar scheduler fields, completion labels, sanitized enum coverage. |
| Offline artifact -> ONNX runtime | Python publishes manifest metadata consumed by Go scheduler startup. | `manifest.json`, model checksum, anomaly thresholds/evidence. |
| Scheduler RPC caller -> ONNX scorer | Caller-provided TaskFeature values influence anomaly distance. | Existing bounded TaskFeature scalar/enum fields only. |
| ONNX scorer -> metrics/status | Runtime anomaly state leaves process for operations. | Low-cardinality status/reason labels only. |
| Quality recorder -> control-state rollups | Runtime metadata becomes durable quality evidence. | Scheduler version/type, task type, coverage level, anomaly counts/rates. |
| Provider secret storage -> runtime provider reload | Encrypted provider secret material is decrypted for active provider config. | Ciphertext, nonce, key ID; plaintext stays in process memory only. |

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-18-01-01 | Information Disclosure | tools/scheduler_training | mitigate | `export.py` rejects forbidden prompt/payload/auth/API-key/secret fields and writes only `SAFE_FIELDS`; publish tests assert runtime artifacts contain only `manifest.json` and `model.onnx`. Evidence: `tools/scheduler_training/scheduler_training/export.py`, `tools/scheduler_training/tests/test_export_schema.py`, `tools/scheduler_training/tests/test_train_publish.py`. | closed |
| T-18-01-02 | Tampering | manifest.json | mitigate | Manifest validation keeps schema/target/parity/checksum checks and adds typed anomaly metadata; invalid anomaly threshold structure degrades anomaly behavior instead of trusting bad metadata. Evidence: `internal/scheduler/onnx/artifact.go`, `internal/scheduler/onnx/artifact_test.go`. | closed |
| T-18-01-03 | Denial of Service | sparse threshold groups | mitigate | `ANOMALY_MIN_SAMPLES=20`; sparse task types increment unavailable evidence and publish no guessed threshold. Evidence: `tools/scheduler_training/scheduler_training/train.py`, `tools/scheduler_training/tests/test_train_publish.py`. | closed |
| T-18-02-01 | Tampering | internal/scheduler/onnx/artifact.go | mitigate | `validateAnomalyThresholds` enforces supported coverage labels and positive threshold/sample counts; invalid metadata sets `degraded/invalid_metadata` while core ONNX validation remains strict. Evidence: `internal/scheduler/onnx/artifact.go`, `internal/scheduler/onnx/artifact_test.go`. | closed |
| T-18-02-02 | Denial of Service | internal/scheduler/onnx/scorer.go | mitigate | OOD confidence floor is `0.05` and uncertainty contribution is capped with `math.Min(severity, 5)`. Evidence: `internal/scheduler/onnx/scorer.go`, `internal/scheduler/onnx/scorer_test.go`. | closed |
| T-18-02-03 | Information Disclosure | metrics/status/logs | mitigate | Status and reason values are bounded enums; Prometheus recorders sanitize scheduler type, task type, coverage level, and anomaly status before label emission. Evidence: `cmd/scheduler/main.go`, `internal/observability/prometheus.go`, `internal/observability/prometheus_test.go`. | closed |
| T-18-02-04 | Spoofing | scheduler RPC callers | mitigate | No new request-controllable RPC fields were added; severity is derived from existing bounded TaskFeature fields plus artifact thresholds. Evidence: `internal/scheduler/types.go`, `internal/scheduler/onnx/scorer.go`, `proto/scheduler/v1/scheduler.proto`. | closed |
| T-18-03-01 | Information Disclosure | internal/observability/prometheus.go | mitigate | Live metric labels are restricted to scheduler type/version, task type, coverage level, and anomaly status; task type, coverage, and anomaly status are closed enum sanitizers. Evidence: `internal/observability/prometheus.go`, `internal/observability/prometheus_test.go`. | closed |
| T-18-03-02 | Tampering | control-state migrations | mitigate | SQLite and PostgreSQL migrations add anomaly columns and coverage-level keying; repository tests cover round trips. Evidence: `internal/controlstate/migrations/sqlite/0010_scheduler_quality_anomaly.sql`, `internal/controlstate/migrations/postgres/0009_scheduler_quality_anomaly.sql`, `internal/controlstate/sqlite/scheduler_quality_rollups_test.go`, `internal/controlstate/postgres/scheduler_quality_rollups_test.go`. | closed |
| T-18-03-03 | Repudiation | quality rollups | mitigate | Rollups store `anomaly_count`, `anomaly_rate`, and `anomaly_unavailable_count`; degraded/unavailable anomaly states do not increment scheduler error/fallback counters. Evidence: `internal/controlstate/types.go`, `internal/controlstate/scheduler_quality_rollup.go`, `internal/scheduler/quality.go`, `internal/scheduler/quality_test.go`. | closed |
| T-18-03-04 | Denial of Service | metrics cardinality | mitigate | `safeCoverageLevel` and `safeAnomalyStatus` use closed allowlists, with unknown values mapped to safe defaults; `model_class` is not a live metric label. Evidence: `internal/observability/prometheus.go`, `internal/observability/prometheus_test.go`. | closed |
| T-18-VFY-01 | Tampering | provider secret decryption | mitigate | Verify-work found malformed nonce state in real Plan4 smoke; `DecryptProviderSecret` now rejects invalid nonce length before `GCM.Open`, and Plan4 smoke uses an isolated Postgres schema. Evidence: `internal/controlstate/secrets.go`, `internal/controlstate/controlstate_test.go`, `tests/integration/plan4_postgres_smoke_test.go`, `18-UAT.md`. | closed |

## Accepted Risks Log

No accepted risks.

## Unregistered Flags

No unregistered threat flags were found in Phase 18 summaries.

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-05 | 12 | 12 | 0 | Codex gsd-secure-phase |
| 2026-07-05 | 13 | 13 | 0 | Codex verify-work hardening follow-up |

## Verification Evidence

- `uv run pytest -v tests/test_train_publish.py tests/test_export_schema.py` passed from `tools/scheduler_training`.
- `go test -v -timeout 60s -count=1 ./internal/scheduler/onnx ./internal/scheduler ./internal/observability ./internal/controlstate -run 'TestLoadArtifact.*Anomaly|TestAnomaly|TestMissingAnomaly|TestPredictionQualityRecorderRecordsUnavailableAnomalySeparately|TestPrometheusSchedulerAnomalyStatusLabelsAreBounded|TestSecretCipherRejectsInvalidNonce'` passed.
- `go test -v -timeout 60s -count=1 ./tests/integration -run 'TestRedisHotState|TestPlan4Postgres'` passed with real Redis, Postgres, and configured provider path.
- `go test -timeout 60s -count=1 ./...` passed.
- `go build ./...` passed.

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-05

