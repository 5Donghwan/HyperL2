package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type dashboardSnapshot struct {
	Title      string              `json:"title"`
	Phase      string              `json:"phase"`
	Done       bool                `json:"done"`
	Error      string              `json:"error,omitempty"`
	StartedAt  string              `json:"started_at"`
	UpdatedAt  string              `json:"updated_at"`
	BundlePath string              `json:"bundle_path,omitempty"`
	Proof      dashboardProofState `json:"proof"`
	L1         dashboardL1State    `json:"l1"`
	Logs       []dashboardLog      `json:"logs"`
}

type dashboardProofState struct {
	BatchSize           int    `json:"batch_size"`
	Total               int    `json:"total"`
	Generated           int    `json:"generated"`
	ReplayLoaded        int    `json:"replay_loaded"`
	CurrentLaneID       int    `json:"current_lane_id"`
	CurrentBatchID      uint64 `json:"current_batch_id"`
	SetupDone           bool   `json:"setup_done"`
	SetupElapsedMS      int64  `json:"setup_elapsed_ms"`
	LastProofElapsedMS  int64  `json:"last_proof_elapsed_ms"`
	GenerationElapsedMS int64  `json:"generation_elapsed_ms"`
	VerifiedTxTotal     uint64 `json:"verified_tx_total"`
}

type dashboardL1State struct {
	Enabled              bool                 `json:"enabled"`
	Mode                 string               `json:"mode"`
	TotalCalls           int                  `json:"total_calls"`
	SentCalls            int                  `json:"sent_calls"`
	ConfirmedCalls       int                  `json:"confirmed_calls"`
	FailedCalls          int                  `json:"failed_calls"`
	VerifiedTxPerCall    uint64               `json:"verified_tx_per_call"`
	VerifiedTxConfirmed  uint64               `json:"verified_tx_confirmed"`
	AggregateReceiptTPS  string               `json:"aggregate_receipt_tps"`
	AggregateBlockTPS    string               `json:"aggregate_block_tps"`
	AggregateEndToEndTPS string               `json:"aggregate_end_to_end_tps"`
	FirstSendAt          string               `json:"first_send_at,omitempty"`
	FirstInclusionAt     string               `json:"first_inclusion_at,omitempty"`
	LastInclusionAt      string               `json:"last_inclusion_at,omitempty"`
	LastReceiptAt        string               `json:"last_receipt_at,omitempty"`
	Transactions         []dashboardTxSummary `json:"transactions"`
}

type dashboardTxSummary struct {
	SenderAddress            string `json:"sender_address,omitempty"`
	ContractAddress          string `json:"contract_address,omitempty"`
	TransactionHash          string `json:"transaction_hash,omitempty"`
	BlockNumber              string `json:"block_number,omitempty"`
	BlockTimestamp           string `json:"block_timestamp,omitempty"`
	GasUsed                  uint64 `json:"gas_used,omitempty"`
	GasLimit                 uint64 `json:"gas_limit,omitempty"`
	BlockInclusionLatencyMS  int64  `json:"block_inclusion_latency_ms,omitempty"`
	ReceiptLatencyMS         int64  `json:"receipt_latency_ms,omitempty"`
	ReceiptVisibilityDelayMS int64  `json:"receipt_visibility_delay_ms,omitempty"`
	EndToEndLatencyMS        int64  `json:"end_to_end_latency_ms,omitempty"`
	Status                   string `json:"status"`
	Error                    string `json:"error,omitempty"`
}

