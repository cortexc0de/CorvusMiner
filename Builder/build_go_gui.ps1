# build_go_gui.ps1 - Build the CorvusMiner Go/Fyne builder GUI
#Requires -Version 5.0

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "[*] CorvusMiner - Go/Fyne Builder Compiler" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan

Set-Location $ScriptDir

# Check Go
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "[!] Go not found. Install from https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

Write-Host "[*] Downloading/tidying dependencies..." -ForegroundColor Yellow
go mod tidy
if ($LASTEXITCODE -ne 0) { Write-Host "[!] go mod tidy failed" -ForegroundColor Red; exit 1 }

# Ensure fyne tools CLI is installed (new location)
$GoPathBin = Join-Path (go env GOPATH) "bin"
$FyneBin = Join-Path $GoPathBin "fyne.exe"
if (-not (Test-Path $FyneBin)) {
    Write-Host "[*] Installing fyne tools CLI..." -ForegroundColor Yellow
    go install fyne.io/tools/cmd/fyne@latest
    if ($LASTEXITCODE -ne 0) { Write-Host "[!] Failed to install fyne CLI" -ForegroundColor Red; exit 1 }
}

$env:CGO_ENABLED = "1"

$IconPath = Join-Path $ScriptDir "logo.png"
if (-not (Test-Path $IconPath)) {
    # Also accept .jpg
    $IconPath = Join-Path $ScriptDir "logo.jpg"
}
$HasIcon = Test-Path $IconPath

# Locate fyne CLI - check GOPATH\bin (canonical location after install)
$GoPathBin = Join-Path (go env GOPATH) "bin"
$FyneBin = Join-Path $GoPathBin "fyne.exe"
if (-not (Test-Path $FyneBin)) {
    $fyneCmd = Get-Command fyne -ErrorAction SilentlyContinue
    if ($fyneCmd) { $FyneBin = $fyneCmd.Source } else { $FyneBin = $null }
}
if ($FyneBin -and $HasIcon) {
    Write-Host "[*] Building with fyne package (icon embedded)..." -ForegroundColor Yellow
    & "$FyneBin" package -os windows -icon "$IconPath" -name "CorvusMinerBuilder"
    if ($LASTEXITCODE -ne 0) { Write-Host "[!] fyne package failed" -ForegroundColor Red; exit 1 }
} else {
    if (-not $FyneBin) {
        Write-Host "[~] fyne CLI not found - building without icon." -ForegroundColor Yellow
        Write-Host "    To embed an icon: go install fyne.io/fyne/v2/cmd/fyne@latest" -ForegroundColor Gray
    } elseif (-not $HasIcon) {
        Write-Host "[~] No icon.png found - building without icon." -ForegroundColor Yellow
        Write-Host "    Place icon.png in the Builder folder to embed it." -ForegroundColor Gray
    }
    Write-Host "[*] Building GUI..." -ForegroundColor Yellow
    go build -ldflags="-H windowsgui" -o "CorvusMinerBuilder.exe" .
    if ($LASTEXITCODE -ne 0) { Write-Host "[!] Build failed" -ForegroundColor Red; exit 1 }
}

Write-Host "[+] Built: $ScriptDir\CorvusMinerBuilder.exe" -ForegroundColor Green
