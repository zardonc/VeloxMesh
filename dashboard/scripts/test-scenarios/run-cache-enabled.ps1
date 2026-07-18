$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..\..")
. (Join-Path $ScriptDir "ScenarioLib.ps1")

$config = Read-ScenarioConfig -Path (Join-Path $ScriptDir "scenarios.json")
$cacheEnabled = Invoke-CacheScenario `
  -Name "Cache Enabled" `
  -Prompts @($config.prompts) `
  -CacheEnabled `
  -MissLatencyMs $config.missLatencyMs `
  -HitLatencyMs $config.hitLatencyMs

$coldWarm = Invoke-ColdWarmCacheScenario `
  -Prompts @($config.prompts) `
  -MissLatencyMs $config.missLatencyMs `
  -HitLatencyMs $config.hitLatencyMs

$output = Join-Path $Root "tmp\test-scenarios\cache-enabled.json"
Write-ScenarioJson -Data @($cacheEnabled, $coldWarm) -Path $output
Write-Host "Wrote $output"