type dashboardLog struct {
	At      string `json:"at"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type dashboardState struct {
	mu        sync.RWMutex
	startedAt time.Time
	updatedAt time.Time
	snapshot  dashboardSnapshot
}

type exporterProgressEvent struct {
	Event            string `json:"event"`
	BatchSize        int    `json:"batchSize"`
	LaneCount        int    `json:"laneCount"`
	Seed             uint64 `json:"seed"`
	Current          int    `json:"current"`
	Total            int    `json:"total"`
	LaneID           int    `json:"laneId"`
	BatchID          uint64 `json:"batchId"`
	ElapsedMS        int64  `json:"elapsedMs"`
	GeneratedTxTotal uint64 `json:"generatedTxTotal"`
	VerifiedTxTotal  uint64 `json:"verifiedTxTotal"`
}

type dashboardSubmitJob struct {
	sender   *sender
	to       common.Address
	tx       *types.Transaction
	gasLimit uint64
	sentAt   time.Time
}

type dashboardReceiptResult struct {
	job           dashboardSubmitJob
	receipt       *types.Receipt
	err           error
	includedAt    time.Time
	receiptSeenAt time.Time
	warning       string
}

func newDashboardState(title string) *dashboardState {
	now := time.Now()
	return &dashboardState{
		startedAt: now,
		updatedAt: now,
		snapshot: dashboardSnapshot{
			Title:     title,
			Phase:     "starting",
			StartedAt: now.Format(time.RFC3339),
			UpdatedAt: now.Format(time.RFC3339),
			Logs:      make([]dashboardLog, 0, 64),
			L1: dashboardL1State{
				Transactions: make([]dashboardTxSummary, 0, 32),
			},
		},
	}
}

func (s *dashboardState) clone() dashboardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.snapshot
	out.Logs = append([]dashboardLog(nil), s.snapshot.Logs...)
	out.L1.Transactions = append([]dashboardTxSummary(nil), s.snapshot.L1.Transactions...)
	return out
}

func (s *dashboardState) mutate(fn func(*dashboardSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.snapshot)
	s.updatedAt = time.Now()
	s.snapshot.UpdatedAt = s.updatedAt.Format(time.RFC3339)
}

func (s *dashboardState) log(level, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Logs = append(snapshot.Logs, dashboardLog{
			At:      time.Now().Format(time.RFC3339),
			Level:   level,
			Message: message,
		})
		if len(snapshot.Logs) > 200 {
			snapshot.Logs = append([]dashboardLog(nil), snapshot.Logs[len(snapshot.Logs)-200:]...)
		}
	})
}

func (s *dashboardState) setPhase(phase string) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Phase = phase
	})
}

func (s *dashboardState) setError(err error) {
	if err == nil {
		return
	}
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Error = err.Error()
		snapshot.Phase = "failed"
		snapshot.Done = true
	})
	s.log("error", "%v", err)
}

func (s *dashboardState) setDone() {
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Done = true
		if snapshot.Error == "" {
			snapshot.Phase = "completed"
		}
	})
}

func (s *dashboardState) applyExporterEvent(event exporterProgressEvent) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		if event.BatchSize != 0 {
			snapshot.Proof.BatchSize = event.BatchSize
		}
		if event.Total != 0 {
			snapshot.Proof.Total = event.Total
		}
		if event.LaneID != 0 || event.Event == "proof_started" || event.Event == "proof_completed" {
			snapshot.Proof.CurrentLaneID = event.LaneID
		}
		if event.BatchID != 0 {
			snapshot.Proof.CurrentBatchID = event.BatchID
		}
		if event.VerifiedTxTotal != 0 {
			snapshot.Proof.VerifiedTxTotal = event.VerifiedTxTotal
		}
		switch event.Event {
		case "setup_started":
			snapshot.Phase = "proof_setup"
		case "setup_completed":
			snapshot.Phase = "proof_generation"
			snapshot.Proof.SetupDone = true
			snapshot.Proof.SetupElapsedMS = event.ElapsedMS
		case "proof_started":
			snapshot.Phase = "proof_generation"
			if event.Total != 0 {
				snapshot.Proof.Total = event.Total
			}
		case "proof_completed":
			snapshot.Phase = "proof_generation"
			snapshot.Proof.Generated = event.Current
			snapshot.Proof.LastProofElapsedMS = event.ElapsedMS
			snapshot.Proof.VerifiedTxTotal = event.GeneratedTxTotal
		case "bundle_completed":
			snapshot.Phase = "bundle_ready"
			snapshot.Proof.GenerationElapsedMS = event.ElapsedMS
			snapshot.Proof.VerifiedTxTotal = event.VerifiedTxTotal
		}
	})
}

func (s *dashboardState) setBundlePath(path string) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.BundlePath = path
	})
}

func (s *dashboardState) configureSubmission(mode string, totalCalls int, verifiedTxPerCall uint64) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Phase = "l1_submitting"
		snapshot.L1.Enabled = true
		snapshot.L1.Mode = mode
		snapshot.L1.TotalCalls = totalCalls
		snapshot.L1.VerifiedTxPerCall = verifiedTxPerCall
	})
}

func (s *dashboardState) noteTxSent(job dashboardSubmitJob) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.L1.SentCalls++
		if snapshot.L1.FirstSendAt == "" {
			snapshot.L1.FirstSendAt = job.sentAt.Format(time.RFC3339)
		}
		snapshot.L1.Transactions = append(snapshot.L1.Transactions, dashboardTxSummary{
			SenderAddress:   job.sender.from.Hex(),
			ContractAddress: job.to.Hex(),
			TransactionHash: job.tx.Hash().Hex(),
			GasLimit:        job.gasLimit,
			Status:          "sent",
		})
	})
}

func (s *dashboardState) noteReceipt(result dashboardReceiptResult, confirmedVerified uint64, firstSendAt, workflowStartedAt time.Time) {
	s.mutate(func(snapshot *dashboardSnapshot) {
		entry := dashboardTxSummary{
			SenderAddress:   result.job.sender.from.Hex(),
			ContractAddress: result.job.to.Hex(),
			TransactionHash: result.job.tx.Hash().Hex(),
			GasLimit:        result.job.gasLimit,
		}
		if result.err != nil {
			snapshot.L1.FailedCalls++
			entry.Status = "failed"
			entry.Error = result.err.Error()
		} else {
			snapshot.L1.ConfirmedCalls++
			snapshot.L1.VerifiedTxConfirmed = confirmedVerified
			receiptSeenAt := result.receiptSeenAt
			if receiptSeenAt.IsZero() {
				receiptSeenAt = time.Now()
			}
			snapshot.L1.LastReceiptAt = receiptSeenAt.Format(time.RFC3339)
			entry.Status = "confirmed"
			entry.BlockNumber = result.receipt.BlockNumber.String()
			entry.GasUsed = result.receipt.GasUsed
			entry.ReceiptLatencyMS = receiptSeenAt.Sub(result.job.sentAt).Milliseconds()
			entry.EndToEndLatencyMS = receiptSeenAt.Sub(workflowStartedAt).Milliseconds()
			if !result.includedAt.IsZero() {
				entry.BlockTimestamp = result.includedAt.Format(time.RFC3339)
				entry.BlockInclusionLatencyMS = clampNonNegativeMS(result.includedAt.Sub(result.job.sentAt))
				entry.ReceiptVisibilityDelayMS = clampNonNegativeMS(receiptSeenAt.Sub(result.includedAt))
				if snapshot.L1.FirstInclusionAt == "" {
					snapshot.L1.FirstInclusionAt = result.includedAt.Format(time.RFC3339)
				}
				snapshot.L1.LastInclusionAt = result.includedAt.Format(time.RFC3339)
			}
			receiptElapsedMs := receiptSeenAt.Sub(firstSendAt).Milliseconds()
			if receiptElapsedMs > 0 {
				snapshot.L1.AggregateReceiptTPS = new(big.Rat).SetFrac(
					new(big.Int).Mul(new(big.Int).SetUint64(confirmedVerified), big.NewInt(1000)),
					big.NewInt(receiptElapsedMs),
				).FloatString(6)
			}
			if !result.includedAt.IsZero() {
				blockElapsedMs := result.includedAt.Sub(firstSendAt).Milliseconds()
				if blockElapsedMs > 0 {
					snapshot.L1.AggregateBlockTPS = new(big.Rat).SetFrac(
						new(big.Int).Mul(new(big.Int).SetUint64(confirmedVerified), big.NewInt(1000)),
						big.NewInt(blockElapsedMs),
					).FloatString(6)
				}
			}
			endToEndElapsedMs := receiptSeenAt.Sub(workflowStartedAt).Milliseconds()
			if endToEndElapsedMs > 0 {
				snapshot.L1.AggregateEndToEndTPS = new(big.Rat).SetFrac(
					new(big.Int).Mul(new(big.Int).SetUint64(confirmedVerified), big.NewInt(1000)),
					big.NewInt(endToEndElapsedMs),
				).FloatString(6)
			}
		}
		snapshot.L1.Transactions = append(snapshot.L1.Transactions, entry)
		if len(snapshot.L1.Transactions) > 100 {
			snapshot.L1.Transactions = append([]dashboardTxSummary(nil), snapshot.L1.Transactions[len(snapshot.L1.Transactions)-100:]...)
		}
	})
}

func clampNonNegativeMS(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	return ms
}

func runDashboard(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	listen := fs.String("listen", ":8088", "dashboard listen address")
	title := fs.String("title", "HyperL2 Progress Dashboard", "dashboard title")
	manifestPath := fs.String("manifest-path", "rust/vectis-prover/Cargo.toml", "Rust manifest path for proof generation")
	bundlePath := fs.String("bundle-path", filepath.Join("build", "dashboard", "certification-bundle.json"), "bundle JSON output path")
	skipGenerate := fs.Bool("skip-generate", false, "skip proof generation and load an existing bundle")
	batchSize := fs.Int("batch-size", 20000, "proof batch size")
	laneCount := fs.Int("lane-count", 10, "proof count / lane count")
	liveLaneCount := fs.Int("live-lane-count", 0, "number of proofs to generate live; 0 means generate all")
	replayBundlePath := fs.String("replay-bundle-path", defaultBundlePath, "existing bundle path to source replay proofs from when live-lane-count < lane-count")
	batchIDBase := fs.Uint64("batch-id-base", 1, "starting batch id")
	seed := fs.Uint64("seed", 7, "deterministic proof seed")
	rpcURL := fs.String("rpc-url", envOr("KAIA_RPC_URL", defaultRPCURL), "RPC URL")
	privateKeysText := fs.String("private-keys", envOr("KAIA_PRIVATE_KEYS", ""), "comma-separated sender private keys")
	gasPriceHex := fs.String("gas-price", envOr("KAIA_GAS_PRICE", ""), "gas price in wei")
	certifierAddressesText := fs.String("certifier-addresses", envOr("KAIA_CERTIFIER_ADDRESSES", ""), "comma-separated SumPreservingBatchVerifier addresses")
	artifactsDir := fs.String("artifacts-dir", envOr("KAIA_OUTPUT_DIR", "build/kaia-preflight")+"/contracts", "contract artifact directory")
	confirmations := fs.Uint64("confirmations", uint64(envOrInt("KAIA_CONFIRMATIONS", defaultConfirmations)), "confirmation count")
	if err := fs.Parse(args); err != nil {
		return err
	}

	state := newDashboardState(*title)
	state.log("info", "dashboard listening on http://127.0.0.1%s", *listen)
	state.log("info", "bundle path: %s", *bundlePath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardHTML))
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state.clone())
	})
	server := &http.Server{Addr: *listen, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			state.setError(err)
		}
	}()

	go func() {
		if err := runDashboardWorkflow(ctx, state, dashboardOptions{
			ManifestPath:       *manifestPath,
			BundlePath:         *bundlePath,
			SkipGenerate:       *skipGenerate,
			BatchSize:          *batchSize,
			LaneCount:          *laneCount,
			LiveLaneCount:      *liveLaneCount,
			ReplayBundlePath:   *replayBundlePath,
			BatchIDBase:        *batchIDBase,
			Seed:               *seed,
			RPCURL:             *rpcURL,
			PrivateKeysText:    *privateKeysText,
			GasPriceHex:        *gasPriceHex,
			CertifierAddresses: *certifierAddressesText,
			ArtifactsDir:       *artifactsDir,
			Confirmations:      *confirmations,
		}); err != nil {
			state.setError(err)
			return
		}
		state.setDone()
		state.log("info", "workflow completed")
	}()

	fmt.Printf("dashboard: http://127.0.0.1%s\n", *listen)
	<-ctx.Done()
	return server.Shutdown(context.Background())
}

type dashboardOptions struct {
	ManifestPath       string
	BundlePath         string
	SkipGenerate       bool
	BatchSize          int
	LaneCount          int
	LiveLaneCount      int
	ReplayBundlePath   string
	BatchIDBase        uint64
	Seed               uint64
	RPCURL             string
	PrivateKeysText    string
	GasPriceHex        string
	CertifierAddresses string
	ArtifactsDir       string
	Confirmations      uint64
}

func runDashboardWorkflow(ctx context.Context, state *dashboardState, opts dashboardOptions) error {
	bundlePath := opts.BundlePath
	if !opts.SkipGenerate {
		if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
			return err
		}
		state.log("info", "starting proof generation: batch_size=%d lane_count=%d", opts.BatchSize, opts.LaneCount)
		if err := generateBundleWithProgress(ctx, state, opts, bundlePath); err != nil {
			return err
		}
	} else {
		state.setPhase("bundle_ready")
		state.log("info", "loading existing bundle: %s", bundlePath)
		bundle, err := loadBundle(bundlePath)
		if err != nil {
			return err
		}
		artifacts, verifiedTxTotal, err := bundle.toArtifacts()
		if err != nil {
			return err
		}
		state.mutate(func(snapshot *dashboardSnapshot) {
			snapshot.BundlePath = bundlePath
			snapshot.Proof.Total = len(artifacts)
			snapshot.Proof.Generated = len(artifacts)
			snapshot.Proof.VerifiedTxTotal = verifiedTxTotal
		})
	}
	state.setBundlePath(bundlePath)

	privateKeys := parseCSV(opts.PrivateKeysText)
	addresses, err := parseAddressList(opts.CertifierAddresses)
	if err != nil {
		return err
	}
	if len(privateKeys) == 0 || len(addresses) == 0 {
		state.log("info", "submission skipped: provide --private-keys and --certifier-addresses to enable L1 submit")
		return nil
	}
	return submitBundleWithProgress(ctx, state, opts, bundlePath, privateKeys, addresses)
}

func generateBundleWithProgress(ctx context.Context, state *dashboardState, opts dashboardOptions, bundlePath string) error {
	liveLaneCount := opts.LiveLaneCount
	if liveLaneCount <= 0 || liveLaneCount > opts.LaneCount {
		liveLaneCount = opts.LaneCount
	}

	if liveLaneCount == opts.LaneCount {
		data, err := runExporterWithProgress(ctx, state, opts, opts.LaneCount)
		if err != nil {
			return err
		}
		if err := os.WriteFile(bundlePath, data, 0o644); err != nil {
			return err
		}
		state.setBundlePath(bundlePath)
		state.log("info", "bundle written: %s", bundlePath)
		return nil
	}

	replayCount := opts.LaneCount - liveLaneCount
	state.log("info", "live/replay mode enabled: live=%d replay=%d", liveLaneCount, replayCount)
	liveData, err := runExporterWithProgress(ctx, state, opts, liveLaneCount)
	if err != nil {
		return err
	}
	var liveBundle jsonBundle
	if err := json.Unmarshal(liveData, &liveBundle); err != nil {
		return err
	}
	replayBundle, err := loadBundle(opts.ReplayBundlePath)
	if err != nil {
		return fmt.Errorf("load replay bundle: %w", err)
	}
	merged, err := mergeBundles(&liveBundle, replayBundle, liveLaneCount, opts.LaneCount)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(bundlePath, out, 0o644); err != nil {
		return err
	}
	state.mutate(func(snapshot *dashboardSnapshot) {
		snapshot.Phase = "bundle_ready"
		snapshot.Proof.Total = opts.LaneCount
		snapshot.Proof.Generated = opts.LaneCount
		snapshot.Proof.ReplayLoaded = replayCount
		snapshot.Proof.VerifiedTxTotal = uint64(opts.BatchSize * opts.LaneCount)
	})
	state.setBundlePath(bundlePath)
	state.log("info", "loaded %d replay proofs from %s", replayCount, opts.ReplayBundlePath)
	state.log("info", "bundle written: %s", bundlePath)
	return nil
}

func runExporterWithProgress(ctx context.Context, state *dashboardState, opts dashboardOptions, laneCount int) ([]byte, error) {
	cmd := exec.CommandContext(ctx,
		"cargo", "+stable", "run",
		"--manifest-path", opts.ManifestPath,
		"--bin", "export_sumproof_bundle",
		"--release",
		"--",
		"--batch-size", fmt.Sprint(opts.BatchSize),
		"--lane-count", fmt.Sprint(laneCount),
		"--batch-id-base", fmt.Sprint(opts.BatchIDBase),
		"--seed", fmt.Sprint(opts.Seed),
		"--progress-json",
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event exporterProgressEvent
		if err := json.Unmarshal(line, &event); err == nil && event.Event != "" {
			state.applyExporterEvent(event)
			switch event.Event {
			case "setup_started":
				state.log("info", "proof setup started")
			case "setup_completed":
				state.log("info", "proof setup completed in %d ms", event.ElapsedMS)
			case "proof_started":
				state.log("info", "proof %d/%d started (lane=%d)", event.Current, event.Total, event.LaneID)
			case "proof_completed":
				state.log("info", "proof %d/%d completed in %d ms", event.Current, event.Total, event.ElapsedMS)
			case "bundle_completed":
				state.log("info", "bundle completed in %d ms", event.ElapsedMS)
			}
			continue
		}
		text := string(bytes.TrimSpace(line))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "Compiling ") ||
			strings.HasPrefix(text, "Finished ") ||
			strings.HasPrefix(text, "Running `") ||
			strings.HasPrefix(text, "warning:") ||
			strings.HasPrefix(text, "error:") {
			state.log("info", "%s", text)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func mergeBundles(liveBundle, replayBundle *jsonBundle, liveLaneCount, totalLaneCount int) (*jsonBundle, error) {
	if liveBundle == nil || replayBundle == nil {
		return nil, errors.New("merge bundles: nil input")
	}
	liveArtifacts := liveBundle.VerifyBatchesInput.Artifacts
	replayArtifacts := replayBundle.VerifyBatchesInput.Artifacts
	if len(liveArtifacts) == 0 {
		liveArtifacts = liveBundle.VerifyTenBatchesInput.Artifacts
	}
	if len(replayArtifacts) == 0 {
		replayArtifacts = replayBundle.VerifyTenBatchesInput.Artifacts
	}
	if len(liveArtifacts) < liveLaneCount {
		return nil, fmt.Errorf("live bundle has %d artifacts, need %d", len(liveArtifacts), liveLaneCount)
	}
	if len(replayArtifacts) < totalLaneCount {
		return nil, fmt.Errorf("replay bundle has %d artifacts, need at least %d", len(replayArtifacts), totalLaneCount)
	}
	if len(liveBundle.LaneHeads) < liveLaneCount {
		return nil, fmt.Errorf("live bundle has %d lane heads, need %d", len(liveBundle.LaneHeads), liveLaneCount)
	}
	if len(replayBundle.LaneHeads) < totalLaneCount {
		return nil, fmt.Errorf("replay bundle has %d lane heads, need at least %d", len(replayBundle.LaneHeads), totalLaneCount)
	}

	mergedArtifacts := make([]jsonBatchTransitionArtifact, 0, totalLaneCount)
	mergedArtifacts = append(mergedArtifacts, liveArtifacts[:liveLaneCount]...)
	mergedArtifacts = append(mergedArtifacts, replayArtifacts[liveLaneCount:totalLaneCount]...)

	mergedLaneHeads := make([]jsonLaneHead, 0, totalLaneCount)
	mergedLaneHeads = append(mergedLaneHeads, liveBundle.LaneHeads[:liveLaneCount]...)
	mergedLaneHeads = append(mergedLaneHeads, replayBundle.LaneHeads[liveLaneCount:totalLaneCount]...)

	verifiedTxTotal := uint64(0)
	for _, artifact := range mergedArtifacts {
		verifiedTxTotal += uint64(artifact.TxCount)
	}

	merged := *liveBundle
	merged.LaneHeads = mergedLaneHeads
	merged.Notes.VerifiedTxTotal = verifiedTxTotal
	merged.VerifyBatchesInput.Artifacts = mergedArtifacts
	merged.VerifyTenBatchesInput.Artifacts = mergedArtifacts
	return &merged, nil
}

func submitBundleWithProgress(ctx context.Context, state *dashboardState, opts dashboardOptions, bundlePath string, privateKeys []string, addresses []common.Address) error {
	bundle, err := loadBundle(bundlePath)
	if err != nil {
		return err
	}
	artifacts, verifiedTxTotal, err := bundle.toArtifacts()
	if err != nil {
		return err
	}
	abiObj, _, err := loadContractArtifact(opts.ArtifactsDir, "SumPreservingBatchVerifier")
	if err != nil {
		return err
	}
	data, err := abiObj.Pack("verifyBatches", artifacts)
	if err != nil {
		return err
	}

	mode := "single"
	switch {
	case len(privateKeys) == 1 && len(addresses) > 1:
		mode = "same_sender_burst"
	case len(privateKeys) == len(addresses) && len(privateKeys) > 1:
		mode = "multisender_burst"
	case len(privateKeys) == 1 && len(addresses) == 1:
		mode = "single"
	default:
		return fmt.Errorf("unsupported key/address layout: private_keys=%d certifier_addresses=%d", len(privateKeys), len(addresses))
	}
	state.configureSubmission(mode, len(addresses), verifiedTxTotal)
	state.log("info", "starting L1 submission: mode=%s calls=%d verified_tx_per_call=%d", mode, len(addresses), verifiedTxTotal)

	jobs, cleanup, err := buildDashboardJobs(ctx, opts, privateKeys, addresses, data)
	if err != nil {
		return err
	}
	defer cleanup()

	firstSendAt := time.Now()
	for i := range jobs {
		jobs[i].sentAt = time.Now()
		if i == 0 {
			firstSendAt = jobs[i].sentAt
		}
		if err := jobs[i].sender.client.SendTransaction(ctx, jobs[i].tx); err != nil {
			return err
		}
		state.noteTxSent(jobs[i])
		state.log("info", "submitted tx %s to %s", jobs[i].tx.Hash().Hex(), jobs[i].to.Hex())
	}

	resultsCh := make(chan dashboardReceiptResult, len(jobs))
	for _, job := range jobs {
		job := job
		go func() {
			receipt, err := waitForReceipt(ctx, job.sender.client, job.tx.Hash(), job.sender.confirmations)
			if err == nil && receipt.Status != types.ReceiptStatusSuccessful {
				err = fmt.Errorf("transaction reverted: hash=%s block=%s gas_used=%d", job.tx.Hash().Hex(), receipt.BlockNumber.String(), receipt.GasUsed)
			}
			result := dashboardReceiptResult{job: job, receipt: receipt, err: err}
			if err == nil {
				result.receiptSeenAt = time.Now()
				header, headerErr := job.sender.client.HeaderByNumber(ctx, receipt.BlockNumber)
				if headerErr == nil {
					result.includedAt = time.Unix(int64(header.Time), 0)
				} else {
					result.warning = fmt.Sprintf("header lookup unavailable for %s: %v", job.tx.Hash().Hex(), headerErr)
				}
			}
			resultsCh <- result
		}()
	}

	var confirmedVerified uint64
	var firstErr error
	for range jobs {
		result := <-resultsCh
		if result.err == nil {
			confirmedVerified += verifiedTxTotal
			state.log("info", "confirmed tx %s in block %s", result.job.tx.Hash().Hex(), result.receipt.BlockNumber.String())
			if result.warning != "" {
				state.log("warn", "%s", result.warning)
			}
		} else {
			state.log("error", "%v", result.err)
			if firstErr == nil {
				firstErr = result.err
			}
		}
		state.noteReceipt(result, confirmedVerified, firstSendAt, state.startedAt)
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func buildDashboardJobs(ctx context.Context, opts dashboardOptions, privateKeys []string, addresses []common.Address, data []byte) ([]dashboardSubmitJob, func(), error) {
	jobs := make([]dashboardSubmitJob, 0, len(addresses))
	cleanupSenders := make([]*sender, 0, len(addresses))
	cleanup := func() {
		for _, txSender := range cleanupSenders {
			txSender.client.Close()
		}
	}

	if len(privateKeys) == 1 {
		txSender, err := newSender(opts.RPCURL, privateKeys[0], opts.GasPriceHex, opts.Confirmations)
		if err != nil {
			return nil, cleanup, err
		}
		cleanupSenders = append(cleanupSenders, txSender)
		nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
		if err != nil {
			cleanup()
			return nil, cleanup, err
		}
		for i, address := range addresses {
			signedTx, gasLimit, err := txSender.buildSignedTransaction(ctx, ptrAddress(address), data, nonce+uint64(i))
			if err != nil {
				cleanup()
				return nil, cleanup, err
			}
			jobs = append(jobs, dashboardSubmitJob{sender: txSender, to: address, tx: signedTx, gasLimit: gasLimit})
		}
		return jobs, cleanup, nil
	}

	if len(privateKeys) != len(addresses) {
		cleanup()
		return nil, cleanup, fmt.Errorf("private key count (%d) does not match certifier count (%d)", len(privateKeys), len(addresses))
	}
	for i, privateKey := range privateKeys {
		txSender, err := newSender(opts.RPCURL, privateKey, opts.GasPriceHex, opts.Confirmations)
		if err != nil {
			cleanup()
			return nil, cleanup, err
		}
		cleanupSenders = append(cleanupSenders, txSender)
		nonce, err := txSender.client.PendingNonceAt(ctx, txSender.from)
		if err != nil {
			cleanup()
			return nil, cleanup, err
		}
		signedTx, gasLimit, err := txSender.buildSignedTransaction(ctx, ptrAddress(addresses[i]), data, nonce)
		if err != nil {
			cleanup()
			return nil, cleanup, err
		}
		jobs = append(jobs, dashboardSubmitJob{sender: txSender, to: addresses[i], tx: signedTx, gasLimit: gasLimit})
	}
	return jobs, cleanup, nil
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>HyperL2 Dashboard</title>
  <style>
    :root {
      --bg: #0f1419;
      --panel: #162029;
      --panel-2: #1d2b36;
      --ink: #e9f0f4;
      --muted: #9bb0bd;
      --accent: #37c5a8;
      --warn: #f0b24b;
      --danger: #ef6b73;
      --border: #2e4452;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "IBM Plex Sans", "Pretendard", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at top right, rgba(55,197,168,0.18), transparent 30%),
        linear-gradient(180deg, #0b1014, var(--bg));
    }
    .wrap { max-width: 1280px; margin: 0 auto; padding: 24px; }
    h1 { margin: 0 0 8px; font-size: 32px; }
    .muted { color: var(--muted); }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
      gap: 16px;
      margin-top: 18px;
    }
    .card {
      background: linear-gradient(180deg, rgba(255,255,255,0.02), rgba(255,255,255,0.01));
      border: 1px solid var(--border);
      border-radius: 18px;
      padding: 18px;
      box-shadow: 0 12px 40px rgba(0,0,0,0.25);
    }
    .metric { font-size: 34px; font-weight: 700; margin: 6px 0; }
    .label { text-transform: uppercase; letter-spacing: 0.08em; font-size: 12px; color: var(--muted); }
    .bar {
      width: 100%; height: 12px; background: #0c1318; border-radius: 999px; overflow: hidden; border: 1px solid var(--border);
    }
    .bar > div { height: 100%; background: linear-gradient(90deg, var(--accent), #7fd8c7); width: 0%; transition: width 0.3s ease; }
    .row { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
    table { width: 100%; border-collapse: collapse; margin-top: 10px; }
    th, td { text-align: left; padding: 10px 8px; border-bottom: 1px solid var(--border); font-size: 14px; }
    th { color: var(--muted); font-weight: 600; }
    .logs { max-height: 280px; overflow: auto; font-family: "IBM Plex Mono", monospace; font-size: 13px; }
    .log { padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,0.04); }
    .ok { color: var(--accent); }
    .warn { color: var(--warn); }
    .err { color: var(--danger); }
    code { color: #c9f0e5; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="row">
      <div>
        <h1 id="title">HyperL2 Dashboard</h1>
        <div class="muted" id="subtitle">Waiting for workflow...</div>
      </div>
      <div class="card" style="min-width:260px;">
        <div class="label">Phase</div>
        <div class="metric" id="phase">starting</div>
        <div class="muted" id="error"></div>
      </div>
    </div>

    <div class="grid">
      <div class="card">
        <div class="label">Proof Progress</div>
        <div class="metric"><span id="proofGenerated">0</span> / <span id="proofTotal">0</span></div>
        <div class="bar"><div id="proofBar"></div></div>
        <div class="muted" id="proofMeta">setup pending</div>
      </div>
      <div class="card">
        <div class="label">Certified Tx Generated</div>
        <div class="metric" id="generatedTx">0</div>
        <div class="muted" id="bundlePath">bundle not written</div>
      </div>
      <div class="card">
        <div class="label">L1 Confirmed Calls</div>
        <div class="metric"><span id="confirmedCalls">0</span> / <span id="totalCalls">0</span></div>
        <div class="bar"><div id="l1Bar"></div></div>
        <div class="muted" id="l1Mode">submission disabled</div>
      </div>
      <div class="card">
        <div class="label">TPS</div>
        <div class="metric" id="aggregateTps">0</div>
        <div class="muted">L1 block inclusion throughput / confirmed certified tx: <span id="confirmedTx">0</span></div>
      </div>
      <div class="card">
        <div class="label">Receipt-Visible TPS</div>
        <div class="metric" id="aggregateReceiptTps">0</div>
        <div class="muted">send -> receipt visible on RPC</div>
      </div>
      <div class="card">
        <div class="label">Aggregate End-to-End TPS</div>
        <div class="metric" id="aggregateEndToEndTps">0</div>
        <div class="muted">workflow start -> receipt visible</div>
      </div>
    </div>

    <div class="grid">
      <div class="card">
        <div class="label">L1 Transactions</div>
        <table>
          <thead>
            <tr><th>Status</th><th>Sender</th><th>Block</th><th>Incl. ms</th><th>Receipt ms</th><th>RPC lag ms</th><th>Gas</th><th>Tx</th></tr>
          </thead>
          <tbody id="txRows"></tbody>
        </table>
      </div>
      <div class="card">
        <div class="label">Logs</div>
        <div class="logs" id="logs"></div>
      </div>
    </div>
  </div>

  <script>
    async function refresh() {
      const res = await fetch('/api/state', { cache: 'no-store' });
      const state = await res.json();
      document.getElementById('title').textContent = state.title;
      document.getElementById('subtitle').textContent = 'Started ' + state.started_at + ' / Updated ' + state.updated_at;
      document.getElementById('phase').textContent = state.phase;
      document.getElementById('error').textContent = state.error || '';
      document.getElementById('error').className = state.error ? 'err' : 'muted';

      const proofTotal = state.proof.total || 0;
      const proofGenerated = state.proof.generated || 0;
      document.getElementById('proofGenerated').textContent = proofGenerated;
      document.getElementById('proofTotal').textContent = proofTotal;
      document.getElementById('proofBar').style.width = proofTotal ? (((proofGenerated / proofTotal) * 100).toFixed(1) + '%') : '0%';
      document.getElementById('proofMeta').textContent = state.proof.setup_done
        ? ('setup ' + state.proof.setup_elapsed_ms + ' ms / last proof ' + state.proof.last_proof_elapsed_ms + ' ms / replay loaded ' + (state.proof.replay_loaded || 0) + ' / lane ' + state.proof.current_lane_id)
        : 'proof setup pending';
      document.getElementById('generatedTx').textContent = (state.proof.verified_tx_total || 0).toLocaleString();
      document.getElementById('bundlePath').textContent = state.bundle_path || 'bundle not written';

      document.getElementById('confirmedCalls').textContent = state.l1.confirmed_calls || 0;
      document.getElementById('totalCalls').textContent = state.l1.total_calls || 0;
      document.getElementById('l1Bar').style.width = state.l1.total_calls ? (((state.l1.confirmed_calls / state.l1.total_calls) * 100).toFixed(1) + '%') : '0%';
      document.getElementById('l1Mode').textContent = state.l1.enabled
        ? (state.l1.mode + ' / ' + state.l1.sent_calls + ' sent / ' + state.l1.failed_calls + ' failed')
        : 'submission disabled';
      document.getElementById('aggregateTps').textContent = state.l1.aggregate_block_tps || '0';
      document.getElementById('aggregateReceiptTps').textContent = state.l1.aggregate_receipt_tps || '0';
      document.getElementById('aggregateEndToEndTps').textContent = state.l1.aggregate_end_to_end_tps || '0';
      document.getElementById('confirmedTx').textContent = (state.l1.verified_tx_confirmed || 0).toLocaleString();

      const txRows = document.getElementById('txRows');
      txRows.innerHTML = '';
      for (const tx of state.l1.transactions.slice(-12).reverse()) {
        const tr = document.createElement('tr');
        tr.innerHTML =
          '<td>' + tx.status + '</td>' +
          '<td><code>' + ((tx.sender_address || '').slice(0,10)) + '</code></td>' +
          '<td>' + (tx.block_number || '-') + '</td>' +
          '<td>' + (tx.block_inclusion_latency_ms ?? '-') + '</td>' +
          '<td>' + (tx.receipt_latency_ms ?? '-') + '</td>' +
          '<td>' + (tx.receipt_visibility_delay_ms ?? '-') + '</td>' +
          '<td>' + (tx.gas_used || '-') + '</td>' +
          '<td><code>' + ((tx.transaction_hash || '').slice(0,12)) + '</code></td>';
        txRows.appendChild(tr);
      }

      const logs = document.getElementById('logs');
      logs.innerHTML = '';
      for (const entry of state.logs.slice(-60).reverse()) {
        const div = document.createElement('div');
        div.className = 'log';
        div.innerHTML = '<span class=\"muted\">' + entry.at + '</span> <strong>' + entry.level + '</strong> ' + entry.message;
        logs.appendChild(div);
      }
    }
    refresh();
    setInterval(refresh, 500);
  </script>
</body>
</html>`
