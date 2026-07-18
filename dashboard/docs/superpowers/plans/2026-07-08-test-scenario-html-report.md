# Test Scenario HTML Report Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build local PowerShell scripts that run cache/no-cache test scenarios and generate a standalone HTML report for the VeloxMesh AI Gateway dashboard.

**Architecture:** Keep all new behavior in `dashboard/scripts/test-scenarios` with one shared library and small scenario runners. Use deterministic local simulation for cache comparison and real local HTTP checks for BFF/frontend/benchmark readiness. Store generated JSON artifacts under `dashboard/tmp/test-scenarios` and the final report under root `outputs/`.

**Tech Stack:** PowerShell 5+, Docker Compose CLI, local BFF HTTP endpoints, JSON, standalone HTML/CSS.

---

## Update: A/B Paired Report Structure

The report must organize primary test scenarios as paired comparisons, not as a flat list. Readiness checks move into an environment section. A/B pairs include:

- Cache Off vs Cache On
- Cold Cache vs Warm Cache
- FCFS Scheduler vs LTR Scheduler
- Baseline Model/Gateway vs Improved Model/Gateway, pending teammate integration
- Normal Provider vs Degraded Provider, pending gateway degradation simulation

Implementation should add tests first for paired report text and comparison artifacts, then update `ScenarioLib.ps1`, `generate-html-report.ps1`, and `run-all.ps1` without changing existing app behavior.

## Chunk 1: Scenario Library and Tests

### Task 1: Create failing tests for scenario math and report generation

**Files:**
- Create: `dashboard/tests/test-scenarios/scenario-lib.tests.ps1`
- Create: `dashboard/scripts/test-scenarios/ScenarioLib.ps1`

- [ ] **Step 1: Write the failing test**

Create tests that dot-source `ScenarioLib.ps1` and assert these functions exist and work:

```powershell
Assert-Equal 50 (Get-Percentile -Values @(10, 50, 100) -Percentile 50) "p50"
Assert-Equal 100 (Get-Percentile -Values @(10, 50, 100) -Percentile 95) "p95 rounds up"
$noCache = Invoke-CacheScenario -Name "No Cache" -Prompts @("a", "a") -CacheEnabled:$false
$cache = Invoke-CacheScenario -Name "Cache Enabled" -Prompts @("a", "a") -CacheEnabled:$true
Assert-Equal 0 $noCache.CacheHits "no-cache hit count"
Assert-Equal 1 $cache.CacheHits "cache hit count"
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tests\test-scenarios\scenario-lib.tests.ps1
```

Expected: FAIL because functions are not implemented.

- [ ] **Step 3: Write minimal implementation**

Implement `ScenarioLib.ps1` with:

- `Get-Percentile`
- `Invoke-CacheScenario`
- `Test-HttpEndpoint`
- `Write-ScenarioJson`
- `New-TestScenarioHtmlReport`

- [ ] **Step 4: Run test to verify it passes**

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tests\test-scenarios\scenario-lib.tests.ps1
```

Expected: PASS.

## Chunk 2: Scenario Runners

### Task 2: Add scenario config and runners

**Files:**
- Create: `dashboard/scripts/test-scenarios/scenarios.json`
- Create: `dashboard/scripts/test-scenarios/run-cache-disabled.ps1`
- Create: `dashboard/scripts/test-scenarios/run-cache-enabled.ps1`
- Create: `dashboard/scripts/test-scenarios/run-provider-fallback.ps1`
- Create: `dashboard/scripts/test-scenarios/generate-html-report.ps1`
- Create: `dashboard/scripts/test-scenarios/run-all.ps1`

- [ ] **Step 1: Write failing integration expectations**

Extend `scenario-lib.tests.ps1` to assert `run-all.ps1` exists and can produce a report when given local deterministic data.

- [ ] **Step 2: Run test to verify it fails**

Run the same PowerShell test file. Expected: FAIL because runner scripts do not exist.

- [ ] **Step 3: Add minimal runners**

Implement runners so:

- `run-cache-disabled.ps1` writes `dashboard/tmp/test-scenarios/cache-disabled.json`
- `run-cache-enabled.ps1` writes `dashboard/tmp/test-scenarios/cache-enabled.json`
- `run-provider-fallback.ps1` writes `dashboard/tmp/test-scenarios/provider-fallback.json`
- `generate-html-report.ps1` writes `outputs/ai-gateway-test-report.html`
- `run-all.ps1` runs all of the above

- [ ] **Step 4: Run test to verify it passes**

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tests\test-scenarios\scenario-lib.tests.ps1
```

Expected: PASS.

## Chunk 3: Verification

### Task 3: Generate real local report

**Files:**
- Generated: `dashboard/tmp/test-scenarios/*.json`
- Generated: `outputs/ai-gateway-test-report.html`

- [ ] **Step 1: Verify local stack is running**

Run:

```powershell
docker compose --env-file .env2.local ps
```

Expected: Redis, Qdrant, Prometheus, OTel, and RedisInsight are running.

- [ ] **Step 2: Run all scenarios**

Run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-scenarios\run-all.ps1
```

Expected: JSON artifacts and HTML report are generated.

- [ ] **Step 3: Verify existing tests**

Run:

```powershell
go test ./...
pnpm test
pnpm build
```

Expected: all pass.

- [ ] **Step 4: Inspect report**

Open or read `outputs/ai-gateway-test-report.html` and confirm it includes:

- Baseline No Cache
- Cache Enabled
- Cold Cache vs Warm Cache
- Provider Fallback Readiness
- Benchmark Store Availability
- Local Stack Health
