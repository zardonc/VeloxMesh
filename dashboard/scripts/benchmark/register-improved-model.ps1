param(
  [Parameter(Mandatory = $true)]
  [string]$GatewayUrl,
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[a-zA-Z0-9._-]+$')]
  [string]$ProviderId,
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^https?://')]
  [string]$BaseUrl,
  [Parameter(Mandatory = $true)]
  [string]$ModelId,
  [Parameter(Mandatory = $true)]
  [string]$ModelVersion,
  [int]$TimeoutSeconds = 30
)

$ErrorActionPreference = 'Stop'

$adminApiKey = [Environment]::GetEnvironmentVariable('VELOXMESH_ADMIN_API_KEY')
$dataApiKey = [Environment]::GetEnvironmentVariable('VELOXMESH_DATA_API_KEY')
$providerApiKey = [Environment]::GetEnvironmentVariable('IMPROVED_MODEL_API_KEY')
if ([string]::IsNullOrWhiteSpace($adminApiKey)) {
  throw 'VELOXMESH_ADMIN_API_KEY is required'
}
if ([string]::IsNullOrWhiteSpace($dataApiKey)) {
  throw 'VELOXMESH_DATA_API_KEY is required'
}
if ([string]::IsNullOrWhiteSpace($providerApiKey)) {
  throw 'IMPROVED_MODEL_API_KEY is required'
}

$gateway = $GatewayUrl.TrimEnd('/')
$adminHeaders = @{
  Authorization = "Bearer $adminApiKey"
  'X-Admin-Actor' = 'improved-model-registration'
  'X-Request-ID' = "improved-model-$([guid]::NewGuid().ToString('N'))"
}
$dataHeaders = @{ Authorization = "Bearer $dataApiKey" }

$health = Invoke-RestMethod -Method Get -Uri "$gateway/healthz" -TimeoutSec $TimeoutSeconds
if ([string]$health -ne 'ok') {
  throw 'Gateway health check did not return ok'
}

$providerBody = @{
  id = $ProviderId
  name = "Improved Model ($ModelVersion)"
  type = 'openai-compatible'
  base_url = $BaseUrl.TrimEnd('/')
  enabled = $true
  api_key = $providerApiKey
  models = @($ModelId)
  default_model = $ModelId
  timeout = "${TimeoutSeconds}s"
} | ConvertTo-Json -Depth 5

$provider = Invoke-RestMethod -Method Post -Uri "$gateway/admin/v1/providers" -Headers $adminHeaders -ContentType 'application/json' -Body $providerBody -TimeoutSec $TimeoutSeconds
if ([string]$provider.id -ne $ProviderId) {
  throw 'Gateway did not return the requested Provider ID'
}

$models = Invoke-RestMethod -Method Get -Uri "$gateway/v1/models" -Headers $dataHeaders -TimeoutSec $TimeoutSeconds
$modelFound = @($models.data | Where-Object { [string]$_.id -eq $ModelId }).Count -gt 0
if (-not $modelFound) {
  throw 'Improved model was not returned by the Gateway model list'
}

$chatBody = @{
  model = $ModelId
  messages = @(@{ role = 'user'; content = 'Reply with OK.' })
  max_tokens = 8
} | ConvertTo-Json -Depth 5
$chat = Invoke-RestMethod -Method Post -Uri "$gateway/v1/chat/completions" -Headers $dataHeaders -ContentType 'application/json' -Body $chatBody -TimeoutSec $TimeoutSeconds
$chatVerified = @($chat.choices).Count -gt 0 -and -not [string]::IsNullOrWhiteSpace([string]$chat.choices[0].message.content)
if (-not $chatVerified) {
  throw 'Gateway chat verification returned no assistant content'
}

[ordered]@{
  verified = $true
  provider_id = $ProviderId
  model_id = $ModelId
  model_version = $ModelVersion
  gateway_url = $gateway
  health = 'ok'
  models_verified = $true
  chat_verified = $true
  revision = $provider.revision
} | ConvertTo-Json -Compress

