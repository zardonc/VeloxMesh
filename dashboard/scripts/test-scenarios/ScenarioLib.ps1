$ErrorActionPreference = "Stop"

function Get-Percentile {
  param(
    [Parameter(Mandatory = $true)]
    [double[]]$Values,
    [Parameter(Mandatory = $true)]
    [ValidateRange(0, 100)]
    [double]$Percentile
  )

  if ($Values.Count -eq 0) {
    return 0
  }

  $sorted = @($Values | Sort-Object)
  $rank = [Math]::Ceiling(($Percentile / 100) * $sorted.Count) - 1
  $index = [Math]::Max(0, [Math]::Min($sorted.Count - 1, [int]$rank))
  return [double]$sorted[$index]
}

function Get-Average {
  param(
    [double[]]$Values
  )

  if ($Values.Count -eq 0) {
    return 0
  }

  $sum = 0.0
  foreach ($value in $Values) {
    $sum += $value
  }
  return [Math]::Round($sum / $Values.Count, 2)
}

function Invoke-CacheScenario {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string[]]$Prompts,
    [switch]$CacheEnabled,
    [int]$MissLatencyMs = 120,
    [int]$HitLatencyMs = 28
  )

  $cache = @{}
  $latencies = New-Object System.Collections.Generic.List[double]
  $hits = 0
  $misses = 0
  $events = New-Object System.Collections.Generic.List[object]

  for ($i = 0; $i -lt $Prompts.Count; $i++) {
    $prompt = $Prompts[$i]
    $key = $prompt.Trim().ToLowerInvariant()
    $hit = $false

    if ($CacheEnabled -and $cache.ContainsKey($key)) {
      $hit = $true
      $hits += 1
      $latency = $HitLatencyMs
    } else {
      $misses += 1
      $latency = $MissLatencyMs + (($i % 4) * 9)
      if ($CacheEnabled) {
        $cache[$key] = $true
      }
    }

    $latencies.Add([double]$latency)
    $events.Add([pscustomobject]@{
      Index = $i + 1
      Prompt = $prompt
      CacheHit = $hit
      LatencyMs = $latency
    })
  }

  $successRate = if ($Prompts.Count -eq 0) { 0 } else { 100 }
  $hitRate = if ($Prompts.Count -eq 0) { 0 } else { [Math]::Round(($hits / $Prompts.Count) * 100, 2) }

  return [pscustomobject]@{
    Name = $Name
    Type = if ($CacheEnabled) { "cache-enabled" } else { "cache-disabled" }
    Passed = $true
    RequestCount = $Prompts.Count
    SuccessRate = $successRate
    CacheEnabled = [bool]$CacheEnabled
    CacheHits = $hits
    CacheMisses = $misses
    CacheHitRate = $hitRate
    AverageLatencyMs = Get-Average -Values $latencies.ToArray()
    P50LatencyMs = Get-Percentile -Values $latencies.ToArray() -Percentile 50
    P95LatencyMs = Get-Percentile -Values $latencies.ToArray() -Percentile 95
    Events = $events.ToArray()
    Notes = if ($CacheEnabled) { "Repeated prompts are served from the local cache simulation." } else { "Every prompt is treated as a cache miss." }
  }
}

function Invoke-ColdWarmCacheScenario {
  param(
    [Parameter(Mandatory = $true)]
    [string[]]$Prompts,
    [int]$MissLatencyMs = 120,
    [int]$HitLatencyMs = 28
  )

  $cold = Invoke-CacheScenario -Name "Cold Cache Pass" -Prompts $Prompts -CacheEnabled -MissLatencyMs $MissLatencyMs -HitLatencyMs $HitLatencyMs
  $warmPrompts = @($Prompts + $Prompts)
  $warm = Invoke-CacheScenario -Name "Warm Cache Pass" -Prompts $warmPrompts -CacheEnabled -MissLatencyMs $MissLatencyMs -HitLatencyMs $HitLatencyMs
  $reduction = if ($cold.AverageLatencyMs -eq 0) { 0 } else { [Math]::Round((($cold.AverageLatencyMs - $warm.AverageLatencyMs) / $cold.AverageLatencyMs) * 100, 2) }

  return [pscustomobject]@{
    Name = "Cold Cache vs Warm Cache"
    Type = "cache-warmup"
    Passed = ($warm.CacheHits -gt $cold.CacheHits)
    RequestCount = $warm.RequestCount
    SuccessRate = 100
    CacheEnabled = $true
    CacheHits = $warm.CacheHits
    CacheMisses = $warm.CacheMisses
    CacheHitRate = $warm.CacheHitRate
    AverageLatencyMs = $warm.AverageLatencyMs
    P50LatencyMs = $warm.P50LatencyMs
    P95LatencyMs = $warm.P95LatencyMs
    ColdAverageLatencyMs = $cold.AverageLatencyMs
    WarmAverageLatencyMs = $warm.AverageLatencyMs
    ImprovementPercent = $reduction
    Notes = "Second pass reuses repeated prompts and demonstrates cache warmup behavior."
  }
}

