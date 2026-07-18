$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = Resolve-Path (Join-Path $ScriptDir "..\..")

Push-Location $Root
try {
  docker compose --env-file .env2.local up -d | Out-Host
} finally {
  Pop-Location
}

& (Join-Path $ScriptDir "run-cache-disabled.ps1")
& (Join-Path $ScriptDir "run-cache-enabled.ps1")
& (Join-Path $ScriptDir "run-provider-fallback.ps1")
& (Join-Path $ScriptDir "generate-html-report.ps1")
