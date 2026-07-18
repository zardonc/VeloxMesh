# Request-Level Benchmark and Improved Model Implementation Plan

> Execute this plan test-first. Preserve all previous uncommitted work and stop after Step 7 verification.

## Task 1: Request Store and report package

- Add failing BFF tests for canonical rows, raw CSV, ZIP entries, recomputed summaries, no-data behavior, and Admin authorization.
- Add an injectable request-level Benchmark Store and Redis implementation.
- Build raw CSV and the complete ZIP from allowlisted request fields.

## Task 2: Dashboard export integration

- Add failing frontend tests for BFF raw CSV and ZIP download endpoints.
- Replace client-generated aggregate exports in the Benchmarks page with authenticated BFF downloads.
- Preserve loading, failure, and retry-safe UI behavior.

## Task 3: Request-level runner and publisher

- Add failing Python unit tests for method IDs, Gateway-only URL usage, request ID/header capture, row count, recalculation, Redis document shape, and secret exclusion.
- Implement a standard-library runner that emits canonical request records.
- Implement a publisher that merges run directories and writes both Redis snapshots.
- Update the real-flow PowerShell entry point to use repository-local scripts.

## Task 4: Improved model registration

- Add a redacted environment contract.
- Add a PowerShell registration/verification command that creates an OpenAI-compatible Provider through the Gateway Admin API and verifies models, health, and a minimal Gateway chat request.
- Require provider ID, model ID, and model version; never persist the API key in artifacts.

## Task 5: Verification

- Run dashboard and related root Go tests.
- Run Python unit tests.
- Run frontend unit tests, production build, and Playwright.
- Inspect ZIP entries and recompute a summary from raw CSV.
- Run sensitive-data scans.
- Record the real improved-model endpoint as pending when its contract has not been supplied.