function New-ComparisonPair {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [object]$Baseline,
    [Parameter(Mandatory = $true)]
    [object]$Treatment,
    [string]$Metric = "Average latency",
    [string]$Notes = ""
  )

  $baselineValue = [double]$Baseline.AverageLatencyMs
  $treatmentValue = [double]$Treatment.AverageLatencyMs
  $improvement = if ($baselineValue -eq 0) { 0 } else { [Math]::Round((($baselineValue - $treatmentValue) / $baselineValue) * 100, 2) }

  return [pscustomobject]@{
    Name = $Name
    Type = "comparison"
    Passed = ($Treatment.Passed -and $Baseline.Passed)
    BaselineName = $Baseline.Name
    TreatmentName = $Treatment.Name
    Metric = $Metric
    BaselineValue = $baselineValue
    TreatmentValue = $treatmentValue
    ImprovementPercent = $improvement
    Baseline = $Baseline
    Treatment = $Treatment
    Notes = if ($Notes.Trim()) { $Notes } else { "$($Treatment.Name) is compared against $($Baseline.Name)." }
  }
}

function New-ManualComparisonPair {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$BaselineName,
    [Parameter(Mandatory = $true)]
    [string]$TreatmentName,
    [Parameter(Mandatory = $true)]
    [double]$BaselineValue,
    [Parameter(Mandatory = $true)]
    [double]$TreatmentValue,
    [string]$Metric = "Mean latency",
    [bool]$Passed = $true,
    [string]$Notes = ""
  )

  $improvement = if ($BaselineValue -eq 0) { 0 } else { [Math]::Round((($BaselineValue - $TreatmentValue) / $BaselineValue) * 100, 2) }

  return [pscustomobject]@{
    Name = $Name
    Type = "comparison"
    Passed = $Passed
    BaselineName = $BaselineName
    TreatmentName = $TreatmentName
    Metric = $Metric
    BaselineValue = [Math]::Round($BaselineValue, 2)
    TreatmentValue = [Math]::Round($TreatmentValue, 2)
    ImprovementPercent = $improvement
    Notes = $Notes
  }
}

function New-PendingComparisonPair {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$BaselineName,
    [Parameter(Mandatory = $true)]
    [string]$TreatmentName,
    [string]$Notes = ""
  )

  return [pscustomobject]@{
    Name = $Name
    Type = "pending-comparison"
    Passed = $false
    Pending = $true
    BaselineName = $BaselineName
    TreatmentName = $TreatmentName
    Metric = "Pending integration"
    BaselineValue = ""
    TreatmentValue = ""
    ImprovementPercent = ""
    Notes = $Notes
  }
}

function Test-HttpEndpoint {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$Url,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )

  try {
    $request = @{
      UseBasicParsing = $true
      Uri = $Url
      TimeoutSec = 10
    }
    if ($WebSession) {
      $request.WebSession = $WebSession
    }
    $response = Invoke-WebRequest @request
    return [pscustomobject]@{
      Name = $Name
      Url = $Url
      Passed = ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300)
      StatusCode = $response.StatusCode
      Detail = "HTTP $($response.StatusCode)"
    }
  } catch {
    return [pscustomobject]@{
      Name = $Name
      Url = $Url
      Passed = $false
      StatusCode = 0
      Detail = $_.Exception.Message
    }
  }
}

