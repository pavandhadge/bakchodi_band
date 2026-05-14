$ErrorActionPreference = "Stop"

$Repo = $env:BAKCHODI_REPO
if (-not $Repo) {
  $Repo = "pavandhadge/bakchodi_band"
}

$Version = $env:BAKCHODI_VERSION
if (-not $Version) {
  $Version = "latest"
}

$Branch = $env:BAKCHODI_BRANCH
if (-not $Branch) {
  $Branch = "main"
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  Write-Error "Run PowerShell as Administrator."
}

if ($Version -eq "latest") {
  $BinaryUrl = "https://github.com/$Repo/releases/latest/download/bakchodi-windows-amd64.exe"
} else {
  $BinaryUrl = "https://github.com/$Repo/releases/download/$Version/bakchodi-windows-amd64.exe"
}

$TempDir = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP ("bakchodi-" + [guid]::NewGuid()))
$Binary = Join-Path $TempDir "bakchodi.exe"
$Installer = Join-Path $TempDir "install.ps1"

Invoke-WebRequest -UseBasicParsing -Uri $BinaryUrl -OutFile $Binary
Invoke-WebRequest -UseBasicParsing -Uri "https://raw.githubusercontent.com/$Repo/$Branch/packaging/windows/install.ps1" -OutFile $Installer

& $Installer -Binary $Binary
