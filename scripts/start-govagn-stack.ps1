param(
  [string]$ComposeFile = "docker-compose.yml",
  [int]$DockerWaitSeconds = 180
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$composePath = if ([System.IO.Path]::IsPathRooted($ComposeFile)) { $ComposeFile } else { Join-Path $repoRoot $ComposeFile }
$envFile = Join-Path $repoRoot ".env.local"
$logDir = Join-Path $repoRoot "artifacts"
$logFile = Join-Path $logDir "autostart.log"

New-Item -ItemType Directory -Force -Path $logDir | Out-Null

function Write-Log {
  param([string]$Message)
  $line = "[{0}] {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
  Add-Content -Path $logFile -Value $line
}

function Test-DockerReady {
  try {
    docker info | Out-Null
    return $true
  } catch {
    return $false
  }
}

function Start-DockerDesktopIfPresent {
  $candidates = @(
    (Join-Path $env:ProgramFiles "Docker\\Docker\\Docker Desktop.exe"),
    (Join-Path ${env:ProgramFiles(x86)} "Docker\\Docker\\Docker Desktop.exe")
  )
  foreach ($candidate in $candidates) {
    if ($candidate -and (Test-Path $candidate)) {
      Write-Log "Starting Docker Desktop from $candidate"
      Start-Process -FilePath $candidate -WindowStyle Hidden | Out-Null
      return
    }
  }
  Write-Log "Docker Desktop executable not found in standard locations; relying on existing daemon."
}

if (-not (Test-Path $composePath)) {
  throw "Compose file not found: $composePath"
}

Write-Log "Govagn autostart begin"
Write-Log "Using compose file: $composePath"

if (-not (Test-DockerReady)) {
  Start-DockerDesktopIfPresent
}

$deadline = (Get-Date).AddSeconds($DockerWaitSeconds)
while ((Get-Date) -lt $deadline) {
  if (Test-DockerReady) {
    break
  }
  Start-Sleep -Seconds 3
}

if (-not (Test-DockerReady)) {
  throw "Docker daemon did not become ready within $DockerWaitSeconds seconds."
}

Push-Location $repoRoot
try {
  $composeArgs = @("compose", "-f", $composePath)
  if (Test-Path $envFile) {
    $composeArgs += @("--env-file", $envFile)
  } else {
    Write-Log ".env.local not found at $envFile; continuing without explicit env file."
  }
  $composeArgs += @("up", "-d", "--remove-orphans")
  Write-Log ("Running: docker " + ($composeArgs -join " "))
  & docker @composeArgs | Out-Null
  Write-Log "Govagn stack started successfully."
} finally {
  Pop-Location
}
