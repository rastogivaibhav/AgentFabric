param(
  [switch]$SkipSeed
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repoRoot ".env.local"

function Require-Command {
  param([string]$Name)
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
    throw "Missing required command: $Name"
  }
}

function Wait-Http {
  param(
    [string]$Name,
    [string]$Url,
    [int]$TimeoutSeconds = 120
  )

  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  Write-Host -NoNewline ("Waiting for {0,-12}" -f $Name)
  while ((Get-Date) -lt $deadline) {
    try {
      Invoke-WebRequest -Uri $Url -UseBasicParsing | Out-Null
      Write-Host " ready"
      return
    } catch {
      Write-Host -NoNewline "."
      Start-Sleep -Seconds 2
    }
  }
  Write-Host " timeout"
  throw "Timed out waiting for $Name at $Url"
}

Require-Command docker

if (-not (Test-Path $envFile)) {
  @"
GV_ENV=development
GV_AUTH_DISABLED=true
GV_JWT_SECRET=dev-secret-change-in-production
GV_ADMIN_PASSWORD=admin
GV_VAULT_KEY=0000000000000000000000000000000000000000000000000000000000000000
GV_CORS_ORIGINS=http://localhost:3000,http://localhost:5173
"@ | Set-Content -Path $envFile
  Write-Host "Created $envFile"
}

Push-Location $repoRoot
try {
  Write-Host "Starting Govagn local stack..."
  docker compose -f docker-compose.yml --env-file $envFile up -d --build

  Wait-Http -Name "gateway" -Url "http://localhost:8080/healthz"
  Wait-Http -Name "collector" -Url "http://localhost:4318/healthz"

  if (-not $SkipSeed) {
    & (Join-Path $PSScriptRoot "seed-demo-data.ps1")
  }

  Write-Host ""
  Write-Host "Govagn local stack is ready."
  Write-Host "Gateway:      http://localhost:8080"
  Write-Host "Portal:       http://localhost:3000"
  Write-Host "Collector:    http://localhost:4318"
  Write-Host "Swagger UI:   http://localhost:8080/docs/swagger"
  Write-Host "Prometheus:   http://localhost:9090"
  Write-Host "Grafana:      http://localhost:9091"
  Write-Host "Jaeger:       http://localhost:16686"
} finally {
  Pop-Location
}
