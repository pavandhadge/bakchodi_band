param(
    [string]$Repo = $env:BAKCHODI_REPO,
    [string]$Version = $env:BAKCHODI_VERSION
)

$ErrorActionPreference = "Stop"

if (-not $Repo) { $Repo = "pavandhadge/bakchodi_band" }
if (-not $Version) { $Version = "latest" }

function Test-BakchodiInstall {
    param([string]$BinaryPath)

    Write-Host "Testing installation..."
    if (-not (Test-Path $BinaryPath)) {
        throw "Installed binary not found: $BinaryPath"
    }

    $usage = & $BinaryPath 2>&1 | Out-String
    if ($usage -notmatch "bakchodi_band") {
        Write-Host "Warning: bakchodi binary did not print expected usage text"
    }

    Start-Sleep -Seconds 2
    $svc = Get-Service -Name "bakchodi_band"
    if ($svc.Status -ne "Running") {
        throw "bakchodi_band service is $($svc.Status), expected Running"
    }
    Write-Host "Test passed: bakchodi_band service is running"
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Error: Run PowerShell as Administrator"
    Write-Host "  iex ((New-Object System.Net.WebClient).DownloadString('https://raw.githubusercontent.com/$Repo/main/packaging/install.ps1'))"
    exit 1
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq "AMD64") { $arch = "amd64" }
elseif ($arch -eq "ARM64") { $arch = "arm64" }

$asset = "bakchodi-windows-$arch"
if ($Version -eq "latest") {
    $url = "https://github.com/$Repo/releases/latest/download/$asset.exe"
} else {
    $url = "https://github.com/$Repo/releases/download/$Version/$asset.exe"
}

Write-Host "Downloading bakchodi for Windows ($arch)..."
$tempDir = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP "bakchodi-$(Get-Random)")
$binary = Join-Path $tempDir "bakchodi.exe"

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $binary

    $installDir = "$env:ProgramFiles\bakchodi_band"
    $dataDir = "$env:ProgramData\bakchodi_band"
    $dest = Join-Path $installDir "bakchodi.exe"

    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
    Copy-Item -Force $binary $dest

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

    Test-BakchodiInstall -BinaryPath $dest

    Write-Host ""
    Write-Host "Installation complete!"
    Write-Host "The bakchodi_band service is installed and configured to start automatically."
    Write-Host "Try: bakchodi block --url youtube.com"
}
finally {
    Remove-Item -Force -Recurse $tempDir -ErrorAction SilentlyContinue
}
