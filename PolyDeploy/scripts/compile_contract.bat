@echo off
rem compile_contract.bat
rem Compile TextStorage.sol with solc on Windows.
rem Requires: solc >= 0.8.20
rem   Install via npm:   npm install -g solc  (runs as solcjs)
rem   Install via scoop: scoop install solidity  (runs as solc)
rem   Download binary:   https://github.com/ethereum/solidity/releases

setlocal enabledelayedexpansion

set "SCRIPT_DIR=%~dp0"
rem Strip trailing backslash from SCRIPT_DIR
if "%SCRIPT_DIR:~-1%"=="\" set "SCRIPT_DIR=%SCRIPT_DIR:~0,-1%"

rem Root is one level above scripts\
for %%I in ("%SCRIPT_DIR%\..") do set "ROOT_DIR=%%~fI"

set "CONTRACT_DIR=%ROOT_DIR%\contracts"
set "BUILD_DIR=%CONTRACT_DIR%\build"
set "SOL_FILE=%CONTRACT_DIR%\TextStorage.sol"

echo =^> Compiling %SOL_FILE% ...

rem Prefer native solc; fall back to solcjs (installed by npm install -g solc)
set "SOLC_CMD="
where solc   >nul 2>&1 && set "SOLC_CMD=solc"
if "%SOLC_CMD%"=="" (
    where solcjs >nul 2>&1 && set "SOLC_CMD=solcjs"
)
if "%SOLC_CMD%"=="" (
    echo ERROR: Neither solc nor solcjs found in PATH.
    echo Install options:
    echo   npm install -g solc          ^(installs solcjs^)
    echo   scoop install solidity       ^(installs solc^)
    echo   Download binary: https://github.com/ethereum/solidity/releases
    exit /b 1
)
echo Using compiler: %SOLC_CMD%

if not exist "%BUILD_DIR%" mkdir "%BUILD_DIR%"

%SOLC_CMD% ^
    --abi ^
    --bin ^
    --optimize ^
    --optimize-runs 200 ^
    --overwrite ^
    -o "%BUILD_DIR%" ^
    "%SOL_FILE%"

if errorlevel 1 (
    echo ERROR: %SOLC_CMD% exited with an error.
    exit /b 1
)

copy /y "%BUILD_DIR%\TextStorage.bin" "%CONTRACT_DIR%\TextStorage.bin" >nul
copy /y "%BUILD_DIR%\TextStorage.abi" "%CONTRACT_DIR%\TextStorage.abi" >nul

echo =^> Done.
echo     Bytecode : %CONTRACT_DIR%\TextStorage.bin
echo     ABI      : %CONTRACT_DIR%\TextStorage.abi

endlocal
