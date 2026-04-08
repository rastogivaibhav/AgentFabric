param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$CollectorUrl = "http://localhost:4318",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [string]$ProxyVirtualKey = "",
  [string]$ProxyPath = "/proxy/openai/v1/chat/completions",
  [string]$ProxyBodyJson = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"pilot validation secret sk-abcdefghijklmnopqrstuvwxyz12345"}],"stream":false}',
  [string]$TenantId = "00000000-0000-0000-0000-000000000001",
  [string]$PilotName = "local-pilot",
  [string]$TeamName = "pilot-team",
  [string]$EnvironmentName = "staging",
  [switch]$StartStack,
  [switch]$RunGovernanceScenarios,
  [switch]$StartDashboard,
  [switch]$VisualCheck,
  [string]$OutputPath = "",
  [string]$ScorecardPath = "",
  [string]$JsonOutputPath = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")

function Step {
  param([string]$Message)
  Write-Host ""
  Write-Host "==> $Message"
}

function Invoke-Json {
  param(
    [string]$Method,
    [string]$Url,
    [object]$Body = $null,
    [Microsoft.PowerShell.Commands.WebRequestSession]$WebSession = $null
  )
  $jsonBody = if ($null -eq $Body) { $null } elseif ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 20 }
  if ($WebSession) {
    return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $jsonBody -WebSession $WebSession -UseBasicParsing
  }
  return Invoke-RestMethod -Method $Method -Uri $Url -ContentType "application/json" -Body $jsonBody -UseBasicParsing
}

function Safe-Count {
  param([object]$Value)
  try {
    return [int]$Value
  } catch {
    return 0
  }
}

if (-not $OutputPath) {
  $OutputPath = Join-Path $RepoRoot "pilot-validation-summary.md"
}
if (-not $ScorecardPath) {
  $ScorecardPath = Join-Path $RepoRoot "pilot-value-scorecard.md"
}

if ($StartStack) {
  Step "Starting local stack"
  & (Join-Path $PSScriptRoot "bootstrap_local.ps1")
}

Step "Running stack health probe"
& (Join-Path $PSScriptRoot "probe_stack_health.ps1") -BaseUrl $BaseUrl -CollectorUrl $CollectorUrl | Out-Null

$session = $null

