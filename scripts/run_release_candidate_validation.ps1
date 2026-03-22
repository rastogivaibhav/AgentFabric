param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [switch]$StartStack
)

$ErrorActionPreference = "Stop"

function Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Get-Json {
  param(
    [string]$Url,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )
  if ($WebSession) {
    return Invoke-RestMethod -Uri $Url -WebSession $WebSession -UseBasicParsing
  }
  return Invoke-RestMethod -Uri $Url -UseBasicParsing
}

function Invoke-Json {
  param(
    [string]$Method,
    [string]$Url,
    [object]$Body,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )
  $jsonBody = if ($null -ne $Body) { $Body | ConvertTo-Json -Depth 10 } else { $null }
  if ($WebSession) {
    return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $jsonBody -WebSession $WebSession -UseBasicParsing
  }
  return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $jsonBody -UseBasicParsing
}

function Invoke-Delete {
  param(
    [string]$Url,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )
  if ($WebSession) {
    Invoke-WebRequest -Method Delete -Uri $Url -WebSession $WebSession -UseBasicParsing | Out-Null
    return
  }
  Invoke-WebRequest -Method Delete -Uri $Url -UseBasicParsing | Out-Null
}

if ($StartStack) {
  Step "Starting local stack"
  & (Join-Path $PSScriptRoot "bootstrap_local.ps1")
}

Step "Checking readiness, health, and docs"
Get-Json -Url "$BaseUrl/healthz" | Out-Null
Get-Json -Url "$BaseUrl/readyz" | Out-Null
Invoke-WebRequest -Uri "$BaseUrl/docs/openapi.yaml" -UseBasicParsing | Out-Null
Invoke-WebRequest -Uri "$BaseUrl/docs/swagger" -UseBasicParsing | Out-Null

Step "Checking public runtime endpoints"
Get-Json -Url "$BaseUrl/api/v1/analytics/overview" | Out-Null
Get-Json -Url "$BaseUrl/api/v1/environments" | Out-Null

if (-not ($AdminUser -and $AdminPassword)) {
  Write-Warning "Skipping authenticated admin validation because AdminUser/AdminPassword were not provided."
  Write-Host ""
  Write-Host "Release candidate validation passed."
  exit 0
}

$session = $null
$tempRuleId = $null

try {
  Step "Checking authenticated admin paths"
  $loginBody = @{
    username = $AdminUser
    password = $AdminPassword
  } | ConvertTo-Json

  Invoke-WebRequest `
    -Method Post `
    -Uri "$BaseUrl/auth/login" `
    -ContentType "application/json" `
    -Body $loginBody `
    -SessionVariable session `
    -UseBasicParsing | Out-Null

  Get-Json -Url "$BaseUrl/auth/me" -WebSession $session | Out-Null
  $pricingRules = Get-Json -Url "$BaseUrl/api/v1/pricing" -WebSession $session
  $policyRules = Get-Json -Url "$BaseUrl/api/v1/policies" -WebSession $session
  $controlAudit = Get-Json -Url "$BaseUrl/api/v1/audit/control" -WebSession $session

  if ($null -eq $pricingRules.items) { throw "pricing API returned an unexpected shape" }
  if ($null -eq $policyRules.items) { throw "policy API returned an unexpected shape" }
  if ($null -eq $controlAudit.items) { throw "control audit API returned an unexpected shape" }

  Step "Checking pricing preview"
  $pricingPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/pricing/preview" -Body @{
    provider = "openai"
    model = "gpt-4o"
    input_tokens = 120
    output_tokens = 40
  } -WebSession $session

  if ($pricingPreview.total_cost_usd -le 0) {
    throw "pricing preview did not return a positive total cost"
  }

  Step "Checking policy preview with a staged deny rule"
  $timestamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  $createdRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "rc-preview-$timestamp"
    rule_type = "traffic"
    enabled = $true
    priority = 9999
    action = "deny"
    provider = "openai"
    model_pattern = "gpt-4o"
    environment = "staging"
    max_tokens = 10
    description = "temporary release validation rule"
  } -WebSession $session

  $tempRuleId = $createdRule.id
  if (-not $tempRuleId) {
    throw "policy upsert did not return an id"
  }

  $policyPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    provider = "openai"
    model = "gpt-4o"
    environment = "staging"
    estimated_tokens = 128
    request_body = "contact me at someone@example.com"
    response_body = "safe response"
  } -WebSession $session

  if (-not $policyPreview.traffic.matched) {
    throw "policy preview traffic check did not match the staged deny rule"
  }
  if ($policyPreview.traffic.action -ne "deny") {
    throw "policy preview traffic action was expected to be deny"
  }

  Step "Checking DLP preview path"
  $dlpPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    provider = "openai"
    model = "gpt-4o"
    environment = "production"
    estimated_tokens = 32
    request_body = "secret token sk-1234567890abcdefghijklmnop"
    response_body = "user email someone@example.com"
  } -WebSession $session

  if ($null -eq $dlpPreview.request_dlp -or $null -eq $dlpPreview.response_dlp) {
    throw "policy preview DLP response did not contain both request and response decisions"
  }

  Step "Checking control-plane audit after temporary mutation"
  $controlAuditAfter = Get-Json -Url "$BaseUrl/api/v1/audit/control" -WebSession $session
  if (($controlAuditAfter.count | ForEach-Object { [int]$_ }) -lt ($controlAudit.count | ForEach-Object { [int]$_ })) {
    throw "control-plane audit count regressed after temporary mutation"
  }
}
finally {
  if ($session -and $tempRuleId) {
    try {
      Invoke-Delete -Url "$BaseUrl/api/v1/policies/$tempRuleId" -WebSession $session
    } catch {
      Write-Warning "Failed to clean up temporary policy rule $tempRuleId"
    }
  }
}

Write-Host ""
Write-Host "Release candidate validation passed."
