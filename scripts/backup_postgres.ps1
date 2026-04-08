param(
  [Parameter(Mandatory = $true)]
  [string]$DatabaseUrl,
  [string]$OutputDir = ".\\backups",
  [ValidateSet("custom", "plain", "directory", "tar")]
  [string]$Format = "custom",
  [int]$RetentionDays = 7
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command "pg_dump" -ErrorAction SilentlyContinue)) {
  throw "pg_dump was not found in PATH. Install PostgreSQL client tools first."
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$extension = switch ($Format) {
  "plain" { "sql" }
  "directory" { "dir" }
  "tar" { "tar" }
  default { "dump" }
}
$target = Join-Path $OutputDir ("govagn-" + $timestamp + "." + $extension)

& pg_dump $DatabaseUrl --format=$Format --file=$target
if ($LASTEXITCODE -ne 0) {
  throw "pg_dump failed with exit code $LASTEXITCODE"
}

Get-ChildItem -Path $OutputDir -File |
  Where-Object { $_.LastWriteTimeUtc -lt (Get-Date).ToUniversalTime().AddDays(-$RetentionDays) } |
  Remove-Item -Force -ErrorAction SilentlyContinue

Write-Host "Backup created at $target"
