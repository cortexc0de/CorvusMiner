// manager/main.go
// Read or update the text stored in a deployed TextStorage contract.
//
// Usage:
//   cd go
//   go run ./cmd/manager get
//   go run ./cmd/manager set "New message"
//
// The 'set' command will prompt for your private key (input is hidden).
//
// Optional env:
//   RPC_URL          – defaults to https://polygon.drpc.org
//   DEPLOYMENT_FILE  – path to deployment.json (default: ../deployment.json)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/term"

	"polydeploy/internal/textstorage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	rpcURL := envOr("RPC_URL", "https://polygon.drpc.org")
	deploymentFile := envOr("DEPLOYMENT_FILE", "../deployment.json")

	// ── Load deployment.json ──────────────────────────────────────────────────
	raw, err := os.ReadFile(deploymentFile)
	if err != nil {
		// Try same directory as fallback
		raw, err = os.ReadFile("deployment.json")
		if err != nil {
			log.Fatalf("Cannot read deployment.json: %v\nRun the deployer first.", err)
		}
	}
	var dep struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(raw, &dep); err != nil {
		log.Fatalf("Malformed deployment.json: %v", err)
	}
	contractAddr := common.HexToAddress(dep.Address)

	// ── Connect to RPC ────────────────────────────────────────────────────────
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", rpcURL, err)
	}
	defer client.Close()

	parsedABI, err := abi.JSON(strings.NewReader(textstorage.ABI))
	if err != nil {
		log.Fatalf("Failed to parse ABI: %v", err)
	}

	bound := bind.NewBoundContract(contractAddr, parsedABI, client, client, client)

	switch cmd := os.Args[1]; cmd {
	case "get":
		cmdGet(bound)

	case "set":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: manager set <text>")
			os.Exit(1)
		}
		newText := strings.Join(os.Args[2:], " ")
		cmdSet(client, bound, newText)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// cmdGet calls getText() and prints the result.
func cmdGet(bound *bind.BoundContract) {
	var out []interface{}
	if err := bound.Call(
		&bind.CallOpts{Context: context.Background()},
		&out,
		"getText",
	); err != nil {
		log.Fatalf("getText() failed: %v", err)
	}

	text, ok := out[0].(string)
	if !ok {
		log.Fatalf("Unexpected return type: %T", out[0])
	}
	fmt.Printf("Stored text: %s\n", text)
}

// cmdSet calls setText(newText) and waits for mining.
func cmdSet(client *ethclient.Client, bound *bind.BoundContract, newText string) {
	privateKeyHex := readPrivateKey()
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Invalid PRIVATE_KEY: %v", err)
	}
	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

	ctx := context.Background()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch chain ID: %v", err)
	}
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
	auth.GasLimit = 100_000

	fmt.Printf("Sending setText(%q) …\n", newText)
	tx, err := bound.Transact(auth, "setText", newText)
	if err != nil {
		log.Fatalf("setText() failed: %v", err)
	}
	fmt.Printf("Tx hash: %s\n", tx.Hash().Hex())
	fmt.Println("Waiting for mining …")

	receipt, err := bind.WaitMined(ctx, client, tx)
	if err != nil {
		log.Fatalf("Error waiting for mining: %v", err)
	}
	fmt.Printf("Mined in block %d. Text updated to: %q\n",
		receipt.BlockNumber.Uint64(), newText)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  manager get")
	fmt.Fprintln(os.Stderr, "  manager set <new text>")
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
