package pipeline

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"hyperl2/internal/frame"
)

type proofLine struct {
	BlockIndex uint32 `json:"block_index"`
	ProofHex   string `json:"proof_hex"`
}

type ProofBook struct {
	seed  string
	proof map[uint32][]byte
}

func NewProofBook(seed string) *ProofBook {
	return &ProofBook{
		seed:  seed,
		proof: make(map[uint32][]byte),
	}
}

func (b *ProofBook) EnsureBlock(blockIndex uint32) []byte {
	if p, ok := b.proof[blockIndex]; ok {
		return p
	}
	p := GenerateProofBytes(b.seed, blockIndex)
	b.proof[blockIndex] = p
	return p
}

func (b *ProofBook) ProofForBlock(blockIndex uint32) []byte {
	return b.EnsureBlock(blockIndex)
}

func (b *ProofBook) SaveJSONL(path string, blocks int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create proof dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create proof file: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for i := 0; i < blocks; i++ {
		bi := uint32(i)
		line := proofLine{
			BlockIndex: bi,
			ProofHex:   hex.EncodeToString(b.EnsureBlock(bi)),
		}
		if err := enc.Encode(line); err != nil {
			return fmt.Errorf("encode proof line: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush proof file: %w", err)
	}
	return nil
}

func (b *ProofBook) LoadJSONL(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open proof file: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var line proofLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return fmt.Errorf("parse proof line: %w", err)
		}
		proofBytes, err := hex.DecodeString(line.ProofHex)
		if err != nil {
			return fmt.Errorf("decode proof hex: %w", err)
		}
		b.proof[line.BlockIndex] = proofBytes
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan proof file: %w", err)
	}
	return nil
}

func BuildProofRecord(blockIndex uint32, commitment [32]byte, proofBytes []byte) frame.ProofRecordV1 {
	dup := make([]byte, len(proofBytes))
	copy(dup, proofBytes)
	return frame.ProofRecordV1{
		BlockIndex:     blockIndex,
		CommitmentHash: commitment,
		ProofBytes:     dup,
	}
}

func GenerateProofBytes(seed string, blockIndex uint32) []byte {
	seedHash := sha256.Sum256([]byte(seed))
	payload := make([]byte, 4+len(seedHash))
	binary.LittleEndian.PutUint32(payload[0:4], blockIndex)
	copy(payload[4:], seedHash[:])
	sum := sha256.Sum256(payload)
	out := make([]byte, len(sum))
	copy(out, sum[:])
	return out
}
