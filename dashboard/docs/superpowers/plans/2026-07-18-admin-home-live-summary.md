# Admin Home Live Summary Implementation Plan

> Execute this plan test-first and stop after Step 6 verification. Do not begin the next project step.

## Task 1: Lock the BFF contract with failing tests

- Add tests for complete, partial, and unavailable summary responses.
- Add calculation tests for today filtering, outcome rates, P95, and queue depth.
- Verify response metrics originate from injected stores and Gateway endpoints.

## Task 2: Implement live BFF aggregation

- Add a focused summary aggregator beside the existing BFF server.
- Query independent sources concurrently.
- Return nullable fields, source metadata, warnings, and timestamps.
- Keep demo output behind explicit demo mode only.

## Task 3: Lock frontend behavior with failing tests

- Test exact BFF-to-view-model consistency.
- Test unavailable formatting and source metadata.
- Test that production data loading never falls back to fixed Admin Home values.

## Task 4: Implement Admin Home rendering

- Replace `getAdminOverview` fixed values with `/bff/admin/summary`.
- Render Partial/Error states, source labels, generated time, and nullable metrics.
- Keep the existing Refresh action wired to a fresh BFF request.

## Task 5: Verify Step 6

- Run focused and full dashboard Go tests.
- Run frontend unit tests and production build.
- Run Playwright acceptance tests.
- Inspect production code for forbidden fixed Admin Home metrics.
- Record remaining external integration limitations without starting Step 7.

