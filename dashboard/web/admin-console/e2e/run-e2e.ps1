$ErrorActionPreference = "Stop"

$container = "veloxmesh-dashboard-e2e-redis-$PID"
$image = if ($env:E2E_REDIS_IMAGE) { $env:E2E_REDIS_IMAGE } else { "redis/redis-stack-server:latest" }
$exitCode = 1

try {
    docker run --detach --rm --name $container --publish "127.0.0.1::6379" $image | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Could not start the isolated Redis container. Confirm Docker Desktop is running."
    }

    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
		$previousPreference = $ErrorActionPreference
		$ErrorActionPreference = "SilentlyContinue"
		$pong = docker exec $container redis-cli ping 2>&1
		$pingExitCode = $LASTEXITCODE
		$ErrorActionPreference = $previousPreference
		if ($pingExitCode -eq 0 -and $pong -match "PONG") {
            $ready = $true
            break
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $ready) {
        throw "The isolated Redis container did not become ready."
    }

    $binding = docker port $container "6379/tcp"
    if ($binding -notmatch ':(\d+)\s*$') {
        throw "Could not determine the isolated Redis port from: $binding"
    }

    $env:E2E_REDIS_CONTAINER = $container
    $env:E2E_REDIS_ADDR = "127.0.0.1:$($Matches[1])"
    & npx.cmd playwright test @args
    $exitCode = $LASTEXITCODE
}
finally {
    docker rm --force $container 2>$null | Out-Null
}

exit $exitCode
