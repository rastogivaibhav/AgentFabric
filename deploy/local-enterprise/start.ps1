param(
    [Parameter(Mandatory = $true)][string]$PulPackagePath,
    [Parameter(Mandatory = $true)][string]$GovAgnPath,
    [string]$WslDistribution = 'Ubuntu-24.04'
)
$ErrorActionPreference = 'Stop'

if (-not $env:SPACE_LOCAL_PASSWORD) { throw 'Set SPACE_LOCAL_PASSWORD before starting the stack.' }

$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$govagnCompose = Join-Path $GovAgnPath 'docker-compose.yml'
$override = Join-Path $PSScriptRoot 'govagn-space.override.yml'
if (-not (Test-Path -LiteralPath $govagnCompose)) { throw "GovAgn compose file not found: $govagnCompose" }
if (-not (Test-Path -LiteralPath $PulPackagePath)) { throw "PUL package directory not found: $PulPackagePath" }

$agsContainers = @('ags-sovereign-db-1','ags-sovereign-redis-1','ags-sovereign-ags-control-plane-1','ags-sovereign-ags-dashboard-1')
foreach ($name in $agsContainers) {
    $status = docker inspect $name --format '{{.State.Status}}' 2>$null
    if (-not $status) { throw "Required AGS container is absent: $name" }
    if ($status -ne 'running') { docker start $name | Out-Null }
}

$spaceContainers = @(docker ps -a --filter 'name=^/space-local-' --format '{{.Names}}')
if ($spaceContainers.Count -lt 6) { throw 'The SPACE Kind cluster is missing. Bootstrap it before starting AgentFabric.' }
foreach ($name in $spaceContainers) {
    if ((docker inspect $name --format '{{.State.Status}}') -ne 'running') { docker start $name | Out-Null }
}

docker compose -f $govagnCompose -f $override up -d postgres redis api-gateway collector portal prometheus alertmanager grafana jaeger | Out-Null

$repoWsl = (wsl.exe -d $WslDistribution -- wslpath -a $repo).Trim()
$pulWsl = (wsl.exe -d $WslDistribution -- wslpath -a (Resolve-Path $PulPackagePath).Path).Trim()
wsl.exe -d $WslDistribution -- env "PUL_PACKAGE_DIR=$pulWsl" bash "$repoWsl/deploy/local-enterprise/start-pul.sh"
if ($LASTEXITCODE -ne 0) { throw 'PUL did not start.' }
$previousWslenv = $env:WSLENV
try {
    $entries = @($previousWslenv -split ':' | Where-Object { $_ })
    if ($entries -notcontains 'SPACE_LOCAL_PASSWORD/u') { $entries += 'SPACE_LOCAL_PASSWORD/u' }
    $env:WSLENV = $entries -join ':'
    wsl.exe -d $WslDistribution -- bash "$repoWsl/deploy/local-enterprise/bootstrap-agent.sh"
    if ($LASTEXITCODE -ne 0) { throw 'Enterprise agent did not start.' }
} finally {
    $env:WSLENV = $previousWslenv
}

Write-Output 'AgentFabric enterprise agent stack is running at http://127.0.0.1:8090/'
