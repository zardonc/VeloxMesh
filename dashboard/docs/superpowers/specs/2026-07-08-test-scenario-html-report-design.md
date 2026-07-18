# Test Scenario HTML Report Design

## Goal

Create runnable local test-scenario scripts for the VeloxMesh AI Gateway dashboard and generate a standalone HTML report that compares cache-disabled and cache-enabled behavior, plus dependency and benchmark readiness.

## Scope

The report is a local development and demonstration artifact. It should not call external LLM providers by default. Instead, it measures the local BFF, Docker dependencies, Redis benchmark data, and deterministic cache simulation so the report can be regenerated on the user's machine without spending API quota.

## A/B Test Scenarios

Primary test scenarios are paired comparisons. Readiness checks are prerequisites and should not be counted as optimization test scenarios.

1. Cache Off vs Cache On
   - A: repeated prompt processing without cache reuse.
   - B: the same prompt set with local cache enabled.
   - Compare request count, success rate, average latency, P50, P95, cache hits, and cache hit rate.

2. Cold Cache vs Warm Cache
   - A: first cache-enabled pass before cache is populated.
   - B: second pass after repeated prompts are cached.
   - Compare latency reduction from cache warmup.

3. FCFS Scheduler vs LTR Scheduler
   - A: first-come-first-served scheduling from the benchmark table.
   - B: latency-aware / learning-to-rank scheduling from the benchmark table.
   - Compare mean latency, P95 latency, throughput, and latency reduction.

4. Baseline Model/Gateway vs Improved Model/Gateway
   - A: original model or original gateway behavior.
   - B: teammate-integrated improved model or gateway behavior.
   - Mark pending until those modules are merged.

5. Normal Provider vs Degraded Provider
   - A: healthy provider path.
   - B: slow or unavailable provider path.
   - Mark pending until the gateway exposes a safe degraded-provider simulation path.

## Environment Readiness

These checks appear above the A/B results:

- Local Stack Health: Gateway, BFF, and frontend respond locally.
- Provider Readiness: provider API is reachable and at least one provider is healthy.
- Benchmark Store Availability: `/bff/admin/benchmarks` exposes benchmark rows and Redis/Qdrant readiness.

## Files

- `dashboard/scripts/test-scenarios/ScenarioLib.ps1`
  - Shared PowerShell functions for percentile math, scenario execution, endpoint checks, JSON writing, and HTML rendering.

- `dashboard/scripts/test-scenarios/run-cache-disabled.ps1`
  - Runs only the no-cache scenario and writes JSON.

- `dashboard/scripts/test-scenarios/run-cache-enabled.ps1`
  - Runs only the cache-enabled scenario and writes JSON.

- `dashboard/scripts/test-scenarios/run-provider-fallback.ps1`
  - Runs provider and benchmark readiness checks.

- `dashboard/scripts/test-scenarios/generate-html-report.ps1`
  - Converts scenario JSON into the standalone HTML report.

- `dashboard/scripts/test-scenarios/run-all.ps1`
  - Starts from the current local stack, runs all scenarios, writes JSON artifacts, and generates the HTML report.

- `dashboard/scripts/test-scenarios/scenarios.json`
  - Prompt set and scenario configuration.

- `dashboard/tests/test-scenarios/scenario-lib.tests.ps1`
  - Lightweight script tests for scenario math, cache behavior, JSON output, and report generation.

- `outputs/ai-gateway-test-report.html`
  - Generated final report.

## Report Layout

The HTML report will include:

- Executive summary with overall readiness.
- Environment checks for Docker Compose, BFF, frontend, Redis/Qdrant benchmark status.
- A/B Test Results with paired comparison cards.
- A/B comparison table showing baseline value, treatment value, and delta.
- Pending integration pairs for teammate-owned gateway/model scenarios.
- Raw JSON artifact links or file paths for traceability.

## Constraints

- Keep scripts runnable from Windows PowerShell.
- Do not require Pester or new global dependencies.
- Do not send real provider requests unless a future script explicitly adds that mode.
- Do not modify existing app behavior.
- Do not commit automatically.
