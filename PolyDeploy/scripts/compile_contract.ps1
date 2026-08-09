# compile_contract.ps1
# Compile TextStorage.sol with solc on Windows.
# Requires: solc >= 0.8.20  (https://github.com/ethereum/solidity/releases)
#   or via npm:  npm install -g solc
#   or via scoop: scoop install solidity

$ErrorActionPreference = "Stop"

$ScriptDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir     = Split-Path -Parent $ScriptDir
$ContractDir = Join-Path $RootDir "contracts"
$BuildDir    = Join-Path $ContractDir "build"
$SolFile     = Join-Path $ContractDir "TextStorage.sol"

Write-Host "==> Compiling $SolFile ..."

if (-not (Get-Command solc -ErrorAction SilentlyContinue)) {
    Write-Error @"
solc not found in PATH.
Install options:
  npm install -g solc
  scoop install solidity
  Download binary: https://github.com/ethereum/solidity/releases
"@
    exit 1
}

New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

& solc `
    --abi `
    --bin `
    --optimize `
    --optimize-runs 200 `
    --overwrite `
    -o $BuildDir `
    $SolFile

if ($LASTEXITCODE -ne 0) {
    Write-Error "solc exited with code $LASTEXITCODE"
    exit $LASTEXITCODE
}

Copy-Item (Join-Path $BuildDir "TextStorage.bin") (Join-Path $ContractDir "TextStorage.bin") -Force
Copy-Item (Join-Path $BuildDir "TextStorage.abi") (Join-Path $ContractDir "TextStorage.abi") -Force

Write-Host "==> Done."
Write-Host "    Bytecode : $ContractDir\TextStorage.bin"
Write-Host "    ABI      : $ContractDir\TextStorage.abi"
