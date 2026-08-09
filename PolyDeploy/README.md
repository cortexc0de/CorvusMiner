# PolyDeploy

Deploy a text-storage smart contract to **Polygon mainnet**, update its content
with a Go manager, and read it from a standalone C++ client — all over the
public `https://polygon.drpc.org` RPC.

```
contracts/            Solidity source
go/                   Go module (deployer + manager)
  cmd/deploy/         Deploy the contract  → writes deployment.json
  cmd/manager/        Read / update stored text
  internal/textstorage/ Contract ABI constant
client/               C++ read-only client
  main.cpp
  CMakeLists.txt
  vcpkg.json
scripts/              Build helpers
  compile_contract.sh   (Linux / macOS / Git Bash)
  compile_contract.ps1  (Windows PowerShell)
```

---

## Prerequisites

| Tool | Purpose | Install |
|------|---------|---------|
| **solc ≥ 0.8.20** | Compile Solidity | `npm i -g solc` or [solidity releases](https://github.com/ethereum/solidity/releases) |
| **Go ≥ 1.21** | Deployer + manager | https://go.dev/dl/ |
| **CMake ≥ 3.16** | Build C++ client | https://cmake.org/download/ |
| **libcurl** | HTTP in C++ client | see below |
| **Git** | CMake FetchContent for nlohmann/json | https://git-scm.com |

### libcurl on Windows (choose one)
```powershell
# vcpkg (recommended)
vcpkg install curl

# Or via winget
winget install curl.curl
```

### libcurl on macOS / Linux
```bash
brew install curl          # macOS
sudo apt install libcurl4-openssl-dev   # Debian/Ubuntu
sudo dnf install libcurl-devel          # Fedora
```

---

## Step 1 — Compile the contract

**Windows (PowerShell)**
```powershell
.\scripts\compile_contract.ps1
```

**Linux / macOS / Git Bash**
```bash
bash scripts/compile_contract.sh
```

This generates `contracts/TextStorage.bin` and `contracts/TextStorage.abi`.

---

## Step 2 — Deploy to Polygon

> **Important:** Polygon mainnet (chain 137) requires real MATIC for gas.
> To test for free first use **Polygon Amoy testnet**:
> `RPC_URL=https://rpc-amoy.polygon.technology`
> and get free MATIC from https://faucet.polygon.technology

```powershell
# Windows PowerShell
$env:PRIVATE_KEY = "your_hex_private_key"
$env:INITIAL_TEXT = "Hello from Polygon!"
cd go
go mod tidy          # first run only — downloads go-ethereum
go run ./cmd/deploy
```

```bash
# Linux / macOS / Git Bash
export PRIVATE_KEY="your_hex_private_key"
export INITIAL_TEXT="Hello from Polygon!"
cd go
go mod tidy
go run ./cmd/deploy
```

On success a `deployment.json` is written to the **repo root**:
```json
{
  "address":     "0xABC…",
  "network":     "polygon",
  "chainId":     137,
  "deployer":    "0x…",
  "blockNumber": 12345678,
  "txHash":      "0x…",
  "selectors": {
    "getText":  "0x…",
    "setText":  "0x…",
    "owner":    "0x…"
  },
  "deployedAt": "2025-01-01T00:00:00Z"
}
```

---

## Step 3 — Read / update text with the Go manager

```powershell
# Read
cd go
go run ./cmd/manager get

# Update (only the deploying address can do this)
$env:PRIVATE_KEY = "your_hex_private_key"
go run ./cmd/manager set "New message stored on chain!"
```

---

## Step 4 — Build the C++ client

### With vcpkg (Windows, recommended)
```powershell
cd client
cmake -B build -DCMAKE_TOOLCHAIN_FILE="C:/vcpkg/scripts/buildsystems/vcpkg.cmake"
cmake --build build --config Release
```

### Without vcpkg (system libcurl)
```bash
cd client
cmake -B build
cmake --build build
```

CMake fetches `nlohmann/json` automatically; only `libcurl` must be installed.

---

## Step 5 — Run the C++ client

```powershell
# From the repo root (so it can find deployment.json)
.\client\build\Release\polydeploy-client.exe

# Or supply paths explicitly
.\client\build\Release\polydeploy-client.exe `
    --deployment .\deployment.json `
    --rpc        https://polygon.drpc.org
```

Expected output:
```
PolyDeploy Client
=================
Contract  : 0xABC…
Network   : polygon (chain 137)
RPC       : https://polygon.drpc.org
Selector  : getText() = 0x…

Querying contract…

Stored text
-----------
Hello from Polygon!
```

---

## Security notes

- **Never commit your private key.** `.env` and `.env.*` (not `.env.example`)
  are listed in `.gitignore` by convention.
- The C++ client is **read-only** — it only calls `eth_call`, which costs no gas
  and does not require a private key.
- Only the deploying wallet (`owner`) can call `setText`.
