param(
  [string[]]$Dataset = @(),
  [string]$GatewayUrl = "http://127.0.0.1:18080",
  [string]$EnvFile = "",
  [string]$Model = "oc/deepseek-v4-flash-free",
  [string]$Provider = "openai-compatible",
  [string]$Method = "Our Gateway Method",
  [string]$GatewayVersion = "VeloxMesh",
  [int]$Concurrency = 1,
  [double]$RequestRate = 0,
  [int]$TimeoutSeconds = 120,
  [string]$ReportDir = "",
  [string]$RedisAddr = "127.0.0.1:6379"
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$DashboardRoot = Resolve-Path (Join-Path $ScriptDir "..\..")
$WorkspaceRoot = Resolve-Path (Join-Path $DashboardRoot "..")
$VeloxMeshRoot = Join-Path $WorkspaceRoot "VeloxMesh"
$Runner = Join-Path $VeloxMeshRoot "scripts\run-gateway-dataset.py"
$Publisher = Join-Path $VeloxMeshRoot "scripts\publish-benchmark-results.py"

if ($Dataset.Count -eq 0) {
  $Dataset = @(
    (Join-Path $VeloxMeshRoot "testdata\full-benchmark-work\step3_jsonl\mmlu_5.jsonl"),
    (Join-Path $VeloxMeshRoot "testdata\full-benchmark-work\step3_jsonl\lmsys_5.jsonl")
  )
}
if (-not $EnvFile) {
  $EnvFile = Join-Path $VeloxMeshRoot "env\veloxmesh.env"
}
if (-not $ReportDir) {
  $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
  $ReportDir = Join-Path $VeloxMeshRoot "reports\dashboard-e2e-$stamp"
}
if (-not (Test-Path -LiteralPath $Runner)) {
  throw "Gateway dataset runner not found: $Runner"
}
if (-not (Test-Path -LiteralPath $Publisher)) {
  throw "Benchmark publisher not found: $Publisher"
}

$reportRoots = New-Object System.Collections.Generic.List[string]
$hadRunFailures = $false
$runStamp = Get-Date -Format "yyyyMMddTHHmmss"

foreach ($datasetPath in $Dataset) {
  if (-not (Test-Path -LiteralPath $datasetPath)) {
    throw "Dataset not found: $datasetPath"
  }

  $datasetName = [System.IO.Path]::GetFileNameWithoutExtension($datasetPath)
  $datasetReportDir = if ($Dataset.Count -eq 1) { $ReportDir } else { Join-Path $ReportDir $datasetName }
  $reportRoots.Add($datasetReportDir)
  $runId = "$runStamp-$datasetName"

  Write-Host "Running real gateway benchmark through $GatewayUrl with $datasetName"
  $runnerArgs = @(
    $Runner,
    "--dataset", $datasetPath,
    "--report-dir", $datasetReportDir,
    "--env-file", $EnvFile,
    "--gateway-url", $GatewayUrl,
    "--model", $Model,
    "--provider", $Provider,
    "--method", $Method,
    "--gateway-version", $GatewayVersion,
    "--concurrency", $Concurrency,
    "--timeout-seconds", $TimeoutSeconds,
    "--run-id", $runId
  )
  if ($RequestRate -gt 0) {
    $runnerArgs += @("--request-rate", $RequestRate)
  }
  python @runnerArgs
  if ($LASTEXITCODE -ne 0) {
    $hadRunFailures = $true
    Write-Warning "$datasetName completed with failed or invalid model responses; publishing the real failure state."
  }

  foreach ($requiredName in @("summary.json", "latency.csv", "responses.jsonl", "run_config.json")) {
    if (-not (Test-Path -LiteralPath (Join-Path $datasetReportDir $requiredName))) {
      throw "Gateway run did not produce $requiredName in $datasetReportDir"
    }
  }
}

$snapshotPath = Join-Path $ReportDir "dashboard-benchmark-snapshot.json"
$publisherArgs = @($Publisher)
foreach ($reportRoot in $reportRoots) {
  $publisherArgs += @("--report-dir", $reportRoot)
}
$publisherArgs += @(
  "--redis-addr", $RedisAddr,
  "--snapshot-output", $snapshotPath
)
python @publisherArgs
if ($LASTEXITCODE -ne 0) {
  throw "Benchmark publication failed with exit code $LASTEXITCODE"
}

Write-Host "Published real gateway benchmarks to Redis key veloxmesh:benchmarks"
Write-Host "Dashboard endpoint: http://127.0.0.1:8080/bff/admin/benchmarks"
Write-Host "Control panel: http://127.0.0.1:5173/"
Write-Host "Snapshot: $snapshotPath"

if ($hadRunFailures) {
  exit 1
}
