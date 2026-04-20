param(
  [string]$BaseUrl = "http://localhost:8080",
  [string]$CollectorUrl = "http://localhost:4318",
  [string]$AdminUser = "",
  [string]$AdminPassword = "",
  [string]$ProxyVirtualKey = "",
  [string]$ProxyPath = "/proxy/openai/v1/chat/completions",
  [string]$ProxyBodyJson = '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"production deployment validation"}],"stream":false}',
  [string]$TenantId = "00000000-0000-0000-0000-000000000001",
  [string]$DatabaseUrl = "",
  [string]$BackupOutputDir = ".\\backups\\production-validation",
  [string]$NetProxyCaCertFile = "",
  [string]$NetProxyCaKeyFile = "",
  [switch]$LiveStreamSingleReplica,
  [switch]$LiveStreamFanoutReady,
  [switch]$SkipPackagingSmoke,
  [switch]$SkipStackProbe,
  [switch]$SkipProxyProbe,
  [switch]$SkipCandidateValidation,
  [switch]$SkipBackupDrill,
  [switch]$SkipNetProxyCaDrill,
  [string]$OutputPath = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $OutputPath) {
  $OutputPath = Join-Path $RepoRoot "production-deployment-validation.md"
}

$results = New-Object System.Collections.Generic.List[object]

function Add-Result {
  param(
    [string]$Name,
    [bool]$Passed,
    [string]$Detail
  )
  $script:results.Add([pscustomobject]@{
      Name = $Name
      Passed = $Passed
      Detail = $Detail
  }) | Out-Null
}

function Invoke-Check {
  param(
    [string]$Name,
    [scriptblock]$Action
  )
  try {
    & $Action | Out-Null
    Add-Result -Name $Name -Passed $true -Detail "passed"
  } catch {
    Add-Result -Name $Name -Passed $false -Detail $_.Exception.Message
  }
}

$topologyDetail = ""
if ($LiveStreamFanoutReady) {
  $topologyDetail = "fan-out ready"
  Add-Result -Name "Live stream topology" -Passed $true -Detail $topologyDetail
} elseif ($LiveStreamSingleReplica) {
  $topologyDetail = "single-replica acknowledged"
  Add-Result -Name "Live stream topology" -Passed $true -Detail $topologyDetail
} else {
  $topologyDetail = "set -LiveStreamSingleReplica or -LiveStreamFanoutReady"
  Add-Result -Name "Live stream topology" -Passed $false -Detail $topologyDetail
}

