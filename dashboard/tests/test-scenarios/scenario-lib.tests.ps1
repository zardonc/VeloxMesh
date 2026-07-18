$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$Library = Join-Path $Root "scripts\test-scenarios\ScenarioLib.ps1"
$RunAll = Join-Path $Root "scripts\test-scenarios\run-all.ps1"

. $Library

function Assert-Equal {
  param(
    [object]$Expected,
    [object]$Actual,
    [string]$Message
  )

  if ($Expected -ne $Actual) {
    throw "$Message expected <$Expected> but got <$Actual>"
  }
}

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )

  if (-not $Condition) {
    throw $Message
  }
}

Assert-Equal 50 (Get-Percentile -Values @(10, 50, 100) -Percentile 50) "p50"
Assert-Equal 100 (Get-Percentile -Values @(10, 50, 100) -Percentile 95) "p95 rounds up"

$noCache = Invoke-CacheScenario -Name "No Cache" -Prompts @("a", "a") -CacheEnabled:$false
$cache = Invoke-CacheScenario -Name "Cache Enabled" -Prompts @("a", "a") -CacheEnabled:$true

Assert-Equal 0 $noCache.CacheHits "no-cache hit count"
Assert-Equal 1 $cache.CacheHits "cache hit count"
Assert-True ($cache.AverageLatencyMs -lt $noCache.AverageLatencyMs) "cache should lower average latency"

$cachePair = New-ComparisonPair -Name "Cache Off vs Cache On" -Baseline $noCache -Treatment $cache -Metric "Average latency"
Assert-Equal "Cache Off vs Cache On" $cachePair.Name "comparison pair name"
Assert-Equal "No Cache" $cachePair.BaselineName "comparison baseline"
Assert-Equal "Cache Enabled" $cachePair.TreatmentName "comparison treatment"
Assert-True ($cachePair.ImprovementPercent -gt 0) "cache pair should show improvement"

$tmpDir = Join-Path $Root "tmp\test-scenarios\unit"
$jsonPath = Join-Path $tmpDir "cache-enabled.json"
Write-ScenarioJson -Data $cache -Path $jsonPath
Assert-True (Test-Path $jsonPath) "scenario json should be written"

$readiness = New-ReadinessScenario -Name "Unit Readiness" -Checks @(
  [pscustomobject]@{ Name = "Gateway health"; Passed = $true; StatusCode = 200; Detail = "HTTP 200" }
) -Notes "Unit readiness notes."
$reportPath = Join-Path $tmpDir "report.html"
New-TestScenarioHtmlReport -ScenarioResults @($noCache, $cache) -ComparisonPairs @($cachePair) -EnvironmentChecks @($readiness) -OutputPath $reportPath -Title "Unit Report"
$report = Get-Content -LiteralPath $reportPath -Raw
Assert-True ($report.Contains("Unit Report")) "report should include title"
Assert-True ($report.Contains("Environment Readiness")) "report should include readiness section"
Assert-True ($report.Contains("Gateway health")) "report should include readiness check details"
Assert-True ($report.Contains("A/B Test Results")) "report should include paired test section"
Assert-True ($report.Contains("Cache Off vs Cache On")) "report should include pair name"
Assert-True ($report.Contains("No Cache")) "report should include baseline name"
Assert-True ($report.Contains("Cache Enabled")) "report should include treatment name"

$benchmarkPayload = [pscustomobject]@{
  source = "redis"
  storage = [pscustomobject]@{
    redis = [pscustomobject]@{ status = "connected"; detail = "loaded veloxmesh:benchmarks" }
    qdrant = [pscustomobject]@{ status = "connected"; detail = "collection ready" }
  }
}
$storageStatus = Get-BenchmarkStorageStatus -Payload $benchmarkPayload
Assert-Equal "connected" $storageStatus.RedisStatus "redis nested status"
Assert-Equal "connected" $storageStatus.QdrantStatus "qdrant nested status"
Assert-True $storageStatus.RedisConnected "redis should be connected"
Assert-True $storageStatus.QdrantConnected "qdrant should be connected"

$nestedItems = ConvertTo-ObjectArray -InputObject @(,@(
  [pscustomobject]@{ Name = "First" },
  [pscustomobject]@{ Name = "Second" }
))
Assert-Equal 2 $nestedItems.Count "nested JSON array should be flattened"
Assert-Equal "First" $nestedItems[0].Name "first flattened item"
Assert-Equal "Second" $nestedItems[1].Name "second flattened item"

Assert-True (Test-Path $RunAll) "run-all script should exist"

$providerFallbackScript = Get-Content -LiteralPath (Join-Path $Root "scripts\test-scenarios\run-provider-fallback.ps1") -Raw
Assert-True ($providerFallbackScript.Contains("New-AdminWebSession")) "provider fallback script should authenticate before admin API checks"

Write-Host "scenario-lib tests passed"
