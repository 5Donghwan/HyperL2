package pipeline

type LaneBatchEnvelope struct {
	Phase           string `json:"phase"`
	BlockIndex      uint32 `json:"block_index"`
	LaneID          uint16 `json:"lane_id"`
	BatchHex        string `json:"batch_hex"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms"`
}

type BlockCommitEnvelope struct {
	Phase             string `json:"phase"`
	BlockIndex        uint32 `json:"block_index"`
	CommitmentHashHex string `json:"commitment_hash_hex"`
	TxCount           uint32 `json:"tx_count"`
	TxPayloadHex      string `json:"tx_payload_hex"`
	CreatedAtUnixMs   int64  `json:"created_at_unix_ms"`
}

type L1SubmitEnvelope struct {
	Phase             string `json:"phase"`
	BlockIndex        uint32 `json:"block_index"`
	CommitmentHashHex string `json:"commitment_hash_hex"`
	ProofHex          string `json:"proof_hex"`
	TxCount           uint32 `json:"tx_count"`
	TxPayloadHex      string `json:"tx_payload_hex"`
	CreatedAtUnixMs   int64  `json:"created_at_unix_ms"`
}
