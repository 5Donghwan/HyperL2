package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"hyperl2/internal/config"
	"hyperl2/internal/frame"
	"hyperl2/internal/metrics"
	"hyperl2/internal/queue"
)

type Runner struct {
	cfg        config.Config
	configPath string
	metrics    *metrics.Registry

	ingressQ *queue.Ring[BlockJob]
	laneQ    *queue.Ring[LaneBatch]
	commitQ  *queue.Ring[BlockCommit]

	proofBook *ProofBook
	l1        *L1CertNode
}

type RunResult struct {
	Report Report
	Path   string
}

func NewRunner(cfg config.Config, configPath string) (*Runner, error) {
	ingressQ, err := queue.NewRing[BlockJob](cfg.QueueCapacity)
	if err != nil {
		return nil, err
	}
	laneQ, err := queue.NewRing[LaneBatch](cfg.QueueCapacity)
	if err != nil {
		return nil, err
	}
	commitQ, err := queue.NewRing[BlockCommit](cfg.QueueCapacity)
	if err != nil {
		return nil, err
	}
	m := metrics.NewRegistry()
	p := NewProofBook(cfg.ProofSeed)
	if cfg.ProofDatasetPath != "" {
		if _, err := os.Stat(cfg.ProofDatasetPath); err == nil {
			if err := p.LoadJSONL(cfg.ProofDatasetPath); err != nil {
				return nil, fmt.Errorf("load proof dataset: %w", err)
			}
		}
	}
	return &Runner{
		cfg:        cfg,
		configPath: configPath,
		metrics:    m,
		ingressQ:   ingressQ,
		laneQ:      laneQ,
		commitQ:    commitQ,
		proofBook:  p,
		l1:         NewL1CertNode(cfg.ProofSeed, m),
	}, nil
}

func (r *Runner) Run(ctx context.Context) (RunResult, error) {
	started := time.Now()

	mux := http.NewServeMux()
	mux.Handle("/metrics", r.metrics.PrometheusHandler())
	metricsSrv := &http.Server{Addr: r.cfg.MetricsListenAddr, Handler: mux}
	go func() {
		_ = metricsSrv.ListenAndServe()
	}()
	defer func() {
		_ = metricsSrv.Shutdown(context.Background())
	}()

	var ingressDone atomic.Bool
	var sequencerDone atomic.Bool
	var aggregatorDone atomic.Bool

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		r.runSequencer(ctx, &ingressDone, &sequencerDone)
	}()
	go func() {
		defer wg.Done()
		r.runAggregator(ctx, &sequencerDone, &aggregatorDone)
	}()
	go func() {
		defer wg.Done()
		r.runProofReplayer(ctx, &aggregatorDone)
	}()

	nextBlock := uint32(0)
	nextBlock = r.runIngressPhase(ctx, "warmup", nextBlock, r.cfg.Warmup)
	nextBlock = r.runIngressPhase(ctx, "certification", nextBlock, r.cfg.Certification)
	_ = nextBlock
	ingressDone.Store(true)

	wg.Wait()

	report, err := r.buildReport(started, time.Now())
	if err != nil {
		return RunResult{}, err
	}
	reportPath, err := WriteReport(r.cfg.ReportDir, report)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Report: report, Path: reportPath}, nil
}

func (r *Runner) runIngressPhase(ctx context.Context, phaseName string, startBlock uint32, phase config.Phase) uint32 {
	blocks := r.cfg.BlocksForPhase(phase)
	if blocks == 0 || phase.TPS == 0 {
		return startBlock
	}
	interval := r.cfg.BlockInterval()
	nextTick := time.Now()
	for i := 0; i < blocks; i++ {
		select {
		case <-ctx.Done():
			return startBlock + uint32(i)
		default:
		}

		blockIndex := startBlock + uint32(i)
		if r.cfg.Fault.PauseMillis > 0 && r.cfg.Fault.PauseAtBlock == blockIndex {
			time.Sleep(time.Duration(r.cfg.Fault.PauseMillis) * time.Millisecond)
		}

		txCount := r.cfg.TxPerBlock(phase.TPS)
		txs := GenerateSyntheticTxs(blockIndex, txCount)
		job := BlockJob{
			Phase:      phaseName,
			BlockIndex: blockIndex,
			Txs:        txs,
			CreatedAt:  time.Now(),
		}
		for !r.ingressQ.Enqueue(job) {
			select {
			case <-ctx.Done():
				return startBlock + uint32(i)
			default:
			}
			r.metrics.IncDrop(1)
			time.Sleep(100 * time.Microsecond)
		}
		r.metrics.AddIngressTx(uint64(txCount))
		r.updateQueueDepth()

		nextTick = nextTick.Add(interval)
		now := time.Now()
		if nextTick.After(now) {
			time.Sleep(nextTick.Sub(now))
		}
	}
	return startBlock + uint32(blocks)
}

