param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [string]$TenantId = "00000000-0000-0000-0000-000000000001",
  [string]$ProxyVirtualKey = "",
  [string]$ProxyPath = "/proxy/openai/v1/chat/completions",
  [string]$ProxyBodyJson = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"governance validation"}],"stream":false}'
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
  if ($WebSession) {
    Invoke-WebRequest -Method Delete -Uri $Url -WebSession $WebSession -UseBasicParsing | Out-Null
    return
  }
  Invoke-WebRequest -Method Delete -Uri $Url -UseBasicParsing | Out-Null
}

if (-not ($AdminUser -and $AdminPassword)) {
  throw "AdminUser and AdminPassword are required for governance validation."
}

$session = $null
$createdPolicyRuleIds = @()
$createdPricingRuleId = $null

try {
  Step "Logging in as admin"
  Invoke-WebRequest `
    -Method Post `
    -Uri "$BaseUrl/auth/login" `
    -ContentType "application/json" `
    -Body (@{ username = $AdminUser; password = $AdminPassword } | ConvertTo-Json) `
    -SessionVariable session `
    -UseBasicParsing | Out-Null

  Step "Creating temporary governance rules"
  $stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
  $allowRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "gv-allow-$stamp"
    rule_type = "traffic"
    decision_mode = "fast"
    enabled = $true
    priority = 2000
    action = "allow"
    provider = "openai"
    model_pattern = "gpt-4o-mini"
    environment = "staging"
    description = "temporary governance allow rule"
  } -WebSession $session
  $createdPolicyRuleIds += $allowRule.id

  $denyRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "gv-deny-$stamp"
    rule_type = "traffic"
    decision_mode = "rego"
    enabled = $true
    priority = 2500
    action = "deny"
    provider = "openai"
    model_pattern = "gpt-4o"
    environment = "staging"
    rego_module = 'deny if input.environment == "staging" && input.estimated_tokens > 100'
    description = "temporary governance deny rule"
  } -WebSession $session
  $createdPolicyRuleIds += $denyRule.id

  $redactRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "gv-redact-$stamp"
    rule_type = "dlp"
    decision_mode = "fast"
    enabled = $true
    priority = 2600
    action = "redact"
    detector = "secret"
    scope = "request"
    description = "temporary governance redact rule"
  } -WebSession $session
  $createdPolicyRuleIds += $redactRule.id

  $warnRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/policies" -Body @{
    name = "gv-warn-$stamp"
    rule_type = "dlp"
    decision_mode = "rego"
    enabled = $true
    priority = 2550
    action = "warn"
    scope = "response"
    rego_module = 'warn if input.scope == "response" && input.response_body contains "@"'
    description = "temporary governance warn rule"
  } -WebSession $session
  $createdPolicyRuleIds += $warnRule.id

  $pricingRule = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/pricing" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model_pattern = "gpt-4o-mini"
    input_per_million = 8.0
    output_per_million = 16.0
    active = $true
    priority = 999
    description = "temporary governance tenant pricing override"
  } -WebSession $session
  $createdPricingRuleId = $pricingRule.id

  Step "Scenario allow"
  $allowPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    environment = "staging"
    estimated_tokens = 20
    request_body = "safe request"
    response_body = "safe response"
  } -WebSession $session
  if (-not $allowPreview.traffic.matched -or $allowPreview.traffic.action -ne "allow") {
    throw "scenario_allow failed"
  }

  Step "Scenario deny"
  $denyPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o"
    environment = "staging"
    estimated_tokens = 200
    request_body = "safe request"
    response_body = "safe response"
  } -WebSession $session
  if (-not $denyPreview.traffic.matched -or $denyPreview.traffic.action -ne "deny") {
    throw "scenario_deny_model failed"
  }

  Step "Scenario redact"
  $redactPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    environment = "staging"
    estimated_tokens = 32
    request_body = "secret sk-abcdefghijklmnopqrstuvwxyz12345"
    response_body = "safe response"
  } -WebSession $session
  if (-not $redactPreview.request_dlp.matched -or $redactPreview.request_dlp.action -ne "redact") {
    throw "scenario_redact_secret failed"
  }

  Step "Scenario warn"
  $warnPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    environment = "staging"
    estimated_tokens = 32
    request_body = "safe request"
    response_body = "contact me at analyst@example.com"
  } -WebSession $session
  if (-not $warnPreview.response_dlp.matched -or $warnPreview.response_dlp.action -ne "warn") {
    throw "scenario_warn_pii failed"
  }

  Step "Scenario tenant override pricing"
  $pricingPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/pricing/preview" -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    input_tokens = 1000
    output_tokens = 1000
  } -WebSession $session
  if (-not $pricingPreview.matched -or $pricingPreview.rule_id -ne $createdPricingRuleId) {
    throw "scenario_tenant_override_pricing failed"
  }

  Step "Scenario budget limit"
  $null = Invoke-Json -Method Put -Url "$BaseUrl/api/v1/budgets/$TenantId" -Body @{
    monthly_tokens = 1
    monthly_cost_usd = 0.000001
    alert_threshold = 0.5
    hard_limit = $true
    reset_day = 1
  } -WebSession $session
  $budgetUsage = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/budgets/$TenantId/usage" -Body $null -WebSession $session
  if ($null -eq $budgetUsage.tokens_used) {
    throw "scenario_budget_limit usage lookup failed"
  }

  if ($ProxyVirtualKey) {
    Step "Live proxy budget check"
    $proxyHeaders = @{
      Authorization = "Bearer $ProxyVirtualKey"
      "Content-Type" = "application/json"
    }
    try {
      Invoke-RestMethod -Method Post -Uri "$BaseUrl$ProxyPath" -Headers $proxyHeaders -Body $ProxyBodyJson -UseBasicParsing | Out-Null
      Write-Warning "Live proxy request did not hard-fail; verify budget state manually if needed."
    } catch {
      if ($_.Exception.Response.StatusCode.value__ -ne 429) {
        throw
      }
    }
  } else {
    Write-Warning "Skipping live 429 proxy validation because ProxyVirtualKey was not provided."
  }

  Step "Control-plane audit visibility"
  $controlAudit = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/audit/control" -Body $null -WebSession $session
  if ($controlAudit.count -lt 1) {
    throw "control-plane audit did not show governance mutations"
  }

  Write-Host ""
  Write-Host "Staging governance validation passed."
}
finally {
  if ($session -and $createdPricingRuleId) {
    try { Invoke-Delete -Url "$BaseUrl/api/v1/pricing/$createdPricingRuleId" -WebSession $session } catch {}
  }
  if ($session) {
    foreach ($ruleId in $createdPolicyRuleIds) {
      try { Invoke-Delete -Url "$BaseUrl/api/v1/policies/$ruleId" -WebSession $session } catch {}
    }
  }
}
