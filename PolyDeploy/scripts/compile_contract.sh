#!/usr/bin/env bash
# Compile TextStorage.sol with solc and place the outputs in contracts/
# Requires: solc >= 0.8.20
# Install:  npm install -g solc
#           or: pip install solc-select && solc-select install 0.8.20 && solc-select use 0.8.20
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTRACTS="$ROOT/contracts"

echo "==> Compiling $CONTRACTS/TextStorage.sol …"

if ! command -v solc &>/dev/null; then
    echo "ERROR: solc not found in PATH."
    echo "  Install via npm:       npm install -g solc"
    echo "  Install via pip:       pip install solc-select && solc-select install 0.8.20 && solc-select use 0.8.20"
    exit 1
fi

mkdir -p "$CONTRACTS/build"

solc \
    --abi \
    --bin \
    --optimize \
    --optimize-runs 200 \
    --overwrite \
    -o "$CONTRACTS/build/" \
    "$CONTRACTS/TextStorage.sol"

cp "$CONTRACTS/build/TextStorage.bin" "$CONTRACTS/TextStorage.bin"
cp "$CONTRACTS/build/TextStorage.abi" "$CONTRACTS/TextStorage.abi"

echo "==> Done."
echo "    Bytecode : $CONTRACTS/TextStorage.bin"
echo "    ABI      : $CONTRACTS/TextStorage.abi"
