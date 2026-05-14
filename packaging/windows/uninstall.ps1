$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  Write-Error "Run PowerShell as Administrator, then run: .\packaging\windows\uninstall.ps1"
}

$service = Get-Service -Name "bakchodi_band" -ErrorAction SilentlyContinue
if ($service) {
  Stop-Service -Name "bakchodi_band" -ErrorAction SilentlyContinue
  sc.exe delete "bakchodi_band" | Out-Null
}

Remove-Item -Force -ErrorAction SilentlyContinue "$env:ProgramFiles\bakchodi_band\bakchodi.exe"

Write-Host "bakchodi_band service and binary removed."
Write-Host "User data is still in $env:ProgramData\bakchodi_band."
