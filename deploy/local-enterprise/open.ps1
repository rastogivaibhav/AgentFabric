$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$tokenPath = Join-Path $repo '.local\enterprise-stack\pul\admin-token'
if (-not (Test-Path -LiteralPath $tokenPath)) { throw 'Start the local stack before opening its consoles.' }
$pulToken = (Get-Content -LiteralPath $tokenPath -Raw).Trim()
$urls = @(
    'http://localhost:8090/',
    'http://localhost:13000/traces',
    'http://localhost:8080/',
    'http://localhost:3000/dashboard/sandboxes',
    ('http://localhost:8082/admin?token=' + [Uri]::EscapeDataString($pulToken))
)
foreach ($url in $urls) { Start-Process $url }
Write-Output 'Opened AgentFabric Agents, GovAgn, AGS, SPACE, and PUL.'
