package frame

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	BatchMagic uint32 = 0x42463131 // BF11
	VersionV1  uint16 = 1
)

var (
	ErrTx12Size        = errors.New("tx12 payload must be exactly 12 bytes")
	ErrBadBatchMagic   = errors.New("invalid batch magic")
	ErrBadBatchVer     = errors.New("unsupported batch version")
	ErrShortBatch      = errors.New("batch payload too short")
	ErrTxCountMismatch = errors.New("batch tx_count does not match payload length")
	ErrTxSliceSize     = errors.New("tx slice byte length must be multiple of 12")
)

// Tx12 is the fixed certification payload.
type Tx12 struct {
	SenderIndex   uint32
	ReceiverIndex uint32
	Amount        uint32
}

func (t Tx12) MarshalBinary() [12]byte {
	var out [12]byte
	binary.LittleEndian.PutUint32(out[0:4], t.SenderIndex)
	binary.LittleEndian.PutUint32(out[4:8], t.ReceiverIndex)
	binary.LittleEndian.PutUint32(out[8:12], t.Amount)
	return out
}

func UnmarshalTx12(payload []byte) (Tx12, error) {
	if len(payload) != 12 {
		return Tx12{}, ErrTx12Size
	}
	return Tx12{
		SenderIndex:   binary.LittleEndian.Uint32(payload[0:4]),
		ReceiverIndex: binary.LittleEndian.Uint32(payload[4:8]),
		Amount:        binary.LittleEndian.Uint32(payload[8:12]),
	}, nil
}

func EncodeTxSlice(txs []Tx12) []byte {
	if len(txs) == 0 {
		return nil
	}
	out := make([]byte, len(txs)*12)
	offset := 0
	for _, tx := range txs {
		enc := tx.MarshalBinary()
		copy(out[offset:offset+12], enc[:])
		offset += 12
	}
	return out
}

func DecodeTxSlice(payload []byte) ([]Tx12, error) {
	if len(payload)%12 != 0 {
		return nil, ErrTxSliceSize
	}
	if len(payload) == 0 {
		return nil, nil
	}
	out := make([]Tx12, len(payload)/12)
	offset := 0
	for i := range out {
		tx, err := UnmarshalTx12(payload[offset : offset+12])
		if err != nil {
			return nil, err
		}
		out[i] = tx
		offset += 12
	}
	return out, nil
}

// BatchFrameV1 stores one lane payload for a specific block.
type BatchFrameV1 struct {
	Flags      uint16
	BlockIndex uint32
	LaneID     uint16
	Payload    []Tx12
}

func (b BatchFrameV1) TxCount() uint32 {
	return uint32(len(b.Payload))
}

func (b BatchFrameV1) MarshalBinary() []byte {
	txCount := b.TxCount()
	headerSize := 4 + 2 + 2 + 4 + 2 + 2 + 4
	payloadSize := int(txCount) * 12
	out := make([]byte, headerSize+payloadSize)

	binary.LittleEndian.PutUint32(out[0:4], BatchMagic)
	binary.LittleEndian.PutUint16(out[4:6], VersionV1)
	binary.LittleEndian.PutUint16(out[6:8], b.Flags)
	binary.LittleEndian.PutUint32(out[8:12], b.BlockIndex)
	binary.LittleEndian.PutUint16(out[12:14], b.LaneID)
	binary.LittleEndian.PutUint16(out[14:16], 0)
	binary.LittleEndian.PutUint32(out[16:20], txCount)

	offset := 20
	for _, tx := range b.Payload {
		enc := tx.MarshalBinary()
		copy(out[offset:offset+12], enc[:])
		offset += 12
	}
	return out
}

func (b *BatchFrameV1) UnmarshalBinary(payload []byte) error {
	if len(payload) < 20 {
		return ErrShortBatch
	}
	if got := binary.LittleEndian.Uint32(payload[0:4]); got != BatchMagic {
		return ErrBadBatchMagic
	}
	if got := binary.LittleEndian.Uint16(payload[4:6]); got != VersionV1 {
		return ErrBadBatchVer
	}

	b.Flags = binary.LittleEndian.Uint16(payload[6:8])
	b.BlockIndex = binary.LittleEndian.Uint32(payload[8:12])
	b.LaneID = binary.LittleEndian.Uint16(payload[12:14])
	txCount := binary.LittleEndian.Uint32(payload[16:20])

	expected := 20 + int(txCount)*12
	if len(payload) != expected {
		return fmt.Errorf("%w: expected %d bytes, got %d", ErrTxCountMismatch, expected, len(payload))
	}

	b.Payload = make([]Tx12, txCount)
	offset := 20
	for i := 0; i < int(txCount); i++ {
		tx, err := UnmarshalTx12(payload[offset : offset+12])
		if err != nil {
			return err
		}
		b.Payload[i] = tx
		offset += 12
	}
	return nil
}

func (b BatchFrameV1) CommitmentHash() [32]byte {
	return sha256.Sum256(b.MarshalBinary())
}

// ProofRecordV1 is the proof payload submitted to L1 cert node.
type ProofRecordV1 struct {
	BlockIndex     uint32
	CommitmentHash [32]byte
	ProofBytes     []byte
}

func (p ProofRecordV1) CommitmentHex() string {
	return hex.EncodeToString(p.CommitmentHash[:])
}
