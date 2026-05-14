$ErrorActionPreference = "Stop"

$Repo = $env:BAKCHODI_REPO
if (-not $Repo) { $Repo = "pavandhadge/bakchodi_band" }

$Branch = $env:BAKCHODI_BRANCH
if (-not $Branch) { $Branch = "main" }

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Error: Run PowerShell as Administrator"
    Write-Host "  iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/$Repo/$Branch/packaging/install.ps1'))"
    exit 1
}

Write-Host "Installing bakchodi from $Repo (branch: $Branch)..."
$script = Join-Path $env:TEMP "bakchodi-install.ps1"
Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/$Repo/$Branch/packaging/install.ps1" -OutFile $script
& $script
Remove-Item $script -Force
