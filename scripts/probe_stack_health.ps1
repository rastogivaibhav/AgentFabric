param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$CollectorUrl = "http://localhost:4318"
)

$ErrorActionPreference = "Stop"

function Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Get-Json {
  param([string]$Url)
  return Invoke-RestMethod -Uri $Url -UseBasicParsing
}

function Assert-Check {
  param(
    [object]$Response,
    [string]$CheckName
  )
  if ($null -eq $Response.checks -or -not $Response.checks.PSObject.Properties.Name.Contains($CheckName)) {
    throw "missing readiness check '$CheckName'"
  }
  $check = $Response.checks.PSObject.Properties[$CheckName].Value
  $status = "$($check.status)"
  if ($status -ne "ok" -and $status -ne "loaded" -and $status -ne "configured" -and $status -ne "healthy") {
    throw "check '$CheckName' is not healthy: $status"
  }
}

Step "Probing gateway health"
$gatewayHealth = Get-Json "$BaseUrl/healthz"
if ($gatewayHealth.status -ne "ok") {
  throw "gateway healthz is not ok"
}

Step "Probing gateway readiness"
$gatewayReady = Get-Json "$BaseUrl/readyz"
if ($gatewayReady.status -ne "ok") {
  throw "gateway readyz is not ok"
}
Assert-Check $gatewayReady "postgres"
Assert-Check $gatewayReady "redis"
Assert-Check $gatewayReady "pricing_rules"
Assert-Check $gatewayReady "policy_engine"
Assert-Check $gatewayReady "startup_state"

Step "Probing collector health"
$collectorHealth = Get-Json "$CollectorUrl/healthz"
if ($collectorHealth.status -ne "ok") {
  throw "collector healthz is not ok"
}

Step "Probing collector readiness"
$collectorReady = Get-Json "$CollectorUrl/readyz"
if ($collectorReady.status -ne "ok") {
  throw "collector readyz is not ok"
}
Assert-Check $collectorReady "receiver"
Assert-Check $collectorReady "gateway_export"
Assert-Check $collectorReady "pricing_config"
Assert-Check $collectorReady "gateway_auth_token"
Assert-Check $collectorReady "gateway_readyz"

Write-Host ""
Write-Host "Stack health probe passed."
