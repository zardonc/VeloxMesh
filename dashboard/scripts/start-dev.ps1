$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$Frontend = Join-Path $Root "web\admin-console"
$Tmp = Join-Path $Root "tmp"
$GoExe = Join-Path $env:USERPROFILE "tools\go\bin\go.exe"
$PnpmExe = "C:\Users\USER\.cache\codex-runtimes\codex-primary-runtime\dependencies\bin\pnpm.cmd"
$DockerBin = "C:\Program Files\Docker\Docker\resources\bin"

$env:Path = "$DockerBin;$env:Path"

New-Item -ItemType Directory -Force -Path $Tmp | Out-Null

docker compose --env-file (Join-Path $Root ".env2.local") --project-directory $Root up -d

$gatewayOut = Join-Path $Tmp "gateway.out.log"
$gatewayErr = Join-Path $Tmp "gateway.err.log"
$frontendOut = Join-Path $Tmp "ai-gateway-dashboard.out.log"
$frontendErr = Join-Path $Tmp "ai-gateway-dashboard.err.log"
$gatewayExe = Join-Path $Tmp "ai-gateway-dashboard-bff.exe"

Remove-Item $gatewayOut, $gatewayErr, $frontendOut, $frontendErr -ErrorAction SilentlyContinue
& $GoExe build -o $gatewayExe ./cmd/gateway

$backendCommand = "cd '$Root'; & '$gatewayExe'"
$frontendCommand = "cd '$Frontend'; & '$PnpmExe' dev"

Start-Process -FilePath powershell.exe `
  -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", $backendCommand) `
  -WindowStyle Hidden `
  -RedirectStandardOutput $gatewayOut `
  -RedirectStandardError $gatewayErr

Start-Sleep -Seconds 2

Start-Process -FilePath powershell.exe `
  -ArgumentList @("-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", $frontendCommand) `
  -WindowStyle Hidden `
  -RedirectStandardOutput $frontendOut `
  -RedirectStandardError $frontendErr

Write-Host "Gateway: http://127.0.0.1:8080"
Write-Host "AI Gateway Dashboard: http://127.0.0.1:5173"
Write-Host "Logs: $Tmp"
