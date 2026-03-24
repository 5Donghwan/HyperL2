package pipeline

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"hyperl2/internal/frame"
)

type BlockJob struct {
	Phase      string
	BlockIndex uint32
	Txs        []frame.Tx12
	CreatedAt  time.Time
}

type LaneBatch struct {
	Phase      string
	BlockIndex uint32
	LaneID     uint16
	Payload    []frame.Tx12
	CreatedAt  time.Time
}

type BlockCommit struct {
	Phase          string
	BlockIndex     uint32
	CommitmentHash [32]byte
	TxCount        uint32
	Txs            []frame.Tx12
	CreatedAt      time.Time
}

type SubmitRequest struct {
	Phase          string
	BlockIndex     uint32
	CommitmentHash [32]byte
	Proof          frame.ProofRecordV1
	TxCount        uint32
	Txs            []frame.Tx12
	CreatedAt      time.Time
}

type SubmitResponse struct {
	Accepted   bool
	Duplicate  bool
	ProofOK    bool
	Finalized  bool
	Reason     string
	LatencyMs  uint64
	BlockIndex uint32
}

type BlockLog struct {
	Phase          string  `json:"phase"`
	BlockIndex     uint32  `json:"block_index"`
	TxCount        uint32  `json:"tx_count"`
	ProofVerified  bool    `json:"proof_verified"`
	Finalized      bool    `json:"finalized"`
	Duplicate      bool    `json:"duplicate"`
	StateApplied   bool    `json:"state_applied"`
	StateVersion   uint64  `json:"state_version"`
	DeltaCount     int     `json:"delta_count"`
	Error          string  `json:"error,omitempty"`
	FinalizeMs     float64 `json:"finalize_latency_ms"`
	CommitmentHash string  `json:"commitment_hash"`
}

func AggregateCommitment(laneHashes map[uint16][32]byte) ([32]byte, error) {
	if len(laneHashes) == 0 {
		return [32]byte{}, fmt.Errorf("cannot aggregate empty lane hash map")
	}
	lanes := make([]int, 0, len(laneHashes))
	for laneID := range laneHashes {
		lanes = append(lanes, int(laneID))
	}
	sort.Ints(lanes)

	payload := make([]byte, 0, len(lanes)*34)
	for _, lane := range lanes {
		laneID := uint16(lane)
		var laneBuf [2]byte
		binary.LittleEndian.PutUint16(laneBuf[:], laneID)
		payload = append(payload, laneBuf[:]...)
		h := laneHashes[laneID]
		payload = append(payload, h[:]...)
	}
	return sha256.Sum256(payload), nil
}

func BuildLaneBatchFrame(blockIndex uint32, laneID uint16, payload []frame.Tx12) frame.BatchFrameV1 {
	return frame.BatchFrameV1{
		Flags:      0,
		BlockIndex: blockIndex,
		LaneID:     laneID,
		Payload:    payload,
	}
}

func CommitmentFromTxs(blockIndex uint32, txs []frame.Tx12) [32]byte {
	txPayload := frame.EncodeTxSlice(txs)
	in := make([]byte, 4+len(txPayload))
	binary.LittleEndian.PutUint32(in[0:4], blockIndex)
	copy(in[4:], txPayload)
	return sha256.Sum256(in)
}
