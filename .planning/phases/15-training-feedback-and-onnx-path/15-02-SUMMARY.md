---
phase: 15-training-feedback-and-onnx-path
plan: 15-02
subsystem: tooling
tags: [scheduler, training, python, uv, onnx]
requires:
  - phase: 15-01
    provides: durable safe scheduler training samples
provides:
  - offline scheduler training Python package
  - safe export schema
  - P70 output-token train/evaluate commands
  - versioned ONNX runtime artifact publisher
affects: [scheduler-training, docs]
tech-stack:
  added: [uv, onnx, pytest]
  patterns: [offline tooling package, runtime artifact manifest]
key-files:
  created:
    - tools/scheduler_training/pyproject.toml
    - tools/scheduler_training/scheduler_training/export.py
    - tools/scheduler_training/scheduler_training/train.py
    - tools/scheduler_training/scheduler_training/evaluate.py
    - tools/scheduler_training/scheduler_training/artifacts.py
    - tools/scheduler_training/scheduler_training/publish.py
  modified:
    - README.md
key-decisions:
  - "Offline tooling uses uv and stays outside the Go gateway runtime."
  - "The first model target is p70_output_tokens."
  - "Published runtime artifacts contain only model.onnx and manifest.json."
patterns-established:
  - "Runtime artifact manifests include feature schema, training window, metrics, ONNX parity, checksum, and model parameters."
requirements-completed: [ML-01]
duration: 10min
completed: 2026-07-04
---

# Phase 15 Plan 02: Scheduler Training Tooling Summary

**uv-based offline scheduler tooling that trains a P70 output-token predictor and publishes checked ONNX artifacts**

## Performance

- **Duration:** 10 min
- **Started:** 2026-07-04T11:48:00-07:00
- **Completed:** 2026-07-04T11:57:43-07:00
- **Tasks:** 3
- **Files modified:** 15

## Accomplishments

- Created `tools/scheduler_training` as a self-contained Python package with a `scheduler-training` CLI.
- Added safe JSONL export sanitization and CSV writing for allowlisted scheduler sample fields.
- Added P70 output-token training/evaluation and a publisher that writes `model.onnx` plus `manifest.json`.

## Task Commits

1. **Task 1: Scaffold Python package and export schema** - `d504f72` (feat)
2. **Task 2: Train and evaluate the P70 output-token predictor** - `995e501` (feat)
3. **Task 3: Publish versioned ONNX artifact directories** - `78f9580` (feat)

## Files Created/Modified

- `tools/scheduler_training/pyproject.toml` - uv package, CLI, and dependencies.
- `tools/scheduler_training/scheduler_training/export.py` - safe export schema and sanitization.
- `tools/scheduler_training/scheduler_training/train.py` - P70 output-token predictor training.
- `tools/scheduler_training/scheduler_training/evaluate.py` - deterministic evaluation metrics.
- `tools/scheduler_training/scheduler_training/artifacts.py` - ONNX file generation, checksum, and manifest contract.
- `tools/scheduler_training/scheduler_training/publish.py` - versioned runtime artifact publisher.
- `tools/scheduler_training/tests/*.py` - export, train/evaluate, and publish coverage.

## Decisions Made

- Used real `onnx` package checks for the generated `model.onnx`.
- Kept training intentionally small and deterministic: P70 output tokens only, no latency or multi-output model in this phase.
- Added local `.gitignore` for `.venv` and pytest cache instead of broad root ignore churn.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

- `uv` needed access to its configured cache outside the workspace, so tests ran with approved elevated execution.

## User Setup Required

Use `uv` from `tools/scheduler_training` to run export, train, evaluate, and publish commands.

## Verification

- `uv run pytest`
- `rg "prompt|messages|authorization|api_key|secret|payload_hash|payload" tools/scheduler_training/tests` returned no matches.
- `rg "dataset|training.log" tools/scheduler_training/tests` returned no matches.

## Next Phase Readiness

Plan 15-03 can load the published `manifest.json` and `model.onnx` artifact directory, validate checksum/schema, and serve scheduler scores through the existing scheduler contract.

---
*Phase: 15-training-feedback-and-onnx-path*
*Completed: 2026-07-04*
