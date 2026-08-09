// deploy/main.go
// Compiles deployment info, connects to Polygon, deploys TextStorage,
// and writes deployment.json (contract address + function selectors).
//
// You will be prompted to enter your private key (input is hidden).
//
// Optional environment variables:
//   RPC_URL       – defaults to https://polygon.drpc.org
//   INITIAL_TEXT  – text to store at deploy time  (default: "Hello from Polygon!")
//   BYTECODE_FILE – path to compiled .bin file    (default: contracts/TextStorage.bin)
//
// Run after compiling the contract:
//   cd go && go run ./cmd/deploy

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/term"

	"polydeploy/internal/textstorage"
)

// deploymentInfo is persisted to deployment.json so both the manager and
// the C++ client can find the contract without re-reading the chain.
type deploymentInfo struct {
	Address     string            `json:"address"`
	Network     string            `json:"network"`
	ChainID     int64             `json:"chainId"`
	Deployer    string            `json:"deployer"`
	BlockNumber uint64            `json:"blockNumber"`
	TxHash      string            `json:"txHash"`
	Selectors   map[string]string `json:"selectors"`
	DeployedAt  string            `json:"deployedAt"`
}

func main() {
	rpcURL := envOr("RPC_URL", "https://polygon.drpc.org")
	initialText := envOr("INITIAL_TEXT", "Hello from Polygon!")
	bytecodeFile := envOr("BYTECODE_FILE", "../contracts/TextStorage.bin")
	privateKeyHex := readPrivateKey()

	// ── Load compiled bytecode ────────────────────────────────────────────────
	raw, err := os.ReadFile(bytecodeFile)
	if err != nil {
		log.Fatalf("Cannot read bytecode from %s: %v\n"+
			"Run scripts\\compile_contract.bat first.", bytecodeFile, err)
	}
	binHex := strings.TrimSpace(strings.TrimPrefix(string(raw), "0x"))
	bytecode, err := hex.DecodeString(binHex)
	if err != nil {
		log.Fatalf("Invalid bytecode hex in %s: %v", bytecodeFile, err)
	}

	// ── Load private key ──────────────────────────────────────────────────────
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Invalid PRIVATE_KEY: %v", err)
	}
	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

	// ── Connect to RPC ────────────────────────────────────────────────────────
	fmt.Printf("Connecting to %s …\n", rpcURL)
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to RPC: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch chain ID: %v", err)
	}
	fmt.Printf("Chain ID : %s\n", chainID.String())

	// ── Build transaction signer ──────────────────────────────────────────────
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to build transactor: %v", err)
	}

	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		log.Fatalf("Failed to fetch nonce: %v", err)
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch gas price: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.GasPrice = gasPrice
	auth.GasLimit = 600_000 // generous limit for constructor with string arg

	// ── Parse ABI ─────────────────────────────────────────────────────────────
	parsedABI, err := abi.JSON(strings.NewReader(textstorage.ABI))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	// ── Deploy ────────────────────────────────────────────────────────────────
	fmt.Printf("Deploying TextStorage (initial text: %q) from %s …\n",
		initialText, fromAddress.Hex())

	address, tx, _, err := bind.DeployContract(auth, parsedABI, bytecode, client, initialText)
	if err != nil {
		log.Fatalf("Deployment transaction failed: %v", err)
	}
	fmt.Printf("Tx hash  : %s\n", tx.Hash().Hex())
	fmt.Printf("Address  : %s\n", address.Hex())
	fmt.Println("Waiting for the transaction to be mined …")

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		log.Fatalf("Error waiting for mining: %v", err)
	}
	fmt.Printf("Mined in block %d  (gas used: %d)\n",
		receipt.BlockNumber.Uint64(), receipt.GasUsed)

	// ── Collect function selectors for the C++ client ─────────────────────────
	selectors := make(map[string]string, len(parsedABI.Methods))
	for name, method := range parsedABI.Methods {
		selectors[name] = "0x" + hex.EncodeToString(method.ID)
	}

	// ── Write deployment.json ─────────────────────────────────────────────────
	info := deploymentInfo{
		Address:     address.Hex(),
		Network:     "polygon",
		ChainID:     chainID.Int64(),
		Deployer:    fromAddress.Hex(),
		BlockNumber: receipt.BlockNumber.Uint64(),
		TxHash:      tx.Hash().Hex(),
		Selectors:   selectors,
		DeployedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	// Write to repo root (one level up from go/)
	outPath := "../deployment.json"
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		// Fall back to current directory
		outPath = "deployment.json"
		if err2 := os.WriteFile(outPath, data, 0o644); err2 != nil {
			log.Fatalf("Failed to write deployment.json: %v", err2)
		}
	}
	fmt.Printf("\ndeployment.json written to %s\n", outPath)
	fmt.Println("Done.")
}

// readPrivateKey prompts the user for a private key on stderr with no echo
// and returns the trimmed hex string without a 0x prefix.
func readPrivateKey() string {
	fmt.Fprint(os.Stderr, "Enter private key (input hidden): ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // print newline after hidden input
	if err != nil {
		log.Fatalf("Failed to read private key: %v", err)
	}
	hexKey := strings.TrimSpace(strings.TrimPrefix(string(raw), "0x"))
	if hexKey == "" {
		log.Fatal("Private key cannot be empty.")
	}
	return hexKey
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