if (-not $SkipPackagingSmoke) {
  Invoke-Check -Name "Compose render (local)" -Action {
    Push-Location $RepoRoot
    try {
      docker compose -f docker-compose.yml config | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Check -Name "Compose render (production overlay)" -Action {
    Push-Location $RepoRoot
    try {
      docker compose -f docker-compose.yml -f deploy/docker/docker-compose.prod.yml --env-file deploy/docker/.env.production.example config | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Check -Name "Helm lint" -Action {
    Push-Location $RepoRoot
    try {
      docker run --rm -v "${RepoRoot}:/work" -w /work alpine/helm:3.14.0 lint deploy/helm | Out-Null
    } finally {
      Pop-Location
    }
  }
  Invoke-Check -Name "Helm template" -Action {
    Push-Location $RepoRoot
    try {
      docker run --rm -v "${RepoRoot}:/work" -w /work alpine/helm:3.14.0 template govagn deploy/helm --set collector.image.tag=ga --set api.image.tag=ga --set portal.image.tag=ga | Out-Null
    } finally {
      Pop-Location
    }
  }
  Push-Location $RepoRoot
  try {
    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
      docker run --rm -v "${RepoRoot}:/work" -w /work alpine/helm:3.14.0 template govagn deploy/helm --set api.replicas=2 2>$null | Out-Null
      $guardExitCode = $LASTEXITCODE
    } finally {
      $ErrorActionPreference = $previousErrorAction
    }
    if ($guardExitCode -eq 0) {
      Add-Result -Name "Helm live-stream topology guard" -Passed $false -Detail "api.replicas=2 rendered successfully when it should fail"
    } else {
      Add-Result -Name "Helm live-stream topology guard" -Passed $true -Detail "api.replicas=2 render blocked as expected"
    }
  } finally {
    Pop-Location
  }
}

if (-not $SkipStackProbe) {
  Invoke-Check -Name "Stack probe" -Action {
    & (Join-Path $PSScriptRoot "probe_stack_health.ps1") -BaseUrl $BaseUrl -CollectorUrl $CollectorUrl | Out-Null
  }
}

if (-not $SkipProxyProbe) {
  if ($AdminUser -and $AdminPassword -and $ProxyVirtualKey) {
    Invoke-Check -Name "Proxy path proof" -Action {
      & (Join-Path $PSScriptRoot "probe_proxy_path.ps1") `
        -BaseUrl $BaseUrl `
        -AdminUser $AdminUser `
        -AdminPassword $AdminPassword `
        -ProxyVirtualKey $ProxyVirtualKey `
        -ProxyPath $ProxyPath `
        -ProxyBodyJson $ProxyBodyJson `
        -TenantId $TenantId | Out-Null
    }
  } else {
    Add-Result -Name "Proxy path proof" -Passed $false -Detail "AdminUser, AdminPassword, and ProxyVirtualKey are required"
  }
}

if (-not $SkipCandidateValidation) {
  if ($AdminUser -and $AdminPassword -and $ProxyVirtualKey) {
    Invoke-Check -Name "Release candidate validation" -Action {
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
  } else {
    Add-Result -Name "Release candidate validation" -Passed $false -Detail "AdminUser, AdminPassword, and ProxyVirtualKey are required"
  }
}

$backupArtifact = ""
if (-not $SkipBackupDrill) {
  if ($DatabaseUrl) {
    New-Item -ItemType Directory -Force -Path $BackupOutputDir | Out-Null
    try {
      & (Join-Path $PSScriptRoot "backup_postgres.ps1") -DatabaseUrl $DatabaseUrl -OutputDir $BackupOutputDir -Format custom -RetentionDays 7 | Out-Null
      $backupArtifact = Get-ChildItem -Path $BackupOutputDir -File | Sort-Object LastWriteTimeUtc | Select-Object -Last 1 -ExpandProperty FullName
      if ($backupArtifact) {
        Add-Result -Name "Backup drill" -Passed $true -Detail "backup created at $backupArtifact"
      } else {
        Add-Result -Name "Backup drill" -Passed $false -Detail "backup script completed without creating an artifact"
      }
    } catch {
      Add-Result -Name "Backup drill" -Passed $false -Detail $_.Exception.Message
    }
  } else {
    Add-Result -Name "Backup drill" -Passed $false -Detail "DatabaseUrl is required"
  }
}

$netProxyReport = Join-Path $RepoRoot "netproxy-ca-drill.md"
if (-not $SkipNetProxyCaDrill) {
  if ($NetProxyCaCertFile -and $NetProxyCaKeyFile) {
    Invoke-Check -Name "NetProxy CA drill" -Action {
      & (Join-Path $PSScriptRoot "exercise_netproxy_ca_backup_restore.ps1") `
        -NetProxyCaCertFile $NetProxyCaCertFile `
        -NetProxyCaKeyFile $NetProxyCaKeyFile `
        -OutputPath $netProxyReport | Out-Null
    }
  } else {
    Add-Result -Name "NetProxy CA drill" -Passed $false -Detail "NetProxyCaCertFile and NetProxyCaKeyFile are required"
  }
}

$failed = @($results | Where-Object { -not $_.Passed }).Count -gt 0
$validationResult = if ($failed) { "FAIL" } else { "PASS" }

$packagingNames = @(
  "Live stream topology",
  "Compose render (local)",
  "Compose render (production overlay)",
  "Helm lint",
  "Helm template",
  "Helm live-stream topology guard"
)
$candidateNames = @(
  "Stack probe",
  "Proxy path proof",
  "Release candidate validation",
  "Backup drill",
  "NetProxy CA drill"
)

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("# Govagn Production Deployment Validation") | Out-Null
$lines.Add("") | Out-Null
$lines.Add("- Validation result: $validationResult") | Out-Null
$lines.Add("- Generated at: $((Get-Date).ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss UTC"))") | Out-Null
$lines.Add("- Base URL: $BaseUrl") | Out-Null
$lines.Add("- Collector URL: $CollectorUrl") | Out-Null
$lines.Add("") | Out-Null
$lines.Add("## Packaging and Topology") | Out-Null
foreach ($result in $results | Where-Object { $packagingNames -contains $_.Name }) {
  $icon = if ($result.Passed) { "[PASS]" } else { "[FAIL]" }
  $lines.Add("- $icon $($result.Name): $($result.Detail)") | Out-Null
}
$lines.Add("") | Out-Null
$lines.Add("## Candidate Environment") | Out-Null
foreach ($result in $results | Where-Object { $candidateNames -contains $_.Name }) {
  $icon = if ($result.Passed) { "[PASS]" } else { "[FAIL]" }
  $lines.Add("- $icon $($result.Name): $($result.Detail)") | Out-Null
}
$lines.Add("") | Out-Null
$lines.Add("## Operator Notes") | Out-Null
$lines.Add("- Live stream topology: $topologyDetail") | Out-Null
if ($backupArtifact) {
  $lines.Add("- Backup artifact: $backupArtifact") | Out-Null
}
if (Test-Path $netProxyReport) {
  $lines.Add("- NetProxy CA drill report: $netProxyReport") | Out-Null
}

$summary = $lines -join "`n"
Set-Content -Path $OutputPath -Value $summary -Encoding UTF8
Write-Host "Production deployment validation summary written to $OutputPath"
if ($failed) {
  exit 1
}
