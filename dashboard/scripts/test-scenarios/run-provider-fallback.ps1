$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..\..")
. (Join-Path $ScriptDir "ScenarioLib.ps1")

$config = Read-ScenarioConfig -Path (Join-Path $ScriptDir "scenarios.json")
$bffBaseUrl = ([uri]$config.endpoints.bffHealth).GetLeftPart([System.UriPartial]::Authority)
$adminSession = New-AdminWebSession -BaseUrl $bffBaseUrl

$stackChecks = @(
  (Test-HttpEndpoint -Name "Gateway health" -Url $config.endpoints.gatewayHealth),
  (Test-HttpEndpoint -Name "BFF health" -Url $config.endpoints.bffHealth),
  (Test-HttpEndpoint -Name "Frontend dev server" -Url $config.endpoints.frontend)
)

$providerChecks = New-Object System.Collections.Generic.List[object]
$providerEndpoint = Test-HttpEndpoint -Name "Provider API" -Url $config.endpoints.providers -WebSession $adminSession
$providerChecks.Add($providerEndpoint)
if ($providerEndpoint.Passed) {
  try {
    $providerPayload = Invoke-RestMethod -Uri $config.endpoints.providers -TimeoutSec 10 -WebSession $adminSession
    $providerCount = @($providerPayload.providers).Count
    $healthyCount = @($providerPayload.providers | Where-Object { $_.status -eq "Healthy" }).Count
    $providerChecks.Add([pscustomobject]@{
      Name = "Healthy provider available"
      Passed = ($healthyCount -gt 0)
      StatusCode = 200
      Detail = "$healthyCount healthy provider(s) out of $providerCount"
    })
  } catch {
    $providerChecks.Add([pscustomobject]@{
      Name = "Provider payload parse"
      Passed = $false
      StatusCode = 0
      Detail = $_.Exception.Message
    })
  }
}

$benchmarkChecks = New-Object System.Collections.Generic.List[object]
$benchmarkEndpoint = Test-HttpEndpoint -Name "Benchmark API" -Url $config.endpoints.benchmarks -WebSession $adminSession
$benchmarkChecks.Add($benchmarkEndpoint)
if ($benchmarkEndpoint.Passed) {
  try {
    $benchmarkPayload = Invoke-RestMethod -Uri $config.endpoints.benchmarks -TimeoutSec 10 -WebSession $adminSession
    $benchmarkRows = @($benchmarkPayload.benchmarks).Count
    $source = [string]$benchmarkPayload.source
    $storageStatus = Get-BenchmarkStorageStatus -Payload $benchmarkPayload
    $benchmarkChecks.Add([pscustomobject]@{
      Name = "Benchmark rows"
      Passed = ($benchmarkRows -gt 0)
      StatusCode = 200
      Detail = "$benchmarkRows row(s) from $source"
    })
    $benchmarkChecks.Add([pscustomobject]@{
      Name = "Redis benchmark store"
      Passed = $storageStatus.RedisConnected
      StatusCode = 200
      Detail = "$($storageStatus.RedisStatus) - $($storageStatus.RedisDetail)"
    })
    $benchmarkChecks.Add([pscustomobject]@{
      Name = "Qdrant benchmark store"
      Passed = $storageStatus.QdrantConnected
      StatusCode = 200
      Detail = "$($storageStatus.QdrantStatus) - $($storageStatus.QdrantDetail)"
    })
  } catch {
    $benchmarkChecks.Add([pscustomobject]@{
      Name = "Benchmark payload parse"
      Passed = $false
      StatusCode = 0
      Detail = $_.Exception.Message
    })
  }
}

$results = @(
  (New-ReadinessScenario -Name "Local Stack Health" -Checks $stackChecks -Notes "Gateway, BFF, and frontend endpoints must respond locally."),
  (New-ReadinessScenario -Name "Provider Fallback Readiness" -Checks $providerChecks.ToArray() -Notes "Provider list is reachable and at least one healthy provider can receive fallback traffic."),
  (New-ReadinessScenario -Name "Benchmark Store Availability" -Checks $benchmarkChecks.ToArray() -Notes "Dashboard benchmark rows are available with Redis and Qdrant readiness signals.")
)

$output = Join-Path $Root "tmp\test-scenarios\provider-fallback.json"
Write-ScenarioJson -Data $results -Path $output
Write-Host "Wrote $output"
