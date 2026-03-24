package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	defaultRPCURL        = "https://testnet.zkrypton.zkrypto.com"
	defaultArtifactsDir  = "build/kaia-preflight/contracts"
	defaultBundlePath    = "build/kaia-preflight/certification-bundle.json"
	defaultConfirmations = 1
	pollInterval         = 250 * time.Millisecond
	txTimeout            = 5 * time.Minute
	headerLookupTimeout  = 15 * time.Second
)

type g1Point struct {
	X *big.Int
	Y *big.Int
}

type g2Point struct {
	X [2]*big.Int
	Y [2]*big.Int
}

type verifyingKey struct {
	AlphaG1    g1Point
	BetaG2     g2Point
	GammaG2    g2Point
	DeltaG2    g2Point
	GammaAbcG1 [2]g1Point
}

type batchTransitionProof struct {
	A g1Point
	B g2Point
	C g1Point
	D g1Point
}

type batchTransitionArtifact struct {
	LaneId              uint16
	BatchId             uint64
	TxCount             uint32
	PrevStateCommitment g1Point
	NextStateCommitment g1Point
	Proof               batchTransitionProof
}

type jsonG1Point struct {
	X string `json:"x"`
	Y string `json:"y"`
}

type jsonG2Point struct {
	X [2]string `json:"x"`
	Y [2]string `json:"y"`
}

type jsonVerifyingKey struct {
	AlphaG1    jsonG1Point    `json:"alphaG1"`
	BetaG2     jsonG2Point    `json:"betaG2"`
	GammaG2    jsonG2Point    `json:"gammaG2"`
	DeltaG2    jsonG2Point    `json:"deltaG2"`
	GammaAbcG1 [2]jsonG1Point `json:"gammaAbcG1"`
}

type jsonBatchTransitionProof struct {
	A jsonG1Point `json:"a"`
	B jsonG2Point `json:"b"`
	C jsonG1Point `json:"c"`
	D jsonG1Point `json:"d"`
}

type jsonBatchTransitionArtifact struct {
	LaneID              uint16                   `json:"laneId"`
	BatchID             uint64                   `json:"batchId"`
	TxCount             uint32                   `json:"txCount"`
	PrevStateCommitment jsonG1Point              `json:"prevStateCommitment"`
	NextStateCommitment jsonG1Point              `json:"nextStateCommitment"`
	Proof               jsonBatchTransitionProof `json:"proof"`
}

type jsonLaneHead struct {
	LaneID      uint16      `json:"laneId"`
	InitialHead jsonG1Point `json:"initialHead"`
}

type jsonBundle struct {
	VerifyingKey jsonVerifyingKey `json:"verifyingKey"`
	LaneHeads    []jsonLaneHead   `json:"laneHeads"`
	Notes        struct {
		VerifiedTxTotal uint64 `json:"verified_tx_total"`
	} `json:"notes"`
	VerifyBatchesInput struct {
		Artifacts []jsonBatchTransitionArtifact `json:"artifacts"`
	} `json:"verifyBatchesInput"`
	VerifyTenBatchesInput struct {
		Artifacts []jsonBatchTransitionArtifact `json:"artifacts"`
	} `json:"verifyTenBatchesInput"`
}

type endpointSummary struct {
	RPCURL          string `json:"rpc_url"`
	ChainID         string `json:"chain_id"`
	NetworkID       string `json:"network_id"`
	GasPriceWei     string `json:"gas_price_wei"`
	LatestBlock     string `json:"latest_block"`
	LatestTimestamp string `json:"latest_timestamp_utc"`
	BaseFeeWei      string `json:"base_fee_wei,omitempty"`
}

type keySummary struct {
	Address       string `json:"address"`
	PrivateKeyHex string `json:"private_key_hex"`
}

type accountStatusSummary struct {
	RPCURL               string `json:"rpc_url"`
	Address              string `json:"address"`
	ChainID              string `json:"chain_id"`
	NetworkID            string `json:"network_id"`
	BalanceWei           string `json:"balance_wei"`
	BalanceKAIA          string `json:"balance_kaia"`
	Nonce                uint64 `json:"nonce"`
	PendingNonce         uint64 `json:"pending_nonce"`
	CodeSizeBytes        int    `json:"code_size_bytes"`
	SuggestedGasPriceWei string `json:"suggested_gas_price_wei"`
}

type txSummary struct {
	Action                   string `json:"action"`
	SenderAddress            string `json:"sender_address,omitempty"`
	TransactionHash          string `json:"transaction_hash"`
	ContractAddress          string `json:"contract_address,omitempty"`
	BlockNumber              string `json:"block_number,omitempty"`
	BlockTimestamp           string `json:"block_timestamp,omitempty"`
	GasLimit                 uint64 `json:"gas_limit"`
	GasUsed                  uint64 `json:"gas_used"`
	BlockInclusionLatencyMS  int64  `json:"block_inclusion_latency_ms,omitempty"`
	ReceiptLatencyMS         int64  `json:"receipt_latency_ms"`
	ReceiptVisibilityDelayMS int64  `json:"receipt_visibility_delay_ms,omitempty"`
	Confirmations            uint64 `json:"confirmations"`
	VerifiedTxTotal          uint64 `json:"verified_tx_total,omitempty"`
	TPS                      string `json:"tps,omitempty"`
	ReceiptVisibleTPS        string `json:"receipt_visible_tps,omitempty"`
	ValueWei                 string `json:"value_wei,omitempty"`
}

