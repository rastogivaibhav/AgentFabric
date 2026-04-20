param(
  [Parameter(Mandatory = $true)]
  [string]$NetProxyCaCertFile,
  [Parameter(Mandatory = $true)]
  [string]$NetProxyCaKeyFile,
  [string]$OutputDir = ".\\artifacts\\netproxy-ca-drill",
  [string]$OutputPath = "",
  [string]$JsonOutputPath = ""
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
if (-not $OutputPath) {
  $OutputPath = Join-Path $RepoRoot "netproxy-ca-drill.md"
}

if (-not (Test-Path $NetProxyCaCertFile)) {
  throw "NetProxy CA cert file not found: $NetProxyCaCertFile"
}
if (-not (Test-Path $NetProxyCaKeyFile)) {
  throw "NetProxy CA key file not found: $NetProxyCaKeyFile"
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$backupDir = Join-Path $OutputDir "backup-$stamp"
$restoreDir = Join-Path $OutputDir "restore-$stamp"
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
New-Item -ItemType Directory -Force -Path $restoreDir | Out-Null

$backupCert = Join-Path $backupDir "govagn-netproxy-ca.crt"
$backupKey = Join-Path $backupDir "govagn-netproxy-ca.key"
$restoreCert = Join-Path $restoreDir "govagn-netproxy-ca.crt"
$restoreKey = Join-Path $restoreDir "govagn-netproxy-ca.key"

Copy-Item -LiteralPath $NetProxyCaCertFile -Destination $backupCert -Force
Copy-Item -LiteralPath $NetProxyCaKeyFile -Destination $backupKey -Force
Copy-Item -LiteralPath $backupCert -Destination $restoreCert -Force
Copy-Item -LiteralPath $backupKey -Destination $restoreKey -Force

$sourceCertHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $NetProxyCaCertFile).Hash
$sourceKeyHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $NetProxyCaKeyFile).Hash
$restoreCertHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $restoreCert).Hash
$restoreKeyHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $restoreKey).Hash

if ($sourceCertHash -ne $restoreCertHash) {
  throw "restored cert hash mismatch"
}
if ($sourceKeyHash -ne $restoreKeyHash) {
  throw "restored key hash mismatch"
}

$cert = [System.Security.Cryptography.X509Certificates.X509Certificate2]::new($restoreCert)
$certSubject = $cert.Subject
$certIssuer = $cert.Issuer
$certFingerprint = $cert.Thumbprint
$certNotAfter = $cert.NotAfter.ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss UTC")

$summary = @"
# Govagn NetProxy CA Backup and Restore Drill

- Validation result: PASS
- Generated at: $((Get-Date).ToUniversalTime().ToString("yyyy-MM-dd HH:mm:ss UTC"))
- Source cert: $NetProxyCaCertFile
- Source key: $NetProxyCaKeyFile
- Backup artifact directory: $backupDir
- Restore verification directory: $restoreDir

## Certificate Identity

- Subject: $certSubject
- Issuer: $certIssuer
- SHA-256 fingerprint: $certFingerprint
- Not after: $certNotAfter

## Verification

- Source cert hash: $sourceCertHash
- Restored cert hash: $restoreCertHash
- Source key hash: $sourceKeyHash
- Restored key hash: $restoreKeyHash
- Cert/key restore integrity: verified

## Operator Notes

- This drill copies the persisted NetProxy CA files into a timestamped backup directory.
- It restores those copies into an isolated verification directory without mutating the live source files.
- Use this artifact as release evidence that backup and restore steps were exercised in the current cycle.
"@

Set-Content -Path $OutputPath -Value $summary -Encoding UTF8

if ($JsonOutputPath) {
  $payload = [pscustomobject]@{
    validation_result = "PASS"
    generated_at = (Get-Date).ToUniversalTime().ToString("o")
    source_cert = $NetProxyCaCertFile
    source_key = $NetProxyCaKeyFile
    backup_dir = $backupDir
    restore_dir = $restoreDir
    source_cert_hash = $sourceCertHash
    restored_cert_hash = $restoreCertHash
    source_key_hash = $sourceKeyHash
    restored_key_hash = $restoreKeyHash
    cert_subject = $certSubject
    cert_issuer = $certIssuer
    cert_fingerprint = $certFingerprint
    cert_not_after = $certNotAfter
  }
  $payload | ConvertTo-Json -Depth 10 | Set-Content -Path $JsonOutputPath -Encoding UTF8
}

Write-Host "NetProxy CA drill summary written to $OutputPath"
