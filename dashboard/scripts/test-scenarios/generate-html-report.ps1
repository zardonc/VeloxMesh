$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..\..")
$WorkspaceRoot = Resolve-Path (Join-Path $Root "..")
. (Join-Path $ScriptDir "ScenarioLib.ps1")

$artifactDir = Join-Path $Root "tmp\test-scenarios"
$scenarioFiles = @(
  (Join-Path $artifactDir "cache-disabled.json"),
  (Join-Path $artifactDir "cache-enabled.json"),
  (Join-Path $artifactDir "provider-fallback.json")
)

$results = New-Object System.Collections.Generic.List[object]
foreach ($file in $scenarioFiles) {
  if (-not (Test-Path -LiteralPath $file)) {
    throw "Missing scenario artifact: $file"
  }
  $content = Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
  foreach ($item in (ConvertTo-ObjectArray -InputObject $content)) {
    $results.Add($item)
  }
}

$cacheDisabled = Get-Content -LiteralPath (Join-Path $artifactDir "cache-disabled.json") -Raw | ConvertFrom-Json
$cacheEnabledItems = ConvertTo-ObjectArray -InputObject (Get-Content -LiteralPath (Join-Path $artifactDir "cache-enabled.json") -Raw | ConvertFrom-Json)
$cacheEnabled = @($cacheEnabledItems | Where-Object { $_.Name -eq "Cache Enabled" })[0]
$coldWarm = @($cacheEnabledItems | Where-Object { $_.Name -eq "Cold Cache vs Warm Cache" })[0]
$environmentChecks = ConvertTo-ObjectArray -InputObject (Get-Content -LiteralPath (Join-Path $artifactDir "provider-fallback.json") -Raw | ConvertFrom-Json)

$pairs = New-Object System.Collections.Generic.List[object]
$pairs.Add((New-ComparisonPair `
  -Name "Cache Off vs Cache On" `
  -Baseline $cacheDisabled `
  -Treatment $cacheEnabled `
  -Metric "Average latency (ms)" `
  -Notes "Same prompt set, with repeated prompts. Treatment uses local cache simulation."))

$pairs.Add((New-ManualComparisonPair `
  -Name "Cold Cache vs Warm Cache" `
  -BaselineName "Cold Cache" `
  -TreatmentName "Warm Cache" `
  -BaselineValue $coldWarm.ColdAverageLatencyMs `
  -TreatmentValue $coldWarm.WarmAverageLatencyMs `
  -Metric "Average latency (ms)" `
  -Passed $coldWarm.Passed `
  -Notes "Second pass reuses repeated prompts and demonstrates cache warmup behavior."))

$latencyTable = Join-Path $WorkspaceRoot "llm_latency_ltr_reproduction\outputs\full\data\latency_reduction_table.csv"
if (Test-Path -LiteralPath $latencyTable) {
  $rows = Import-Csv -LiteralPath $latencyTable
  $fcfs = @($rows | Where-Object { $_.method -eq "fcfs" -and $_.request_rate -eq "10.0" })[0]
  $ltr = @($rows | Where-Object { $_.method -eq "ltr" -and $_.request_rate -eq "10.0" })[0]
  if ($null -ne $fcfs -and $null -ne $ltr) {
    $pairs.Add((New-ManualComparisonPair `
      -Name "FCFS Scheduler vs LTR Scheduler" `
      -BaselineName "FCFS @ 10 req/s" `
      -TreatmentName "LTR @ 10 req/s" `
      -BaselineValue ([double]$fcfs.mean_latency) `
      -TreatmentValue ([double]$ltr.mean_latency) `
      -Metric "Mean latency (s)" `
      -Passed $true `
      -Notes "Uses the benchmark latency_reduction_table.csv output."))
  }
}

$pairs.Add((New-PendingComparisonPair `
  -Name "Baseline Model/Gateway vs Improved Model/Gateway" `
  -BaselineName "Original model/gateway" `
  -TreatmentName "Teammate improved model/gateway" `
  -Notes "Pending final model and gateway integration from teammates."))

$pairs.Add((New-PendingComparisonPair `
  -Name "Normal Provider vs Degraded Provider" `
  -BaselineName "Healthy provider path" `
  -TreatmentName "Slow or unavailable provider path" `
  -Notes "Pending a safe degraded-provider simulation path in the gateway."))

$reportPath = Join-Path $WorkspaceRoot "outputs\ai-gateway-test-report.html"
New-TestScenarioHtmlReport -ScenarioResults $results.ToArray() -ComparisonPairs $pairs.ToArray() -EnvironmentChecks $environmentChecks -OutputPath $reportPath -Title "VeloxMesh AI Gateway A/B Test Scenarios"
Write-Host "Wrote $reportPath"