type sender struct {
	client        *ethclient.Client
	privateKey    *ecdsa.PrivateKey
	from          common.Address
	chainID       *big.Int
	gasPrice      *big.Int
	gasLimit      uint64
	confirmations uint64
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate-key":
		exitIfErr(runGenerateKey(os.Args[2:]))
	case "account-status":
		exitIfErr(runAccountStatus(os.Args[2:]))
	case "fund-accounts":
		exitIfErr(runFundAccounts(os.Args[2:]))
	case "discover":
		exitIfErr(runDiscover(os.Args[2:]))
	case "deploy-verifier":
		exitIfErr(runDeployVerifier(os.Args[2:]))
	case "init-vk":
		exitIfErr(runInitializeVerifyingKey(os.Args[2:]))
	case "deploy-certifier":
		exitIfErr(runDeployCertifier(os.Args[2:]))
	case "init-lane-heads":
		exitIfErr(runInitializeLaneHeads(os.Args[2:]))
	case "estimate-bundle":
		exitIfErr(runEstimateBundle(os.Args[2:]))
	case "submit-bundle":
		exitIfErr(runSubmitBundle(os.Args[2:]))
	case "submit-burst":
		exitIfErr(runSubmitBurst(os.Args[2:]))
	case "submit-multisender-burst":
		exitIfErr(runSubmitMultisenderBurst(os.Args[2:]))
	case "dashboard":
		exitIfErr(runDashboard(os.Args[2:]))
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: kaia-proofctl <command> [flags]

commands:
  generate-key
  account-status
  fund-accounts
  discover
  deploy-verifier
  init-vk
  deploy-certifier
  init-lane-heads
  estimate-bundle
  submit-bundle
  submit-burst
  submit-multisender-burst
  dashboard
`)
}

func exitIfErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func runDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return err
	}
	networkID, err := client.NetworkID(ctx)
	if err != nil {
		return err
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return err
	}

	summary := endpointSummary{
		RPCURL:          *rpcURL,
		ChainID:         chainID.String(),
		NetworkID:       networkID.String(),
		GasPriceWei:     gasPrice.String(),
		LatestBlock:     header.Number.String(),
		LatestTimestamp: time.Unix(int64(header.Time), 0).UTC().Format(time.RFC3339),
	}
	if header.BaseFee != nil {
		summary.BaseFeeWei = header.BaseFee.String()
	}
	return printJSON(summary)
}

func runGenerateKey(args []string) error {
	fs := flag.NewFlagSet("generate-key", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return err
	}
	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return errors.New("invalid generated private key")
	}

	return printJSON(keySummary{
		Address:       crypto.PubkeyToAddress(*publicKey).Hex(),
		PrivateKeyHex: hex.EncodeToString(crypto.FromECDSA(privateKey)),
	})
}

func runAccountStatus(args []string) error {
	fs := flag.NewFlagSet("account-status", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	addressText := fs.String("address", envOr("KAIA_SENDER", ""), "account address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	address, err := resolveAccountAddress(*addressText, *privateKeyHex)
	if err != nil {
		return err
	}

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		return err
	}
	networkID, err := client.NetworkID(ctx)
	if err != nil {
		return err
	}
	balance, err := client.BalanceAt(ctx, address, nil)
	if err != nil {
		return err
	}
	nonce, err := client.NonceAt(ctx, address, nil)
	if err != nil {
		return err
	}
	pendingNonce, err := client.PendingNonceAt(ctx, address)
	if err != nil {
		return err
	}
	code, err := client.CodeAt(ctx, address, nil)
	if err != nil {
		return err
	}
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return err
	}

	return printJSON(accountStatusSummary{
		RPCURL:               *rpcURL,
		Address:              address.Hex(),
		ChainID:              chainID.String(),
		NetworkID:            networkID.String(),
		BalanceWei:           balance.String(),
		BalanceKAIA:          weiToEtherString(balance),
		Nonce:                nonce,
		PendingNonce:         pendingNonce,
		CodeSizeBytes:        len(code),
		SuggestedGasPriceWei: gasPrice.String(),
	})
}

func runFundAccounts(args []string) error {
	fs := flag.NewFlagSet("fund-accounts", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	recipientsText := fs.String("recipients", "", "comma-separated recipient addresses")
	amountWeiText := fs.String("amount-wei", "", "funding amount in wei for each recipient")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}

	recipients, err := parseAddressList(*recipientsText)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return errors.New("missing --recipients")
	}

	amountWei, err := parseBigInt(*amountWeiText)
	if err != nil {
		return err
	}
	if amountWei == nil || amountWei.Sign() <= 0 {
		return errors.New("missing or invalid --amount-wei")
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	ctx := context.Background()
	nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
	if err != nil {
		return err
	}

	type pendingTx struct {
		to       common.Address
		tx       *types.Transaction
		gasLimit uint64
		sentAt   time.Time
	}

	pending := make([]pendingTx, 0, len(recipients))
	start := time.Now()
	for i, recipient := range recipients {
		signedTx, gasLimit, err := txSender.buildSignedValueTransaction(ctx, recipient, amountWei, nonce+uint64(i))
		if err != nil {
			return err
		}
		sentAt := time.Now()
		if err := txSender.client.SendTransaction(ctx, signedTx); err != nil {
			return err
		}
		pending = append(pending, pendingTx{
			to:       recipient,
			tx:       signedTx,
			gasLimit: gasLimit,
			sentAt:   sentAt,
		})
	}

	var end time.Time
	results := make([]txSummary, 0, len(pending))
	for _, item := range pending {
		receipt, err := waitForReceipt(ctx, txSender.client, item.tx.Hash(), txSender.confirmations)
		if err != nil {
			return err
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf(
				"fund transaction reverted: hash=%s block=%s gas_used=%d",
				item.tx.Hash().Hex(),
				receipt.BlockNumber.String(),
				receipt.GasUsed,
			)
		}
		end = time.Now()
		results = append(results, txSummary{
			Action:           "fund-account",
			SenderAddress:    txSender.from.Hex(),
			TransactionHash:  item.tx.Hash().Hex(),
			ContractAddress:  item.to.Hex(),
			BlockNumber:      receipt.BlockNumber.String(),
			GasLimit:         item.gasLimit,
			GasUsed:          receipt.GasUsed,
			ReceiptLatencyMS: time.Since(item.sentAt).Milliseconds(),
			Confirmations:    *confirmations,
			ValueWei:         amountWei.String(),
		})
	}
	if end.IsZero() {
		end = time.Now()
	}

	return printJSON(struct {
		Count        int         `json:"count"`
		WindowMS     int64       `json:"window_ms"`
		Transactions []txSummary `json:"transactions"`
	}{
		Count:        len(results),
		WindowMS:     end.Sub(start).Milliseconds(),
		Transactions: results,
	})
}

func runDeployVerifier(args []string) error {
	fs := flag.NewFlagSet("deploy-verifier", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	abiObj, binData, err := loadContractArtifact(*artifactsDir, "CCGroth16BatchVerifier")
	if err != nil {
		return err
	}
	constructor, err := abiObj.Pack("")
	if err != nil {
		return err
	}
	data := append(binData, constructor...)
	receipt, tx, latency, gasLimit, err := txSender.sendTransaction(context.Background(), nil, data)
	if err != nil {
		return err
	}

	return printJSON(txSummary{
		Action:           "deploy-verifier",
		TransactionHash:  tx.Hash().Hex(),
		ContractAddress:  receipt.ContractAddress.Hex(),
		BlockNumber:      receipt.BlockNumber.String(),
		GasLimit:         gasLimit,
		GasUsed:          receipt.GasUsed,
		ReceiptLatencyMS: latency.Milliseconds(),
		Confirmations:    *confirmations,
	})
}

func runInitializeVerifyingKey(args []string) error {
	fs := flag.NewFlagSet("init-vk", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	verifierAddress := fs.String("verifier-address", envOr("KAIA_VERIFIER_ADDRESS", ""), "deployed CCGroth16BatchVerifier address")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *verifierAddress == "" {
		return errors.New("missing --verifier-address")
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "CCGroth16BatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("initializeVerifyingKey", bundle.toVerifyingKey())
	if err != nil {
		return err
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	to := common.HexToAddress(*verifierAddress)
	receipt, tx, latency, gasLimit, err := txSender.sendTransaction(context.Background(), &to, data)
	if err != nil {
		return err
	}
	return printJSON(txSummary{
		Action:           "init-vk",
		TransactionHash:  tx.Hash().Hex(),
		BlockNumber:      receipt.BlockNumber.String(),
		GasLimit:         gasLimit,
		GasUsed:          receipt.GasUsed,
		ReceiptLatencyMS: latency.Milliseconds(),
		Confirmations:    *confirmations,
	})
}

func runDeployCertifier(args []string) error {
	fs := flag.NewFlagSet("deploy-certifier", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	verifierAddress := fs.String("verifier-address", envOr("KAIA_VERIFIER_ADDRESS", ""), "deployed CCGroth16BatchVerifier address")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *verifierAddress == "" {
		return errors.New("missing --verifier-address")
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	abiObj, binData, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	constructor, err := abiObj.Pack("", common.HexToAddress(*verifierAddress))
	if err != nil {
		return err
	}
	data := append(binData, constructor...)
	receipt, tx, latency, gasLimit, err := txSender.sendTransaction(context.Background(), nil, data)
	if err != nil {
		return err
	}

	return printJSON(txSummary{
		Action:           "deploy-certifier",
		TransactionHash:  tx.Hash().Hex(),
		ContractAddress:  receipt.ContractAddress.Hex(),
		BlockNumber:      receipt.BlockNumber.String(),
		GasLimit:         gasLimit,
		GasUsed:          receipt.GasUsed,
		ReceiptLatencyMS: latency.Milliseconds(),
		Confirmations:    *confirmations,
	})
}

func runInitializeLaneHeads(args []string) error {
	fs := flag.NewFlagSet("init-lane-heads", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	certifierAddress := fs.String("certifier-address", envOr("KAIA_CERTIFIER_ADDRESS", ""), "deployed SumPreservingBatchVerifier address")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	skipExisting := fs.Bool("skip-existing", true, "skip lane heads that are already initialized")
	bulk := fs.Bool("bulk", true, "initialize lane heads in a single transaction when possible")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *certifierAddress == "" {
		return errors.New("missing --certifier-address")
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	to := common.HexToAddress(*certifierAddress)
	var results []txSummary
	var skipped []uint16
	var laneIDs []uint16
	var initialHeads []g1Point
	for _, lane := range bundle.LaneHeads {
		if *skipExisting {
			currentHead, err := readLaneHead(context.Background(), txSender.client, to, abiObj, lane.LaneID)
			if err != nil {
				return err
			}
			if currentHead.X.Sign() != 0 || currentHead.Y.Sign() != 0 {
				skipped = append(skipped, lane.LaneID)
				continue
			}
		}

		laneIDs = append(laneIDs, lane.LaneID)
		initialHeads = append(initialHeads, toG1Point(lane.InitialHead))
	}

	if *bulk && len(laneIDs) != 0 {
		data, err := abiObj.Pack("initializeLaneHeads", laneIDs, initialHeads)
		if err != nil {
			return err
		}
		receipt, tx, latency, gasLimit, err := txSender.sendTransaction(context.Background(), &to, data)
		if err != nil {
			return err
		}
		results = append(results, txSummary{
			Action:           "init-lane-heads-bulk",
			TransactionHash:  tx.Hash().Hex(),
			BlockNumber:      receipt.BlockNumber.String(),
			GasLimit:         gasLimit,
			GasUsed:          receipt.GasUsed,
			ReceiptLatencyMS: latency.Milliseconds(),
			Confirmations:    *confirmations,
		})
	}

	if !*bulk {
		for i, laneID := range laneIDs {
			data, err := abiObj.Pack("initializeLaneHead", laneID, initialHeads[i])
			if err != nil {
				return err
			}
			receipt, tx, latency, gasLimit, err := txSender.sendTransaction(context.Background(), &to, data)
			if err != nil {
				return err
			}
			results = append(results, txSummary{
				Action:           "init-lane-head",
				TransactionHash:  tx.Hash().Hex(),
				BlockNumber:      receipt.BlockNumber.String(),
				GasLimit:         gasLimit,
				GasUsed:          receipt.GasUsed,
				ReceiptLatencyMS: latency.Milliseconds(),
				Confirmations:    *confirmations,
			})
		}
	}

	return printJSON(struct {
		Count        int         `json:"count"`
		SkippedLanes []uint16    `json:"skipped_lanes,omitempty"`
		Transactions []txSummary `json:"transactions"`
	}{
		Count:        len(results),
		SkippedLanes: skipped,
		Transactions: results,
	})
}

func runEstimateBundle(args []string) error {
	fs := flag.NewFlagSet("estimate-bundle", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	certifierAddress := fs.String("certifier-address", envOr("KAIA_CERTIFIER_ADDRESS", ""), "deployed SumPreservingBatchVerifier address")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *certifierAddress == "" {
		return errors.New("missing --certifier-address")
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	artifacts, verifiedTxTotal, err := bundle.toArtifacts()
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("verifyBatches", artifacts)
	if err != nil {
		return err
	}

	client, err := ethclient.Dial(*rpcURL)
	if err != nil {
		return err
	}
	defer client.Close()
	from, err := deriveSenderAddress(*privateKeyHex)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From: from,
		To:   ptrAddress(common.HexToAddress(*certifierAddress)),
		Data: data,
	})
	if err != nil {
		return err
	}

	return printJSON(struct {
		CertifierAddress string `json:"certifier_address"`
		EstimatedGas     uint64 `json:"estimated_gas"`
		VerifiedTxTotal  uint64 `json:"verified_tx_total"`
		GasPerVerifiedTx string `json:"gas_per_verified_tx"`
	}{
		CertifierAddress: *certifierAddress,
		EstimatedGas:     gasLimit,
		VerifiedTxTotal:  verifiedTxTotal,
		GasPerVerifiedTx: new(big.Rat).SetFrac(big.NewInt(int64(gasLimit)), big.NewInt(int64(verifiedTxTotal))).FloatString(6),
	})
}

func runSubmitBundle(args []string) error {
	fs := flag.NewFlagSet("submit-bundle", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	certifierAddress := fs.String("certifier-address", envOr("KAIA_CERTIFIER_ADDRESS", ""), "deployed SumPreservingBatchVerifier address")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *certifierAddress == "" {
		return errors.New("missing --certifier-address")
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	artifacts, verifiedTxTotal, err := bundle.toArtifacts()
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("verifyBatches", artifacts)
	if err != nil {
		return err
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), txTimeout)
	defer cancel()

	to := common.HexToAddress(*certifierAddress)
	nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
	if err != nil {
		return err
	}
	tx, gasLimit, err := txSender.buildSignedTransaction(ctx, &to, data, nonce)
	if err != nil {
		return err
	}
	sentAt := time.Now()
	if err := txSender.client.SendTransaction(ctx, tx); err != nil {
		return err
	}
	receipt, err := waitForReceipt(ctx, txSender.client, tx.Hash(), txSender.confirmations)
	if err != nil {
		return err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf(
			"transaction reverted: hash=%s block=%s gas_used=%d",
			tx.Hash().Hex(),
			receipt.BlockNumber.String(),
			receipt.GasUsed,
		)
	}
	receiptSeenAt := time.Now()
	includedAt, err := lookupIncludedAt(ctx, txSender.client, receipt)
	if err != nil {
		return err
	}
	blockInclusionLatencyMS := clampLatencyMS(includedAt.Sub(sentAt))
	receiptLatencyMS := receiptSeenAt.Sub(sentAt).Milliseconds()
	receiptVisibilityDelayMS := clampLatencyMS(receiptSeenAt.Sub(includedAt))
	return printJSON(txSummary{
		Action:                   "submit-bundle",
		TransactionHash:          tx.Hash().Hex(),
		BlockNumber:              receipt.BlockNumber.String(),
		BlockTimestamp:           includedAt.Format(time.RFC3339),
		GasLimit:                 gasLimit,
		GasUsed:                  receipt.GasUsed,
		BlockInclusionLatencyMS:  blockInclusionLatencyMS,
		ReceiptLatencyMS:         receiptLatencyMS,
		ReceiptVisibilityDelayMS: receiptVisibilityDelayMS,
		Confirmations:            *confirmations,
		VerifiedTxTotal:          verifiedTxTotal,
		TPS:                      calculateTPS(verifiedTxTotal, blockInclusionLatencyMS),
		ReceiptVisibleTPS:        calculateTPS(verifiedTxTotal, receiptLatencyMS),
	})
}

func runSubmitBurst(args []string) error {
	fs := flag.NewFlagSet("submit-burst", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeyHex := fs.String("private-key", envOr("KAIA_PRIVATE_KEY", ""), "hex private key")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	certifierAddressesText := fs.String("certifier-addresses", "", "comma-separated SumPreservingBatchVerifier addresses")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}

	addresses, err := parseAddressList(*certifierAddressesText)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errors.New("missing --certifier-addresses")
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	artifacts, verifiedTxTotal, err := bundle.toArtifacts()
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("verifyBatches", artifacts)
	if err != nil {
		return err
	}

	txSender, err := newSender(*rpcURL, *privateKeyHex, *gasPriceHex, *confirmations)
	if err != nil {
		return err
	}
	defer txSender.client.Close()

	ctx := context.Background()
	nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
	if err != nil {
		return err
	}

	type pendingTx struct {
		to       common.Address
		tx       *types.Transaction
		gasLimit uint64
		sentAt   time.Time
	}

	pending := make([]pendingTx, 0, len(addresses))
	start := time.Now()
	for i, address := range addresses {
		signedTx, gasLimit, err := txSender.buildSignedTransaction(ctx, &address, data, nonce+uint64(i))
		if err != nil {
			return err
		}
		sentAt := time.Now()
		if err := txSender.client.SendTransaction(ctx, signedTx); err != nil {
			return err
		}
		pending = append(pending, pendingTx{
			to:       address,
			tx:       signedTx,
			gasLimit: gasLimit,
			sentAt:   sentAt,
		})
	}

	var end time.Time
	var receiptEnd time.Time
	results := make([]txSummary, 0, len(pending))
	totalVerified := verifiedTxTotal * uint64(len(pending))
	for _, item := range pending {
		receipt, err := waitForReceipt(ctx, txSender.client, item.tx.Hash(), txSender.confirmations)
		if err != nil {
			return err
		}
		receiptSeenAt := time.Now()
		includedAt, err := lookupIncludedAt(ctx, txSender.client, receipt)
		if err != nil {
			return err
		}
		if receipt.BlockNumber != nil && includedAt.After(end) {
			end = includedAt
		}
		if receiptSeenAt.After(receiptEnd) {
			receiptEnd = receiptSeenAt
		}
		results = append(results, txSummary{
			Action:                   "submit-burst-item",
			TransactionHash:          item.tx.Hash().Hex(),
			ContractAddress:          item.to.Hex(),
			BlockNumber:              receipt.BlockNumber.String(),
			BlockTimestamp:           includedAt.Format(time.RFC3339),
			GasLimit:                 item.gasLimit,
			GasUsed:                  receipt.GasUsed,
			BlockInclusionLatencyMS:  clampLatencyMS(includedAt.Sub(item.sentAt)),
			ReceiptLatencyMS:         receiptSeenAt.Sub(item.sentAt).Milliseconds(),
			ReceiptVisibilityDelayMS: clampLatencyMS(receiptSeenAt.Sub(includedAt)),
			Confirmations:            *confirmations,
			VerifiedTxTotal:          verifiedTxTotal,
		})
	}
	if end.IsZero() {
		end = time.Now()
	}
	if receiptEnd.IsZero() {
		receiptEnd = time.Now()
	}

	elapsed := end.Sub(start)
	receiptElapsed := receiptEnd.Sub(start)

	return printJSON(struct {
		Count             int         `json:"count"`
		VerifiedTxTotal   uint64      `json:"verified_tx_total"`
		WindowMS          int64       `json:"window_ms"`
		TPS               string      `json:"tps"`
		ReceiptWindowMS   int64       `json:"receipt_window_ms"`
		ReceiptVisibleTPS string      `json:"receipt_visible_tps"`
		Transactions      []txSummary `json:"transactions"`
	}{
		Count:             len(results),
		VerifiedTxTotal:   totalVerified,
		WindowMS:          elapsed.Milliseconds(),
		TPS:               calculateTPS(totalVerified, elapsed.Milliseconds()),
		ReceiptWindowMS:   receiptElapsed.Milliseconds(),
		ReceiptVisibleTPS: calculateTPS(totalVerified, receiptElapsed.Milliseconds()),
		Transactions:      results,
	})
}

func runSubmitMultisenderBurst(args []string) error {
	fs := flag.NewFlagSet("submit-multisender-burst", flag.ContinueOnError)
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeysText := fs.String("private-keys", "", "comma-separated sender private keys")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	certifierAddressesText := fs.String("certifier-addresses", "", "comma-separated SumPreservingBatchVerifier addresses")
	bundlePath := fs.String("bundle", defaultBundlePath, "certification bundle json path")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	alignToNextBlock := fs.Bool("align-to-next-block", false, "wait for the next block before broadcasting the burst")
	alignTimeout := fs.Duration("align-timeout", 45*time.Second, "maximum time to wait for the next block when alignment is enabled")
	broadcastAfterIdle := fs.Duration("broadcast-after-idle", 0, "wait until no new block has appeared for at least this duration before broadcasting the burst")
	if err := fs.Parse(args); err != nil {
		return err
	}

	privateKeys := parseCSV(*privateKeysText)
	if len(privateKeys) == 0 {
		return errors.New("missing --private-keys")
	}
	addresses, err := parseAddressList(*certifierAddressesText)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errors.New("missing --certifier-addresses")
	}
	if len(privateKeys) != len(addresses) {
		return fmt.Errorf("private key count (%d) does not match certifier count (%d)", len(privateKeys), len(addresses))
	}

	bundle, err := loadBundle(*bundlePath)
	if err != nil {
		return err
	}
	artifacts, verifiedTxTotal, err := bundle.toArtifacts()
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(*artifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("verifyBatches", artifacts)
	if err != nil {
		return err
	}

	type senderJob struct {
		sender   *sender
		to       common.Address
		tx       *types.Transaction
		gasLimit uint64
		sentAt   time.Time
	}

	jobs := make([]senderJob, 0, len(addresses))
	for i, privateKeyHex := range privateKeys {
		txSender, err := newSender(*rpcURL, privateKeyHex, *gasPriceHex, *confirmations)
		if err != nil {
			for _, job := range jobs {
				job.sender.client.Close()
			}
			return err
		}
		ctx := context.Background()
		nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
		if err != nil {
			txSender.client.Close()
			for _, job := range jobs {
				job.sender.client.Close()
			}
			return err
		}
		signedTx, gasLimit, err := txSender.buildSignedTransaction(ctx, ptrAddress(addresses[i]), data, nonce)
		if err != nil {
			txSender.client.Close()
			for _, job := range jobs {
				job.sender.client.Close()
			}
			return err
		}
		jobs = append(jobs, senderJob{
			sender:   txSender,
			to:       addresses[i],
			tx:       signedTx,
			gasLimit: gasLimit,
		})
	}
	defer func() {
		for _, job := range jobs {
			job.sender.client.Close()
		}
	}()

	ctx := context.Background()
	if *alignToNextBlock {
		waitCtx, cancel := context.WithTimeout(ctx, *alignTimeout)
		defer cancel()
		if err := waitForNextBlock(waitCtx, jobs[0].sender.client); err != nil {
			return err
		}
	}
	if *broadcastAfterIdle > 0 {
		waitCtx, cancel := context.WithTimeout(ctx, *alignTimeout)
		defer cancel()
		if err := waitForIdleGap(waitCtx, jobs[0].sender.client, *broadcastAfterIdle); err != nil {
			return err
		}
	}
	start := time.Now()
	for i := range jobs {
		jobs[i].sentAt = time.Now()
		if err := jobs[i].sender.client.SendTransaction(ctx, jobs[i].tx); err != nil {
			return err
		}
	}

	var end time.Time
	var receiptEnd time.Time
	results := make([]txSummary, 0, len(jobs))
	for _, job := range jobs {
		receipt, err := waitForReceipt(ctx, job.sender.client, job.tx.Hash(), job.sender.confirmations)
		if err != nil {
			return err
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return fmt.Errorf(
				"transaction reverted: hash=%s block=%s gas_used=%d",
				job.tx.Hash().Hex(),
				receipt.BlockNumber.String(),
				receipt.GasUsed,
			)
		}
		receiptSeenAt := time.Now()
		includedAt, err := lookupIncludedAt(ctx, job.sender.client, receipt)
		if err != nil {
			includedAt = receiptSeenAt
		}
		if includedAt.After(end) {
			end = includedAt
		}
		if receiptSeenAt.After(receiptEnd) {
			receiptEnd = receiptSeenAt
		}
		results = append(results, txSummary{
			Action:                   "submit-multisender-burst-item",
			SenderAddress:            job.sender.from.Hex(),
			ContractAddress:          job.to.Hex(),
			TransactionHash:          job.tx.Hash().Hex(),
			BlockNumber:              receipt.BlockNumber.String(),
			BlockTimestamp:           includedAt.Format(time.RFC3339),
			GasLimit:                 job.gasLimit,
			GasUsed:                  receipt.GasUsed,
			BlockInclusionLatencyMS:  clampLatencyMS(includedAt.Sub(job.sentAt)),
			ReceiptLatencyMS:         receiptSeenAt.Sub(job.sentAt).Milliseconds(),
			ReceiptVisibilityDelayMS: clampLatencyMS(receiptSeenAt.Sub(includedAt)),
			Confirmations:            *confirmations,
			VerifiedTxTotal:          verifiedTxTotal,
		})
	}
	if end.IsZero() {
		end = time.Now()
	}
	if receiptEnd.IsZero() {
		receiptEnd = time.Now()
	}

	totalVerified := verifiedTxTotal * uint64(len(jobs))
	window := end.Sub(start)
	receiptWindow := receiptEnd.Sub(start)

	return printJSON(struct {
		Count                int         `json:"count"`
		VerifiedTxTotal      uint64      `json:"verified_tx_total"`
		WindowMS             int64       `json:"window_ms"`
		TPS                  string      `json:"tps"`
		AggregateBlockTPS    string      `json:"aggregate_block_tps"`
		ReceiptWindowMS      int64       `json:"receipt_window_ms"`
		ReceiptVisibleTPS    string      `json:"receipt_visible_tps"`
		AggregateReceiptTPS  string      `json:"aggregate_receipt_tps"`
		Transactions         []txSummary `json:"transactions"`
	}{
		Count:               len(results),
		VerifiedTxTotal:     totalVerified,
		WindowMS:            window.Milliseconds(),
		TPS:                 calculateTPS(totalVerified, window.Milliseconds()),
		AggregateBlockTPS:   calculateTPS(totalVerified, window.Milliseconds()),
		ReceiptWindowMS:     receiptWindow.Milliseconds(),
		ReceiptVisibleTPS:   calculateTPS(totalVerified, receiptWindow.Milliseconds()),
		AggregateReceiptTPS: calculateTPS(totalVerified, receiptWindow.Milliseconds()),
		Transactions:        results,
	})
}

func newSender(rpcURL, privateKeyHex, gasPriceText string, confirmations uint64) (*sender, error) {
	if privateKeyHex == "" {
		return nil, errors.New("missing private key; set --private-key or KAIA_PRIVATE_KEY")
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return nil, err
	}
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	gasPrice, err := parseBigInt(gasPriceText)
	if err != nil {
		client.Close()
		return nil, err
	}
	if gasPrice == nil {
		gasPrice, err = client.SuggestGasPrice(ctx)
		if err != nil {
			client.Close()
			return nil, err
		}
	}
	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		client.Close()
		return nil, errors.New("invalid private key")
	}
	return &sender{
		client:        client,
		privateKey:    privateKey,
		from:          crypto.PubkeyToAddress(*publicKey),
		chainID:       chainID,
		gasPrice:      gasPrice,
		gasLimit:      uint64(envOrInt("KAIA_GAS_LIMIT", 0)),
		confirmations: confirmations,
	}, nil
}

func (s *sender) sendTransaction(ctx context.Context, to *common.Address, data []byte) (*types.Receipt, *types.Transaction, time.Duration, uint64, error) {
	return s.sendTransactionValue(ctx, to, data, big.NewInt(0))
}

func (s *sender) sendTransactionValue(ctx context.Context, to *common.Address, data []byte, value *big.Int) (*types.Receipt, *types.Transaction, time.Duration, uint64, error) {
	ctx, cancel := context.WithTimeout(ctx, txTimeout)
	defer cancel()

	nonce, err := s.client.PendingNonceAt(ctx, s.from)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	signedTx, gasLimit, err := s.buildSignedTransactionValue(ctx, to, data, value, nonce)
	if err != nil {
		return nil, nil, 0, gasLimit, err
	}

	start := time.Now()
	if err := s.client.SendTransaction(ctx, signedTx); err != nil {
		return nil, nil, 0, gasLimit, err
	}
	receipt, err := waitForReceipt(ctx, s.client, signedTx.Hash(), s.confirmations)
	if err != nil {
		return nil, nil, 0, gasLimit, err
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, nil, 0, gasLimit, fmt.Errorf(
			"transaction reverted: hash=%s block=%s gas_used=%d",
			signedTx.Hash().Hex(),
			receipt.BlockNumber.String(),
			receipt.GasUsed,
		)
	}
	return receipt, signedTx, time.Since(start), gasLimit, nil
}

func (s *sender) buildSignedTransaction(ctx context.Context, to *common.Address, data []byte, nonce uint64) (*types.Transaction, uint64, error) {
	return s.buildSignedTransactionValue(ctx, to, data, big.NewInt(0), nonce)
}

func (s *sender) buildSignedValueTransaction(ctx context.Context, to common.Address, value *big.Int, nonce uint64) (*types.Transaction, uint64, error) {
	return s.buildSignedTransactionValue(ctx, ptrAddress(to), nil, value, nonce)
}

func (s *sender) buildSignedTransactionValue(ctx context.Context, to *common.Address, data []byte, value *big.Int, nonce uint64) (*types.Transaction, uint64, error) {
	gasLimit := s.gasLimit
	if gasLimit == 0 {
		estimatedGas, err := s.client.EstimateGas(ctx, ethereum.CallMsg{
			From:     s.from,
			To:       to,
			Value:    value,
			GasPrice: s.gasPrice,
			Data:     data,
		})
		if err != nil {
			return nil, 0, err
		}
		gasLimit = estimatedGas*12/10 + 25_000
	}

	var tx *types.Transaction
	if to == nil {
		tx = types.NewContractCreation(nonce, value, gasLimit, s.gasPrice, data)
	} else {
		tx = types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			To:       to,
			Value:    value,
			Gas:      gasLimit,
			GasPrice: s.gasPrice,
			Data:     data,
		})
	}

	signer := types.LatestSignerForChainID(s.chainID)
	signedTx, err := types.SignTx(tx, signer, s.privateKey)
	if err != nil {
		return nil, gasLimit, err
	}
	return signedTx, gasLimit, nil
}

func waitForReceipt(ctx context.Context, client *ethclient.Client, txHash common.Hash, confirmations uint64) (*types.Receipt, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			if confirmations <= 1 {
				return receipt, nil
			}
			for {
				header, headerErr := client.HeaderByNumber(ctx, nil)
				if headerErr != nil {
					return nil, headerErr
				}
				target := new(big.Int).Add(receipt.BlockNumber, new(big.Int).SetUint64(confirmations-1))
				if header.Number.Cmp(target) >= 0 {
					return receipt, nil
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ticker.C:
				}
			}
		}
		if err != nil && !strings.Contains(err.Error(), "not found") {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func calculateTPS(verifiedTxTotal uint64, elapsedMS int64) string {
	if elapsedMS <= 0 {
		return "0"
	}
	return new(big.Rat).SetFrac(
		new(big.Int).Mul(new(big.Int).SetUint64(verifiedTxTotal), big.NewInt(1000)),
		big.NewInt(elapsedMS),
	).FloatString(6)
}

func lookupIncludedAt(ctx context.Context, client *ethclient.Client, receipt *types.Receipt) (time.Time, error) {
	if receipt == nil || receipt.BlockNumber == nil {
		return time.Time{}, errors.New("receipt missing block number")
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, headerLookupTimeout)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		header, err := client.HeaderByNumber(deadlineCtx, receipt.BlockNumber)
		if err == nil {
			return time.Unix(int64(header.Time), 0), nil
		}
		if !strings.Contains(err.Error(), "not found") {
			return time.Time{}, err
		}
		select {
		case <-deadlineCtx.Done():
			return time.Time{}, deadlineCtx.Err()
		case <-ticker.C:
		}
	}
}

func waitForNextBlock(ctx context.Context, client *ethclient.Client) error {
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return err
	}
	current := new(big.Int).Set(head.Number)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		next, err := client.HeaderByNumber(ctx, nil)
		if err == nil && next.Number.Cmp(current) > 0 {
			return nil
		}
		if err != nil && !strings.Contains(err.Error(), "not found") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForIdleGap(ctx context.Context, client *ethclient.Client, idle time.Duration) error {
	if idle <= 0 {
		return nil
	}
	head, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		return err
	}
	current := new(big.Int).Set(head.Number)
	lastAdvance := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if time.Since(lastAdvance) >= idle {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			next, err := client.HeaderByNumber(ctx, nil)
			if err == nil && next.Number.Cmp(current) > 0 {
				current = new(big.Int).Set(next.Number)
				lastAdvance = time.Now()
				continue
			}
			if err != nil && !strings.Contains(err.Error(), "not found") {
				return err
			}
		}
	}
}

func clampLatencyMS(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

func loadContractArtifact(artifactsDir, contractName string) (abi.ABI, []byte, error) {
	abiPath := filepath.Join(artifactsDir, contractName+".abi")
	binPath := filepath.Join(artifactsDir, contractName+".bin")

	abiBytes, err := os.ReadFile(abiPath)
	if err != nil {
		return abi.ABI{}, nil, err
	}
	abiObj, err := abi.JSON(strings.NewReader(string(abiBytes)))
	if err != nil {
		return abi.ABI{}, nil, err
	}
	binBytes, err := os.ReadFile(binPath)
	if err != nil {
		return abi.ABI{}, nil, err
	}
	bytecode, err := hex.DecodeString(strings.TrimSpace(string(binBytes)))
	if err != nil {
		return abi.ABI{}, nil, err
	}
	return abiObj, bytecode, nil
}

func readLaneHead(ctx context.Context, client *ethclient.Client, certifier common.Address, abiObj abi.ABI, laneID uint16) (g1Point, error) {
	data, err := abiObj.Pack("laneHead", laneID)
	if err != nil {
		return g1Point{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := client.CallContract(callCtx, ethereum.CallMsg{
		To:   &certifier,
		Data: data,
	}, nil)
	if err != nil {
		return g1Point{}, err
	}
	if len(output) < 64 {
		return g1Point{}, fmt.Errorf("unexpected laneHead response length: %d", len(output))
	}
	return g1Point{
		X: new(big.Int).SetBytes(output[:32]),
		Y: new(big.Int).SetBytes(output[32:64]),
	}, nil
}

func loadBundle(path string) (*jsonBundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bundle jsonBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, err
	}
	return &bundle, nil
}

func (bundle *jsonBundle) toVerifyingKey() verifyingKey {
	return verifyingKey{
		AlphaG1:    toG1Point(bundle.VerifyingKey.AlphaG1),
		BetaG2:     toG2Point(bundle.VerifyingKey.BetaG2),
		GammaG2:    toG2Point(bundle.VerifyingKey.GammaG2),
		DeltaG2:    toG2Point(bundle.VerifyingKey.DeltaG2),
		GammaAbcG1: [2]g1Point{toG1Point(bundle.VerifyingKey.GammaAbcG1[0]), toG1Point(bundle.VerifyingKey.GammaAbcG1[1])},
	}
}

func (bundle *jsonBundle) toArtifacts() ([]batchTransitionArtifact, uint64, error) {
	source := bundle.VerifyBatchesInput.Artifacts
	if len(source) == 0 {
		source = bundle.VerifyTenBatchesInput.Artifacts
	}
	if len(source) == 0 {
		return nil, 0, errors.New("bundle does not contain artifacts")
	}

	result := make([]batchTransitionArtifact, 0, len(source))
	for _, artifact := range source {
		result = append(result, batchTransitionArtifact{
			LaneId:              artifact.LaneID,
			BatchId:             artifact.BatchID,
			TxCount:             artifact.TxCount,
			PrevStateCommitment: toG1Point(artifact.PrevStateCommitment),
			NextStateCommitment: toG1Point(artifact.NextStateCommitment),
			Proof: batchTransitionProof{
				A: toG1Point(artifact.Proof.A),
				B: toG2Point(artifact.Proof.B),
				C: toG1Point(artifact.Proof.C),
				D: toG1Point(artifact.Proof.D),
			},
		})
	}
	return result, bundle.Notes.VerifiedTxTotal, nil
}

func toG1Point(point jsonG1Point) g1Point {
	return g1Point{
		X: mustBigInt(point.X),
		Y: mustBigInt(point.Y),
	}
}

func toG2Point(point jsonG2Point) g2Point {
	return g2Point{
		X: [2]*big.Int{mustBigInt(point.X[0]), mustBigInt(point.X[1])},
		Y: [2]*big.Int{mustBigInt(point.Y[0]), mustBigInt(point.Y[1])},
	}
}

func mustBigInt(text string) *big.Int {
	value, ok := new(big.Int).SetString(strings.TrimSpace(text), 10)
	if !ok {
		panic("invalid big integer: " + text)
	}
	return value
}

func parseBigInt(text string) (*big.Int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	base := 10
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		text = text[2:]
		base = 16
	}
	value, ok := new(big.Int).SetString(text, base)
	if !ok {
		return nil, fmt.Errorf("invalid big integer: %s", text)
	}
	return value, nil
}

func deriveSenderAddress(privateKeyHex string) (common.Address, error) {
	if privateKeyHex == "" {
		return common.Address{}, errors.New("missing private key; set --private-key or KAIA_PRIVATE_KEY")
	}
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return common.Address{}, err
	}
	publicKey, ok := privateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, errors.New("invalid private key")
	}
	return crypto.PubkeyToAddress(*publicKey), nil
}

func resolveAccountAddress(addressText, privateKeyHex string) (common.Address, error) {
	if trimmed := strings.TrimSpace(addressText); trimmed != "" {
		if !common.IsHexAddress(trimmed) {
			return common.Address{}, fmt.Errorf("invalid address: %s", trimmed)
		}
		return common.HexToAddress(trimmed), nil
	}
	return deriveSenderAddress(privateKeyHex)
}

func parseAddressList(text string) ([]common.Address, error) {
	parts := parseCSV(text)
	addresses := make([]common.Address, 0, len(parts))
	for _, part := range parts {
		if !common.IsHexAddress(part) {
			return nil, fmt.Errorf("invalid address: %s", part)
		}
		addresses = append(addresses, common.HexToAddress(part))
	}
	return addresses, nil
}

func parseCSV(text string) []string {
	parts := strings.Split(text, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return values
}

func ptrAddress(address common.Address) *common.Address {
	return &address
}

func weiToEtherString(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	ratio := new(big.Rat).SetFrac(wei, big.NewInt(1_000_000_000_000_000_000))
	return ratio.FloatString(18)
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return fallback
	}
	return int(parsed.Int64())
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