function New-AdminWebSession {
  param(
    [Parameter(Mandatory = $true)]
    [string]$BaseUrl
  )

  $session = New-Object Microsoft.PowerShell.Commands.WebRequestSession
  $suffix = ([guid]::NewGuid().ToString("N")).Substring(0, 12)
  $username = "scenario_admin_$suffix"
  $password = "ScenarioPass1234"
  $email = "$username@example.test"

  $registerBody = @{
    email = $email
    username = $username
    password = $password
    role = "Admin"
  } | ConvertTo-Json
  Invoke-RestMethod -Uri "$BaseUrl/bff/auth/register" -Method Post -ContentType "application/json" -Body $registerBody -WebSession $session -TimeoutSec 10 | Out-Null

  $loginBody = @{
    identifier = $username
    password = $password
  } | ConvertTo-Json
  $login = Invoke-RestMethod -Uri "$BaseUrl/bff/auth/login" -Method Post -ContentType "application/json" -Body $loginBody -WebSession $session -TimeoutSec 10
  $code = [string]$login.devCode
  if (-not $code) {
    $root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
    $outbox = Join-Path $root "tmp\email-outbox.log"
    $content = Get-Content -LiteralPath $outbox -Raw
    $matches = [regex]::Matches($content, "code is\s+(\d{6})")
    if ($matches.Count -eq 0) {
      throw "verification code was not available from login response or local outbox"
    }
    $code = $matches[$matches.Count - 1].Groups[1].Value
  }

  $verifyBody = @{
    challengeId = $login.challengeId
    code = $code
  } | ConvertTo-Json
  Invoke-RestMethod -Uri "$BaseUrl/bff/auth/verify-login" -Method Post -ContentType "application/json" -Body $verifyBody -WebSession $session -TimeoutSec 10 | Out-Null
  return $session
}

function Write-ScenarioJson {
  param(
    [Parameter(Mandatory = $true)]
    [object]$Data,
    [Parameter(Mandatory = $true)]
    [string]$Path
  )

  $directory = Split-Path -Parent $Path
  New-Item -ItemType Directory -Force -Path $directory | Out-Null
  $Data | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $Path -Encoding UTF8
}

function ConvertTo-ObjectArray {
  param(
    [AllowNull()]
    [object]$InputObject
  )

  $items = New-Object System.Collections.Generic.List[object]

  function Add-FlattenedItem {
    param(
      [AllowNull()]
      [object]$Item,
      [System.Collections.Generic.List[object]]$Target
    )

    if ($null -eq $Item) {
      return
    }

    if ($Item -is [System.Array]) {
      foreach ($child in $Item) {
        Add-FlattenedItem -Item $child -Target $Target
      }
    } else {
      $Target.Add($Item)
    }
  }

  Add-FlattenedItem -Item $InputObject -Target $items
  return $items.ToArray()
}

function Read-ScenarioConfig {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Path
  )

  if (-not (Test-Path -LiteralPath $Path)) {
    throw "Scenario config not found: $Path"
  }

  return Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
}

function New-ReadinessScenario {
  param(
    [Parameter(Mandatory = $true)]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [object[]]$Checks,
    [string]$Notes = ""
  )

  $failed = @($Checks | Where-Object { -not $_.Passed })
  return [pscustomobject]@{
    Name = $Name
    Type = "readiness"
    Passed = ($failed.Count -eq 0)
    RequestCount = $Checks.Count
    SuccessRate = if ($Checks.Count -eq 0) { 0 } else { [Math]::Round((($Checks.Count - $failed.Count) / $Checks.Count) * 100, 2) }
    CacheEnabled = $false
    CacheHits = 0
    CacheMisses = 0
    CacheHitRate = 0
    AverageLatencyMs = 0
    P50LatencyMs = 0
    P95LatencyMs = 0
    Checks = $Checks
    Notes = $Notes
  }
}

function Get-BenchmarkStorageStatus {
  param(
    [Parameter(Mandatory = $true)]
    [object]$Payload
  )

  $redisStatus = ""
  $redisDetail = ""
  $qdrantStatus = ""
  $qdrantDetail = ""

  if ($null -ne $Payload.storage) {
    if ($null -ne $Payload.storage.redis) {
      $redisStatus = [string]$Payload.storage.redis.status
      $redisDetail = [string]$Payload.storage.redis.detail
    }
    if ($null -ne $Payload.storage.qdrant) {
      $qdrantStatus = [string]$Payload.storage.qdrant.status
      $qdrantDetail = [string]$Payload.storage.qdrant.detail
    }
  } else {
    if ($null -ne $Payload.redis) {
      $redisStatus = [string]$Payload.redis.status
      $redisDetail = [string]$Payload.redis.detail
    }
    if ($null -ne $Payload.qdrant) {
      $qdrantStatus = [string]$Payload.qdrant.status
      $qdrantDetail = [string]$Payload.qdrant.detail
    }
  }

  return [pscustomobject]@{
    RedisStatus = $redisStatus
    RedisDetail = $redisDetail
    RedisConnected = ($redisStatus.ToLowerInvariant() -eq "connected")
    QdrantStatus = $qdrantStatus
    QdrantDetail = $qdrantDetail
    QdrantConnected = ($qdrantStatus.ToLowerInvariant() -eq "connected")
  }
}

