$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..\..")
. (Join-Path $ScriptDir "ScenarioLib.ps1")

$config = Read-ScenarioConfig -Path (Join-Path $ScriptDir "scenarios.json")
$result = Invoke-CacheScenario `
  -Name "Baseline No Cache" `
  -Prompts @($config.prompts) `
  -MissLatencyMs $config.missLatencyMs `
  -HitLatencyMs $config.hitLatencyMs

$output = Join-Path $Root "tmp\test-scenarios\cache-disabled.json"
Write-ScenarioJson -Data $result -Path $output
Write-Host "Wrote $output"