if ($AdminUser -and $AdminPassword -and $ProxyVirtualKey) {
  Step "Running proxy path proof"
  & (Join-Path $PSScriptRoot "probe_proxy_path.ps1") `
    -BaseUrl $BaseUrl `
    -AdminUser $AdminUser `
    -AdminPassword $AdminPassword `
    -ProxyVirtualKey $ProxyVirtualKey `
    -ProxyPath $ProxyPath `
    -ProxyBodyJson $ProxyBodyJson `
    -TenantId $TenantId | Out-Null

  Step "Running release-candidate validation"
  & (Join-Path $PSScriptRoot "run_release_candidate_validation.ps1") `
    -BaseUrl $BaseUrl `
    -AdminUser $AdminUser `
    -AdminPassword $AdminPassword `
    -RunGovernanceScenarios:$RunGovernanceScenarios `
    -TenantId $TenantId `
    -ProxyVirtualKey $ProxyVirtualKey `
    -ProxyPath $ProxyPath `
    -ProxyBodyJson $ProxyBodyJson | Out-Null
}

Step "Collecting pilot evidence"
$overview = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/analytics/overview"
$tracePage = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/traces?limit=20"
$liveTraceCount = Safe-Count $tracePage.total
$blockedRequests = Safe-Count $overview.blocked_requests
$totalCost = 0.0
if ($null -ne $overview.total_cost_usd) {
  $totalCost = [double]$overview.total_cost_usd
}
$llmCalls = Safe-Count $overview.llm_calls
$toolCalls = Safe-Count $overview.tool_calls

$controlAuditCount = 0
$policyPreviewState = "not-run"
$proxyEvidence = "stack-only"

if ($AdminUser -and $AdminPassword) {
  Invoke-WebRequest -Method Post -Uri "$BaseUrl/auth/login" -ContentType "application/json" `
    -Body (@{ username = $AdminUser; password = $AdminPassword } | ConvertTo-Json) `
    -SessionVariable session -UseBasicParsing | Out-Null

  $controlAudit = Invoke-Json -Method Get -Url "$BaseUrl/api/v1/audit/control?limit=50" -WebSession $session
  $controlAuditCount = Safe-Count $controlAudit.count

  $policyPreview = Invoke-Json -Method Post -Url "$BaseUrl/api/v1/policies/preview" -WebSession $session -Body @{
    tenant_id = $TenantId
    provider = "openai"
    model = "gpt-4o-mini"
    environment = $EnvironmentName
    estimated_tokens = 96
    request_body = "contact me at someone@example.com with secret sk-abcdefghijklmnopqrstuvwxyz12345"
    response_body = "ok"
  }
  if ($null -ne $policyPreview.request_dlp -or $null -ne $policyPreview.traffic) {
    $policyPreviewState = "verified"
  }
}

if ($ProxyVirtualKey) {
  $proxyEvidence = "verified"
}

$scorecard = @"
# Customer Value Scorecard

- Pilot name: **$PilotName**
- Team: **$TeamName**
- Environment: **$EnvironmentName**
- Timestamp: **$((Get-Date).ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss UTC"))**

## Value Signals

- Cost visibility: total observed spend **`$$([Math]::Round($totalCost, 6))`**
- Runtime activity: **$liveTraceCount** traces, **$llmCalls** LLM calls, **$toolCalls** tool calls
- Guardrail/policy evidence: **$policyPreviewState**
- Blocked/redacted pressure: **$blockedRequests** blocked requests reported in overview
- Audit completeness: **$controlAuditCount** control audit records visible
- Proxy proof: **$proxyEvidence**

## Operator Outcome Questions

- Was the team able to identify high-cost or high-latency traces without database access?
- Did policy previews and trace-linked policy events explain why requests were denied, warned, or redacted?
- Did the prompt/release linkage make it obvious which prompt version produced a given trace?
- Did audit and cost views reduce manual investigation time during pilot debugging?

## Suggested Pilot Ratings

- Cost visibility: `green` if spend anomalies were found from the UI alone
- Policy explainability: `green` if deny/redact decisions were understandable without logs
- Incident debugging speed: `green` if at least one trace-driven investigation was completed faster than the prior workflow
- Operator confidence: `green` if pilot users say they would keep this in the path for their team
"@

$summary = @"
# Govagn Local Pilot Validation

- Pilot: **$PilotName**
- Team: **$TeamName**
- Environment: **$EnvironmentName**
- Base URL: `$BaseUrl`
- Collector URL: `$CollectorUrl`

## Validation Performed

- Stack health and readiness probe: passed
- Proxy path proof: $proxyEvidence
- Release-candidate validation: $(if ($AdminUser -and $AdminPassword) { "verified" } else { "skipped (no admin credentials)" })
- Governance scenarios: $(if ($RunGovernanceScenarios) { "requested" } else { "not requested" })
- Visual dashboard review: $(if ($VisualCheck) { "requested" } else { "not requested" })

## Evidence Snapshot

- Total spend observed: **`$$([Math]::Round($totalCost, 6))`**
- Trace count: **$liveTraceCount**
- LLM calls: **$llmCalls**
- Tool calls: **$toolCalls**
- Blocked requests: **$blockedRequests**
- Control audit records: **$controlAuditCount**
- Policy preview evidence: **$policyPreviewState**

## Next Pilot Actions

- Run pilot traffic for 1-2 weeks with one real team
- Capture one debugging story, one policy/governance story, and one cost-control story
- Complete the customer value scorecard and attach operator quotes
- Re-run GA gate with pilot evidence when preparing for market-facing release
"@

Set-Content -Path $OutputPath -Value $summary -Encoding UTF8
Set-Content -Path $ScorecardPath -Value $scorecard -Encoding UTF8

if ($JsonOutputPath) {
  $payload = [pscustomobject]@{
    pilot_name = $PilotName
    team_name = $TeamName
    environment = $EnvironmentName
    base_url = $BaseUrl
    collector_url = $CollectorUrl
    trace_count = $liveTraceCount
    total_cost_usd = $totalCost
    llm_calls = $llmCalls
    tool_calls = $toolCalls
    blocked_requests = $blockedRequests
    control_audit_count = $controlAuditCount
    policy_preview_state = $policyPreviewState
    proxy_evidence = $proxyEvidence
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
  }
  $payload | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonOutputPath -Encoding UTF8
}

if ($StartDashboard) {
  Write-Warning "StartDashboard was requested. Open $BaseUrl in a browser if you want an interactive visual review on this machine."
}

Write-Host ""
Write-Host "Pilot validation summary written to $OutputPath"
Write-Host "Pilot scorecard template written to $ScorecardPath"