function ConvertTo-HtmlEncoded {
  param(
    [AllowNull()]
    [object]$Value
  )

  if ($null -eq $Value) {
    return ""
  }

  return [System.Net.WebUtility]::HtmlEncode([string]$Value)
}

function New-TestScenarioHtmlReport {
  param(
    [Parameter(Mandatory = $true)]
    [object[]]$ScenarioResults,
    [object[]]$ComparisonPairs = @(),
    [object[]]$EnvironmentChecks = @(),
    [Parameter(Mandatory = $true)]
    [string]$OutputPath,
    [string]$Title = "VeloxMesh AI Gateway Test Report"
  )

  $generatedAt = Get-Date -Format "yyyy-MM-dd HH:mm:ss zzz"
  $measuredPairs = @($ComparisonPairs | Where-Object { -not $_.Pending })
  $passedCount = @($measuredPairs | Where-Object { $_.Passed }).Count
  $totalCount = @($measuredPairs).Count
  $envFailed = @($EnvironmentChecks | Where-Object { -not $_.Passed })
  $overall = if ($envFailed.Count -eq 0 -and $totalCount -gt 0 -and $passedCount -eq $totalCount) { "Ready" } else { "Needs attention" }

  if ($ComparisonPairs.Count -eq 0) {
    $ComparisonPairs = @($ScenarioResults | ForEach-Object {
      New-ManualComparisonPair -Name $_.Name -BaselineName $_.Name -TreatmentName $_.Name -BaselineValue $_.AverageLatencyMs -TreatmentValue $_.AverageLatencyMs -Metric "Average latency" -Passed $_.Passed -Notes $_.Notes
    })
  }

  $envCards = foreach ($result in $EnvironmentChecks) {
    $statusClass = if ($result.Passed) { "pass" } else { "fail" }
    $statusText = if ($result.Passed) { "PASS" } else { "FAIL" }
    $metrics = @(
      "Checks: $($result.RequestCount)",
      "Success: $($result.SuccessRate)%"
    ) -join "</li><li>"
    $checkItems = @($result.Checks | ForEach-Object {
      $checkStatus = if ($_.Passed) { "PASS" } else { "FAIL" }
      "<li><strong>$(ConvertTo-HtmlEncoded $_.Name)</strong>: $checkStatus, HTTP $($_.StatusCode), $(ConvertTo-HtmlEncoded $_.Detail)</li>"
    }) -join ""

    "<section class='card $statusClass'><div class='card-head'><h2>$(ConvertTo-HtmlEncoded $result.Name)</h2><span>$statusText</span></div><ul><li>$metrics</li></ul><ul>$checkItems</ul><p>$(ConvertTo-HtmlEncoded $result.Notes)</p></section>"
  }

  $pairCards = foreach ($pair in $ComparisonPairs) {
    $statusClass = if ($pair.Pending) { "pending" } elseif ($pair.Passed) { "pass" } else { "fail" }
    $statusText = if ($pair.Pending) { "PENDING" } elseif ($pair.Passed) { "PASS" } else { "FAIL" }
    $improvementText = if ($pair.Pending) { "Pending teammate module" } else { "$($pair.ImprovementPercent)% improvement" }

    "<section class='card $statusClass'><div class='card-head'><h2>$(ConvertTo-HtmlEncoded $pair.Name)</h2><span>$statusText</span></div><p><strong>A:</strong> $(ConvertTo-HtmlEncoded $pair.BaselineName)</p><p><strong>B:</strong> $(ConvertTo-HtmlEncoded $pair.TreatmentName)</p><ul><li>Metric: $(ConvertTo-HtmlEncoded $pair.Metric)</li><li>A value: $($pair.BaselineValue)</li><li>B value: $($pair.TreatmentValue)</li><li>Delta: $improvementText</li></ul><p>$(ConvertTo-HtmlEncoded $pair.Notes)</p></section>"
  }

  $pairRows = foreach ($pair in $ComparisonPairs) {
    $statusText = if ($pair.Pending) { "PENDING" } elseif ($pair.Passed) { "PASS" } else { "FAIL" }
    $delta = if ($pair.Pending) { "Pending" } else { "$($pair.ImprovementPercent)%" }
    "<tr><td>$(ConvertTo-HtmlEncoded $pair.Name)</td><td>$statusText</td><td>$(ConvertTo-HtmlEncoded $pair.BaselineName)</td><td>$(ConvertTo-HtmlEncoded $pair.TreatmentName)</td><td>$(ConvertTo-HtmlEncoded $pair.Metric)</td><td>$($pair.BaselineValue)</td><td>$($pair.TreatmentValue)</td><td>$delta</td><td>$(ConvertTo-HtmlEncoded $pair.Notes)</td></tr>"
  }

  $html = @"
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>$(ConvertTo-HtmlEncoded $Title)</title>
  <style>
    :root { color-scheme: light; --ink:#17202a; --muted:#5f6b7a; --line:#d8dee8; --panel:#ffffff; --bg:#f5f7fb; --ok:#147d4f; --bad:#b42318; --accent:#315c96; }
    body { margin:0; font-family: Arial, sans-serif; background:var(--bg); color:var(--ink); }
    header { padding:32px 40px 24px; background:#ffffff; border-bottom:1px solid var(--line); }
    header h1 { margin:0 0 8px; font-size:30px; }
    header p { margin:0; color:var(--muted); }
    main { max-width:1180px; margin:0 auto; padding:28px 24px 44px; }
    .summary { display:grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap:14px; margin-bottom:22px; }
    .metric, .card { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:18px; }
    .metric span { display:block; color:var(--muted); font-size:13px; margin-bottom:8px; }
    .metric strong { font-size:24px; }
    .grid { display:grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap:14px; }
    .card-head { display:flex; align-items:center; justify-content:space-between; gap:12px; border-bottom:1px solid var(--line); padding-bottom:10px; }
    .card h2 { margin:0; font-size:18px; }
    .card span { font-weight:700; }
    .pass .card-head span { color:var(--ok); }
    .fail .card-head span { color:var(--bad); }
    .pending .card-head span { color:var(--accent); }
    ul { padding-left:20px; color:var(--muted); }
    table { width:100%; border-collapse:collapse; background:#fff; border:1px solid var(--line); border-radius:8px; overflow:hidden; margin-top:22px; }
    th, td { border-bottom:1px solid var(--line); padding:11px 12px; text-align:left; vertical-align:top; }
    th { background:#eaf0f7; color:#23384f; font-size:13px; }
    footer { color:var(--muted); margin-top:24px; font-size:13px; }
    @media (max-width: 760px) { header { padding:24px; } .summary, .grid { grid-template-columns:1fr; } table { font-size:13px; } }
  </style>
</head>
<body>
  <header>
    <h1>$(ConvertTo-HtmlEncoded $Title)</h1>
    <p>Generated $generatedAt from local deterministic scenarios and AI Gateway BFF readiness checks.</p>
  </header>
  <main>
    <section class="summary">
      <div class="metric"><span>Overall status</span><strong>$overall</strong></div>
      <div class="metric"><span>Measured A/B pairs</span><strong>$passedCount / $totalCount</strong></div>
      <div class="metric"><span>Report target</span><strong>A/B latency testing</strong></div>
    </section>
    <h2>Environment Readiness</h2>
    <section class="grid">
      $($envCards -join "`n")
    </section>
    <h2>A/B Test Results</h2>
    <section class="grid">
      $($pairCards -join "`n")
    </section>
    <table>
      <thead><tr><th>Pair</th><th>Status</th><th>A baseline</th><th>B treatment</th><th>Metric</th><th>A value</th><th>B value</th><th>Delta</th><th>Notes</th></tr></thead>
      <tbody>$($pairRows -join "`n")</tbody>
    </table>
    <footer>Artifacts are generated under dashboard/tmp/test-scenarios. This report avoids real provider calls by default.</footer>
  </main>
</body>
</html>
"@

  $directory = Split-Path -Parent $OutputPath
  New-Item -ItemType Directory -Force -Path $directory | Out-Null
  Set-Content -LiteralPath $OutputPath -Value $html -Encoding UTF8
}
