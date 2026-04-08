param(
  [ValidateSet("ci", "ga")]
  [string]$Mode = $(if ($env:GA_GATE_MODE) { $env:GA_GATE_MODE } else { "ga" }),
  [string]$BaseUrl = "http://localhost:8080",
  [string]$CollectorUrl = "http://localhost:4318",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [string]$ProxyVirtualKey = "",
  [string]$ProxyPath = "/proxy/openai/v1/chat/completions",
  [string]$ProxyBodyJson = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ga gate validation"}],"stream":false}',
  [string]$TenantId = "00000000-0000-0000-0000-000000000001",
  [switch]$RequirePilotProof,
  [string]$PilotScorecardPath = "",
  [switch]$PilotReferenceReady,
  [int]$OpenP0Count = -1,
  [int]$OpenP1Count = -1,
  [switch]$CiGreen,
  [switch]$PackagingGreen,
  [switch]$OutputMarkdown,
  [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$SummaryLines = New-Object System.Collections.Generic.List[string]
$Checks = New-Object System.Collections.Generic.List[object]

function Add-Check {
  param(
    [string]$Name,
    [bool]$Passed,
    [string]$Detail,
    [bool]$Required = $true
  )
  $script:Checks.Add([pscustomobject]@{
      Name     = $Name
      Passed   = $Passed
      Detail   = $Detail
      Required = $Required
  }) | Out-Null
}

function Invoke-Required {
  param(
    [string]$Name,
    [scriptblock]$Action
  )
  try {
    & $Action
    Add-Check -Name $Name -Passed $true -Detail "passed"
  } catch {
    Add-Check -Name $Name -Passed $false -Detail $_.Exception.Message
  }
}

function Get-EnvBool {
  param([string]$Name)
  $value = [string][System.Environment]::GetEnvironmentVariable($Name)
  return $value -match '^(1|true|yes|pass|passed|success)$'
}

function Run-ProcessCheck {
  param(
    [string]$Name,
    [string]$Command
  )
  Invoke-Required -Name $Name -Action {
    & powershell -NoProfile -ExecutionPolicy Bypass -Command $Command | Out-Null
  }
}

function Test-DocsAlignment {
  $docsToCheck = @(
    (Join-Path $RepoRoot "README.md"),
    (Join-Path $RepoRoot "docs\PRODUCTION_CHECKLIST.md"),
    (Join-Path $RepoRoot "docs\RELEASE_BOUNDARIES.md"),
    (Join-Path $RepoRoot "docs\REFERENCE_DEPLOYMENT.md"),
    (Join-Path $RepoRoot "docs\PILOT_PLAYBOOK.md"),
    (Join-Path $RepoRoot "docs\CUSTOMER_VALUE_SCORECARD.md")
  )
  foreach ($doc in $docsToCheck) {
    if (-not (Test-Path $doc)) {
      throw "missing required docs file: $doc"
    }
  }

  $combined = ($docsToCheck | ForEach-Object { [System.IO.File]::ReadAllText($_) }) -join "`n"
  foreach ($provider in @("openai", "anthropic", "google")) {
    if ($combined -notmatch [regex]::Escape($provider)) {
      throw "docs do not mention supported provider '$provider'"
    }
  }
  foreach ($stale in @("af-core", "clickhouse-svc", "kafka-svc", "CLICKHOUSE_URL", "KAFKA_")) {
    if ($combined -match [regex]::Escape($stale)) {
      throw "docs still contain stale runtime reference '$stale'"
    }
  }
}

function Add-SummaryLine {
  param([string]$Line)
  $script:SummaryLines.Add($Line) | Out-Null
}

function Test-PilotEvidence {
  if ($PilotReferenceReady -or (Get-EnvBool "GA_PILOT_REFERENCE_READY")) {
    return @{ Passed = $true; Detail = "external pilot/reference evidence declared ready" }
  }
  if (-not $PilotScorecardPath) {
    return @{ Passed = $false; Detail = "set -PilotScorecardPath or -PilotReferenceReady for market-facing GA" }
  }
  if (-not (Test-Path $PilotScorecardPath)) {
    return @{ Passed = $false; Detail = "pilot scorecard not found at $PilotScorecardPath" }
  }
  $content = [System.IO.File]::ReadAllText((Resolve-Path $PilotScorecardPath))
  foreach ($needle in @("Cost visibility", "Policy", "Debugging", "Recommendation")) {
    if ($content -notmatch [regex]::Escape($needle)) {
      return @{ Passed = $false; Detail = "pilot scorecard is missing section '$needle'" }
    }
  }
  return @{ Passed = $true; Detail = "pilot scorecard present at $PilotScorecardPath" }
}

function Render-Summary {
  param([string]$Decision)

  $timestamp = (Get-Date).ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss 'UTC'")
  Add-SummaryLine "# Govagn GA Gate"
  Add-SummaryLine ""
  Add-SummaryLine "- Decision: **$Decision**"
  Add-SummaryLine "- Mode: $Mode"
  Add-SummaryLine "- Timestamp: $timestamp"
  Add-SummaryLine ""
  Add-SummaryLine "## Evidence"
  foreach ($check in $Checks) {
    $icon = if ($check.Passed) { "[PASS]" } else { "[FAIL]" }
    Add-SummaryLine "- $icon $($check.Name): $($check.Detail)"
  }
  if ($Mode -eq "ga") {
    Add-SummaryLine ""
    Add-SummaryLine "## Blockers"
    Add-SummaryLine "- Open P0 blockers: $OpenP0Count"
    Add-SummaryLine "- Open P1 blockers: $OpenP1Count"
  }

  $summary = ($SummaryLines -join "`n")
  $summary | Write-Host

  if ($OutputMarkdown -and $OutputPath) {
    Set-Content -Path $OutputPath -Value $summary -Encoding UTF8
  }
  if ($env:GITHUB_STEP_SUMMARY) {
    Add-Content -Path $env:GITHUB_STEP_SUMMARY -Value $summary
  }
}

Invoke-Required -Name "Docs alignment" -Action { Test-DocsAlignment }

if ($Mode -eq "ci") {
  $jobResults = @{
    "collector tests"      = [string]$env:GA_COLLECTOR_RESULT
    "api-gateway tests"    = [string]$env:GA_GATEWAY_RESULT
    "portal tests/build"   = [string]$env:GA_PORTAL_RESULT
    "portal playwright"    = [string]$env:GA_PORTAL_E2E_RESULT
    "agent-sdk tests"      = [string]$env:GA_SDK_RESULT
    "helm smoke"           = [string]$env:GA_HELM_RESULT
    "packaging smoke"      = [string]$env:GA_PACKAGING_RESULT
    "secret scan"          = [string]$env:GA_SECRET_SCAN_RESULT
  }

  foreach ($entry in $jobResults.GetEnumerator()) {
    $passed = $entry.Value -eq "success"
    Add-Check -Name $entry.Key -Passed $passed -Detail "result=$($entry.Value)"
  }

  $allRequiredPassed = @($Checks | Where-Object { $_.Required -and -not $_.Passed }).Count -eq 0
  $decision = if ($allRequiredPassed) { "CI PASS - staging evidence still required for GA" } else { "CI NO-GO" }
  Render-Summary -Decision $decision
  if (-not $allRequiredPassed) { exit 1 }
  exit 0
}

if (-not $CiGreen -and -not (Get-EnvBool "GA_CI_GREEN")) {
  Add-Check -Name "CI evidence" -Passed $false -Detail "set -CiGreen or GA_CI_GREEN=true after confirming latest CI is green"
} else {
  Add-Check -Name "CI evidence" -Passed $true -Detail "latest CI reported green"
}

if ($PackagingGreen -or (Get-EnvBool "GA_PACKAGING_GREEN")) {
  Add-Check -Name "Packaging evidence" -Passed $true -Detail "external packaging evidence marked green"
} else {
  Invoke-Required -Name "Compose render (local)" -Action {
    Push-Location $RepoRoot
    try {
      docker compose -f docker-compose.yml config | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Required -Name "Compose render (production overlay)" -Action {
    Push-Location $RepoRoot
    try {
      docker compose -f docker-compose.yml -f deploy/docker/docker-compose.prod.yml --env-file deploy/docker/.env.production.example config | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Required -Name "Helm lint" -Action {
    Push-Location $RepoRoot
    try {
      helm lint deploy/helm | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Required -Name "Helm template" -Action {
    Push-Location $RepoRoot
    try {
      helm template govagn deploy/helm --set collector.image.tag=ga --set api.image.tag=ga --set portal.image.tag=ga | Out-Null
    } finally {
      Pop-Location
    }
  }
}

Invoke-Required -Name "Stack probe" -Action {
  & (Join-Path $PSScriptRoot "probe_stack_health.ps1") -BaseUrl $BaseUrl -CollectorUrl $CollectorUrl | Out-Null
}

if (-not ($AdminUser -and $AdminPassword)) {
  Add-Check -Name "Admin credentials" -Passed $false -Detail "AdminUser/AdminPassword are required for GA mode"
} else {
  Add-Check -Name "Admin credentials" -Passed $true -Detail "provided"
}

if (-not $ProxyVirtualKey) {
  Add-Check -Name "Proxy virtual key" -Passed $false -Detail "ProxyVirtualKey is required for proxy proof and governance validation"
} else {
  Add-Check -Name "Proxy virtual key" -Passed $true -Detail "provided"
}

if ($AdminUser -and $AdminPassword -and $ProxyVirtualKey) {
  Invoke-Required -Name "Proxy path probe" -Action {
    & (Join-Path $PSScriptRoot "probe_proxy_path.ps1") `
      -BaseUrl $BaseUrl `
      -AdminUser $AdminUser `
      -AdminPassword $AdminPassword `
      -ProxyVirtualKey $ProxyVirtualKey `
      -ProxyPath $ProxyPath `
      -ProxyBodyJson $ProxyBodyJson `
      -TenantId $TenantId | Out-Null
  }

  Invoke-Required -Name "Release candidate validation" -Action {
    & (Join-Path $PSScriptRoot "run_release_candidate_validation.ps1") `
      -BaseUrl $BaseUrl `
      -AdminUser $AdminUser `
      -AdminPassword $AdminPassword `
      -RunGovernanceScenarios `
      -TenantId $TenantId `
      -ProxyVirtualKey $ProxyVirtualKey `
      -ProxyPath $ProxyPath `
      -ProxyBodyJson $ProxyBodyJson | Out-Null
  }
}

if ($OpenP0Count -lt 0 -or $OpenP1Count -lt 0) {
  Add-Check -Name "Release blockers declared" -Passed $false -Detail "OpenP0Count and OpenP1Count must be provided in GA mode"
} else {
  $blockersClear = ($OpenP0Count -eq 0 -and $OpenP1Count -eq 0)
  Add-Check -Name "Release blockers declared" -Passed $blockersClear -Detail "P0=$OpenP0Count P1=$OpenP1Count"
}

if ($RequirePilotProof -or (Get-EnvBool "GA_REQUIRE_PILOT_PROOF")) {
  $pilotCheck = Test-PilotEvidence
  Add-Check -Name "Pilot/reference proof" -Passed ([bool]$pilotCheck.Passed) -Detail ([string]$pilotCheck.Detail)
}

$allRequiredPassed = @($Checks | Where-Object { $_.Required -and -not $_.Passed }).Count -eq 0
$decision = if ($allRequiredPassed) { "GO" } else { "NO-GO" }
Render-Summary -Decision $decision
if (-not $allRequiredPassed) { exit 1 }