func (r *Runner) runSequencer(ctx context.Context, ingressDone, sequencerDone *atomic.Bool) {
	defer sequencerDone.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, ok := r.ingressQ.Dequeue()
		if !ok {
			if ingressDone.Load() {
				return
			}
			time.Sleep(100 * time.Microsecond)
			continue
		}

		lanes := make([][]frame.Tx12, r.cfg.Lanes)
		for i, tx := range job.Txs {
			lane := i % r.cfg.Lanes
			lanes[lane] = append(lanes[lane], tx)
		}
		job.Txs = nil

		for laneID := 0; laneID < r.cfg.Lanes; laneID++ {
			batch := LaneBatch{
				Phase:      job.Phase,
				BlockIndex: job.BlockIndex,
				LaneID:     uint16(laneID),
				Payload:    lanes[laneID],
				CreatedAt:  job.CreatedAt,
			}
			for !r.laneQ.Enqueue(batch) {
				select {
				case <-ctx.Done():
					return
				default:
				}
				r.metrics.IncDrop(1)
				time.Sleep(100 * time.Microsecond)
			}
		}
		r.updateQueueDepth()
	}
}

type aggregateState struct {
	phase      string
	createdAt  time.Time
	laneHashes map[uint16][32]byte
	txCount    uint32
	txs        []frame.Tx12
}

func (r *Runner) runAggregator(ctx context.Context, sequencerDone, aggregatorDone *atomic.Bool) {
	defer aggregatorDone.Store(true)
	states := make(map[uint32]*aggregateState)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		batch, ok := r.laneQ.Dequeue()
		if !ok {
			if sequencerDone.Load() && len(states) == 0 {
				return
			}
			time.Sleep(100 * time.Microsecond)
			continue
		}

		st, ok := states[batch.BlockIndex]
		if !ok {
			st = &aggregateState{
				phase:      batch.Phase,
				createdAt:  batch.CreatedAt,
				laneHashes: make(map[uint16][32]byte, r.cfg.Lanes),
				txCount:    0,
				txs:        make([]frame.Tx12, 0, len(batch.Payload)*r.cfg.Lanes),
			}
			states[batch.BlockIndex] = st
		}

		bf := BuildLaneBatchFrame(batch.BlockIndex, batch.LaneID, batch.Payload)
		st.laneHashes[batch.LaneID] = bf.CommitmentHash()
		for _, tx := range batch.Payload {
			if !isPayloadValid(tx) {
				r.metrics.IncDrop(1)
				continue
			}
			st.txCount++
			st.txs = append(st.txs, tx)
		}

		if len(st.laneHashes) == r.cfg.Lanes {
			commit := BlockCommit{
				Phase:          st.phase,
				BlockIndex:     batch.BlockIndex,
				CommitmentHash: CommitmentFromTxs(batch.BlockIndex, st.txs),
				TxCount:        st.txCount,
				Txs:            st.txs,
				CreatedAt:      st.createdAt,
			}
			for !r.commitQ.Enqueue(commit) {
				select {
				case <-ctx.Done():
					return
				default:
				}
				r.metrics.IncDrop(1)
				time.Sleep(100 * time.Microsecond)
			}
			delete(states, batch.BlockIndex)
		}
		r.updateQueueDepth()
	}
}

func (r *Runner) runProofReplayer(ctx context.Context, aggregatorDone *atomic.Bool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		commit, ok := r.commitQ.Dequeue()
		if !ok {
			if aggregatorDone.Load() {
				return
			}
			time.Sleep(100 * time.Microsecond)
			continue
		}

		proofBytes := r.proofBook.ProofForBlock(commit.BlockIndex)
		proof := BuildProofRecord(commit.BlockIndex, commit.CommitmentHash, proofBytes)
		if r.cfg.Fault.BadProofAt != 0 && r.cfg.Fault.BadProofAt == commit.BlockIndex {
			if len(proof.ProofBytes) > 0 {
				proof.ProofBytes[0] ^= 0xff
			}
		}

		req := SubmitRequest{
			Phase:          commit.Phase,
			BlockIndex:     commit.BlockIndex,
			CommitmentHash: commit.CommitmentHash,
			Proof:          proof,
			TxCount:        commit.TxCount,
			Txs:            commit.Txs,
			CreatedAt:      commit.CreatedAt,
		}
		_ = r.l1.SubmitCertifiedBlock(req)
		r.updateQueueDepth()
	}
}

