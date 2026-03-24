package pipeline

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"hyperl2/internal/frame"
	"hyperl2/internal/metrics"
)

type acceptedBlock struct {
	Commitment [32]byte
	TxCount    uint32
	StateVer   uint64
	DeltaCount int
}

type L1CertNode struct {
	proofSeed string
	metrics   *metrics.Registry

	mu           sync.Mutex
	blocks       map[uint32]acceptedBlock
	blockLog     []BlockLog
	state        map[uint32]int64
	stateVersion uint64
}

func NewL1CertNode(proofSeed string, m *metrics.Registry) *L1CertNode {
	return &L1CertNode{
		proofSeed: proofSeed,
		metrics:   m,
		blocks:    make(map[uint32]acceptedBlock),
		blockLog:  make([]BlockLog, 0, 256),
		state:     make(map[uint32]int64),
	}
}

func (n *L1CertNode) SubmitCertifiedBlock(req SubmitRequest) SubmitResponse {
	n.mu.Lock()
	defer n.mu.Unlock()

	latencyMs := uint64(time.Since(req.CreatedAt).Milliseconds())
	proofOK := bytes.Equal(req.Proof.ProofBytes, GenerateProofBytes(n.proofSeed, req.BlockIndex))
	payloadOK := true
	if req.Proof.BlockIndex != req.BlockIndex {
		proofOK = false
	}
	if req.Proof.CommitmentHash != req.CommitmentHash {
		proofOK = false
	}
	if req.TxCount != uint32(len(req.Txs)) {
		payloadOK = false
	}
	for _, tx := range req.Txs {
		if tx.Amount == 0 {
			payloadOK = false
			break
		}
	}
	if expected := CommitmentFromTxs(req.BlockIndex, req.Txs); expected != req.CommitmentHash {
		payloadOK = false
	}

	if existing, ok := n.blocks[req.BlockIndex]; ok {
		if existing.Commitment == req.CommitmentHash && existing.TxCount == req.TxCount {
			entry := BlockLog{
				Phase:          req.Phase,
				BlockIndex:     req.BlockIndex,
				TxCount:        req.TxCount,
				ProofVerified:  proofOK,
				Finalized:      proofOK,
				Duplicate:      true,
				StateApplied:   false,
				StateVersion:   existing.StateVer,
				DeltaCount:     existing.DeltaCount,
				FinalizeMs:     float64(latencyMs),
				CommitmentHash: hex.EncodeToString(req.CommitmentHash[:]),
			}
			n.blockLog = append(n.blockLog, entry)
			return SubmitResponse{
				Accepted:   proofOK && payloadOK,
				Duplicate:  true,
				ProofOK:    proofOK,
				Finalized:  proofOK && payloadOK,
				Reason:     "duplicate submission",
				LatencyMs:  latencyMs,
				BlockIndex: req.BlockIndex,
			}
		}
		entry := BlockLog{
			Phase:          req.Phase,
			BlockIndex:     req.BlockIndex,
			TxCount:        req.TxCount,
			ProofVerified:  false,
			Finalized:      false,
			StateApplied:   false,
			StateVersion:   n.stateVersion,
			DeltaCount:     len(req.Txs),
			Error:          "duplicate block index with different commitment",
			FinalizeMs:     float64(latencyMs),
			CommitmentHash: hex.EncodeToString(req.CommitmentHash[:]),
		}
		n.blockLog = append(n.blockLog, entry)
		return SubmitResponse{
			Accepted:   false,
			Duplicate:  true,
			ProofOK:    false,
			Finalized:  false,
			Reason:     "duplicate block index with different commitment",
			LatencyMs:  latencyMs,
			BlockIndex: req.BlockIndex,
		}
	}

	if !payloadOK {
		entry := BlockLog{
			Phase:          req.Phase,
			BlockIndex:     req.BlockIndex,
			TxCount:        req.TxCount,
			ProofVerified:  proofOK,
			Finalized:      false,
			StateApplied:   false,
			StateVersion:   n.stateVersion,
			DeltaCount:     len(req.Txs),
			Error:          "payload validation failed or commitment mismatch",
			FinalizeMs:     float64(latencyMs),
			CommitmentHash: hex.EncodeToString(req.CommitmentHash[:]),
		}
		n.blockLog = append(n.blockLog, entry)
		return SubmitResponse{
			Accepted:   false,
			ProofOK:    proofOK,
			Finalized:  false,
			Reason:     "payload validation failed",
			LatencyMs:  latencyMs,
			BlockIndex: req.BlockIndex,
		}
	}

	if !proofOK {
		entry := BlockLog{
			Phase:          req.Phase,
			BlockIndex:     req.BlockIndex,
			TxCount:        req.TxCount,
			ProofVerified:  false,
			Finalized:      false,
			StateApplied:   false,
			StateVersion:   n.stateVersion,
			DeltaCount:     len(req.Txs),
			Error:          "proof verification failed",
			FinalizeMs:     float64(latencyMs),
			CommitmentHash: hex.EncodeToString(req.CommitmentHash[:]),
		}
		n.blockLog = append(n.blockLog, entry)
		return SubmitResponse{
			Accepted:   false,
			ProofOK:    false,
			Finalized:  false,
			Reason:     "proof verification failed",
			LatencyMs:  latencyMs,
			BlockIndex: req.BlockIndex,
		}
	}

	deltaCount := n.applyTxs(req.Txs)
	n.stateVersion++

	n.blocks[req.BlockIndex] = acceptedBlock{
		Commitment: req.CommitmentHash,
		TxCount:    req.TxCount,
		StateVer:   n.stateVersion,
		DeltaCount: deltaCount,
	}
	n.metrics.AddVerifiedTx(uint64(req.TxCount))
	n.metrics.ObserveFinalizeLatencyMs(latencyMs)

	entry := BlockLog{
		Phase:          req.Phase,
		BlockIndex:     req.BlockIndex,
		TxCount:        req.TxCount,
		ProofVerified:  true,
		Finalized:      true,
		StateApplied:   true,
		StateVersion:   n.stateVersion,
		DeltaCount:     deltaCount,
		FinalizeMs:     float64(latencyMs),
		CommitmentHash: hex.EncodeToString(req.CommitmentHash[:]),
	}
	n.blockLog = append(n.blockLog, entry)

	return SubmitResponse{
		Accepted:   true,
		ProofOK:    true,
		Finalized:  true,
		Reason:     "ok",
		LatencyMs:  latencyMs,
		BlockIndex: req.BlockIndex,
	}
}

func (n *L1CertNode) BlockLogs() []BlockLog {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]BlockLog, len(n.blockLog))
	copy(out, n.blockLog)
	return out
}

func (n *L1CertNode) VerifiedBlocks() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.blocks)
}

func (n *L1CertNode) MustBlock(blockIndex uint32) (acceptedBlock, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, ok := n.blocks[blockIndex]
	if !ok {
		return acceptedBlock{}, fmt.Errorf("block %d not found", blockIndex)
	}
	return b, nil
}

func (n *L1CertNode) applyTxs(txs []frame.Tx12) int {
	if len(txs) == 0 {
		return 0
	}
	changed := make(map[uint32]struct{}, len(txs)*2)
	for _, tx := range txs {
		amount := int64(tx.Amount)
		n.state[tx.SenderIndex] -= amount
		n.state[tx.ReceiverIndex] += amount
		changed[tx.SenderIndex] = struct{}{}
		changed[tx.ReceiverIndex] = struct{}{}
	}
	return len(changed)
}

func (n *L1CertNode) Balance(index uint32) int64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.state[index]
}

func (n *L1CertNode) StateVersion() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.stateVersion
}
