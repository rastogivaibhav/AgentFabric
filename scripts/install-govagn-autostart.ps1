param(
  [string]$TaskName = "GovagnStackAutoStart",
  [switch]$RunNow
)

$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "start-govagn-stack.ps1"
if (-not (Test-Path $scriptPath)) {
  throw "Missing startup script: $scriptPath"
}

$currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$escapedScript = $scriptPath.Replace('"', '""')
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$escapedScript`""
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $currentUser
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

Register-ScheduledTask `
  -TaskName $TaskName `
  -Action $action `
  -Trigger $trigger `
  -Settings $settings `
  -Description "Auto-start Govagn stack after user sign-in." `
  -User $currentUser `
  -RunLevel Limited `
  -Force | Out-Null

Write-Host "Installed scheduled task '$TaskName' for user $currentUser."
Write-Host "Govagn will auto-start after every machine reboot when the user signs in."

if ($RunNow) {
  Start-ScheduledTask -TaskName $TaskName
  Write-Host "Triggered '$TaskName' once."
}
