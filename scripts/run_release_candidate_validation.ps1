param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [switch]$StartStack
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repoRoot ".env.local"
$session = $null

function Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Assert-JsonGet {
  param(
    [string]$Url,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )
  if ($WebSession) {
    return Invoke-RestMethod -Uri $Url -WebSession $WebSession -UseBasicParsing
  }
  return Invoke-RestMethod -Uri $Url -UseBasicParsing
}

if ($StartStack) {
  Step "Starting local stack"
  & (Join-Path $PSScriptRoot "bootstrap_local.ps1")
}

Step "Checking health and docs"
Assert-JsonGet -Url "$BaseUrl/healthz" | Out-Null
Invoke-WebRequest -Uri "$BaseUrl/docs/openapi.yaml" -UseBasicParsing | Out-Null
Invoke-WebRequest -Uri "$BaseUrl/docs/swagger" -UseBasicParsing | Out-Null

Step "Checking public runtime endpoints"
Assert-JsonGet -Url "$BaseUrl/api/v1/analytics/overview" | Out-Null
Assert-JsonGet -Url "$BaseUrl/api/v1/environments" | Out-Null

if ($AdminUser -and $AdminPassword) {
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

  Assert-JsonGet -Url "$BaseUrl/auth/me" -WebSession $session | Out-Null
  Assert-JsonGet -Url "$BaseUrl/api/v1/pricing" -WebSession $session | Out-Null
  Assert-JsonGet -Url "$BaseUrl/api/v1/policies" -WebSession $session | Out-Null
  Assert-JsonGet -Url "$BaseUrl/api/v1/audit/control" -WebSession $session | Out-Null
} else {
  Write-Warning "Skipping authenticated admin validation because AdminUser/AdminPassword were not provided."
}

Write-Host ""
Write-Host "Release candidate validation passed."
