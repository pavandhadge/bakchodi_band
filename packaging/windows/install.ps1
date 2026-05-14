param(
  [string]$Binary = ".\bakchodi.exe"
)

$ErrorActionPreference = "Stop"

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  Write-Error "Run PowerShell as Administrator, then run: .\packaging\windows\install.ps1"
}

if (-not (Test-Path $Binary)) {
  Write-Error "Binary not found: $Binary. Build it first: go build -o bakchodi.exe ./cmd"
}

$installDir = "$env:ProgramFiles\bakchodi_band"
$dataDir = "$env:ProgramData\bakchodi_band"
$dest = Join-Path $installDir "bakchodi.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
Copy-Item -Force $Binary $dest

$service = Get-Service -Name "bakchodi_band" -ErrorAction SilentlyContinue
if ($service) {
  Stop-Service -Name "bakchodi_band" -ErrorAction SilentlyContinue
  sc.exe delete "bakchodi_band" | Out-Null
  Start-Sleep -Seconds 1
}

New-Service `
  -Name "bakchodi_band" `
  -BinaryPathName "`"$dest`" band" `
  -DisplayName "bakchodi_band" `
  -Description "Local attention blocker daemon" `
  -StartupType Automatic

Start-Service -Name "bakchodi_band"

Write-Host "bakchodi_band installed and started."
Write-Host "Check status: Get-Service bakchodi_band"