func (r *Runner) updateQueueDepth() {
	total := r.ingressQ.Len() + r.laneQ.Len() + r.commitQ.Len()
	r.metrics.SetQueueDepth(total)
}

func GenerateSyntheticTxs(blockIndex uint32, txCount int) []frame.Tx12 {
	if txCount <= 0 {
		return nil
	}
	txs := make([]frame.Tx12, txCount)
	base := uint64(blockIndex) * 1_000_000
	for i := 0; i < txCount; i++ {
		sender := uint32(base + uint64(i))
		receiver := sender ^ 0x9e3779b9
		amount := uint32((i % 4096) + 1)
		txs[i] = frame.Tx12{SenderIndex: sender, ReceiverIndex: receiver, Amount: amount}
	}
	return txs
}

func isPayloadValid(tx frame.Tx12) bool {
	return tx.Amount > 0
}

func (r *Runner) buildReport(started, finished time.Time) (Report, error) {
	logs := r.l1.BlockLogs()
	certVerifiedTx := uint64(0)
	certProofFailures := 0
	certVerifiedBlocks := 0
	for _, entry := range logs {
		if entry.Phase != "certification" {
			continue
		}
		if entry.Finalized && entry.ProofVerified && !entry.Duplicate {
			certVerifiedTx += uint64(entry.TxCount)
			certVerifiedBlocks++
		} else if !entry.ProofVerified {
			certProofFailures++
		}
	}

	certDuration := r.cfg.Certification.DurationSeconds
	avgVerifiedTPS := 0.0
	if certDuration > 0 {
		avgVerifiedTPS = float64(certVerifiedTx) / float64(certDuration)
	}
	expectedCertTx := uint64(r.cfg.BlocksForPhase(r.cfg.Certification) * r.cfg.TxPerBlock(r.cfg.Certification.TPS))

	cfgHash := ""
	if r.configPath != "" {
		if hash, err := fileSHA256Hex(r.configPath); err == nil {
			cfgHash = hash
		}
	}
	binaryHash := ""
	if exe, err := os.Executable(); err == nil {
		if hash, err := fileSHA256Hex(exe); err == nil {
			binaryHash = hash
		}
	}
	host, _ := os.Hostname()

	pass := avgVerifiedTPS >= float64(r.cfg.Certification.TPS) &&
		certVerifiedTx >= expectedCertTx &&
		r.metrics.DropCount() == 0 &&
		certProofFailures == 0

	report := Report{
		ConfigName:              r.cfg.Name,
		StartedAt:               started.UTC().Format(time.RFC3339),
		FinishedAt:              finished.UTC().Format(time.RFC3339),
		Host:                    host,
		GoVersion:               runtime.Version(),
		ConfigSHA256:            cfgHash,
		BinarySHA256:            binaryHash,
		BlockTimeMillis:         r.cfg.BlockTimeMillis,
		GasLimit:                r.cfg.GasLimit,
		Lanes:                   r.cfg.Lanes,
		ExpectedCertificationTx: expectedCertTx,
		CertificationSeconds:    r.cfg.Certification.DurationSeconds,
		CertificationTPSGoal:    r.cfg.Certification.TPS,
		CertificationVerifiedTx: certVerifiedTx,
		CertificationAvgTPS:     avgVerifiedTPS,
		CertifiedBlocks:         certVerifiedBlocks,
		ProofFailures:           certProofFailures,
		DropCount:               r.metrics.DropCount(),
		AverageFinalizeMs:       r.metrics.AverageFinalizeLatencyMs(),
		Pass:                    pass,
		Assumptions: []string{
			"Certification mode skips signature/nonce/balance prechecks and full trie semantics.",
			"L1 receives full tx payload per proof and applies one batched state update per accepted proof.",
			"Proof verification uses deterministic pre-generated certification proofs.",
			"Result is for private-network certification and not public-network decentralization/security.",
		},
		Blocks: logs,
	}
	return report, nil
}

func fileSHA256Hex(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func EnsureReportDir(path string) error {
	return os.MkdirAll(filepath.Clean(path), 0o755)
}
