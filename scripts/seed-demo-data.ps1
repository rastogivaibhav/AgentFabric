param()

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$envFile = Join-Path $repoRoot ".env.local"

Push-Location $repoRoot
try {
  docker compose -f docker-compose.yml --env-file $envFile exec -T postgres `
    psql -U fabric -d agentfabric -f /seed/demo_seed.sql | Out-Null
  Write-Host "Demo pricing and policy rules seeded."
} finally {
  Pop-Location
}
