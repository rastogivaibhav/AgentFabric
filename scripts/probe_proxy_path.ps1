param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [string]$ProxyVirtualKey = "",
  [string]$ProxyPath = "/proxy/openai/v1/chat/completions",
  [string]$ProxyBodyJson = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello secret sk-abcdefghijklmnopqrstuvwxyz12345"}],"stream":false}',
  [string]$TenantId = "00000000-0000-0000-0000-000000000001"
)

$ErrorActionPreference = "Stop"

function Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Invoke-Json {
  param(
    [string]$Method,
    [string]$Url,
    [object]$Body,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession
  )
  $jsonBody = if ($Body -is [string]) { $Body } elseif ($null -ne $Body) { $Body | ConvertTo-Json -Depth 20 } else { $null }
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
  Invoke-WebRequest -Method Delete -Uri $Url -WebSession $WebSession -UseBasicParsing | Out-Null
}

if (-not ($AdminUser -and $AdminPassword)) {
  throw "AdminUser and AdminPassword are required."
}
if (-not $ProxyVirtualKey) {
  throw "ProxyVirtualKey is required."
}

$session = $null
$createdRuleId = $null

try {
  Step "Logging in"
  Invoke-WebRequest -Method Post -Uri "$BaseUrl/auth/login" -ContentType "application/json" `
    -Body (@{ username = $AdminUser; password = $AdminPassword } | ConvertTo-Json) `
    -SessionVariable session -UseBasicParsing | Out-Null

  Step "Checking pricing preview"
  $pricingPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/pricing/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    input_tokens = 120
    output_tokens = 40
  } -WebSession $session
  if ($pricingPreview.total_cost_usd -le 0) {
    throw "pricing preview did not return a positive cost"
  }

  Step "Checking policy preview"
  $policyPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    environment = "staging"
    estimated_tokens = 64
    request_body = "hello secret sk-abcdefghijklmnopqrstuvwxyz12345"
    response_body = "safe response"
  } -WebSession $session
  if ($null -eq $policyPreview.request_dlp) {
    throw "policy preview missing request_dlp"
  }

  Step "Creating temporary request DLP rule"
  $stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  $createdRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "proxy-proof-redact-$stamp"
    rule_type = "dlp"
    decision_mode = "fast"
    enabled = $true
    priority = 3200
    action = "redact"
    detector = "secret"
    scope = "request"
    description = "temporary proxy-path proof rule"
  } -WebSession $session
  $createdRuleId = $createdRule.id

  Step "Sending proxied request"
  $beforeTraces = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/traces?framework=proxy&model=gpt-4o-mini&limit=5" -Body $null -WebSession $session
  $beforeLatest = if ($beforeTraces.items.Count -gt 0) { $beforeTraces.items[0].id } else { "" }
  $proxyHeaders = @{
    Authorization = "Bearer $ProxyVirtualKey"
    "Content-Type" = "application/json"
  }
  Invoke-RestMethod -Method Post -Uri "$BaseUrl$ProxyPath" -Headers $proxyHeaders -Body $ProxyBodyJson -UseBasicParsing | Out-Null

  Step "Verifying trace, cost, audit, and policy visibility"
  Start-Sleep -Milliseconds 800
  $afterTraces = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/traces?framework=proxy&model=gpt-4o-mini&limit=5" -Body $null -WebSession $session
  if ($afterTraces.items.Count -lt 1) {
    throw "no proxy traces found after proxied request"
  }
  $traceId = $afterTraces.items[0].id
  if ($beforeLatest -and $traceId -eq $beforeLatest -and $afterTraces.items.Count -gt 1) {
    $traceId = $afterTraces.items[1].id
  }
  $trace = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/traces/$traceId" -Body $null -WebSession $session
  $cost = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/traces/$traceId/cost" -Body $null -WebSession $session
  $audit = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/audit/control" -Body $null -WebSession $session

  if ($cost.total_usd -le 0) {
    throw "trace cost was not positive"
  }
  if ($null -eq $trace.policy_events) {
    throw "trace is missing policy_events"
  }
  if ($audit.count -lt 1) {
    throw "control audit returned no entries"
  }

  Write-Host ""
  Write-Host "Proxy path probe passed."
}
finally {
  if ($session -and $createdRuleId) {
    try { Invoke-Delete -Url "$BaseUrl/api/v1/policies/$createdRuleId" -WebSession $session } catch {}
  }
}
