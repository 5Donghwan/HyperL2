package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"hyperl2/internal/config"
)

type TxBlockIndex struct {
	BlockIndex  uint32 `json:"block_index"`
	OffsetBytes int64  `json:"offset_bytes"`
	TxCount     int    `json:"tx_count"`
	Phase       string `json:"phase"`
}

type TxDatasetManifest struct {
	Version string         `json:"version"`
	Name    string         `json:"name"`
	Blocks  []TxBlockIndex `json:"blocks"`
}

type DatasetPaths struct {
	TxBlobPath       string
	TxManifestPath   string
	ProofDatasetPath string
}

func GenerateDatasets(cfg config.Config, txDir, proofDir string) (DatasetPaths, error) {
	if err := os.MkdirAll(txDir, 0o755); err != nil {
		return DatasetPaths{}, fmt.Errorf("create tx dir: %w", err)
	}
	if err := os.MkdirAll(proofDir, 0o755); err != nil {
		return DatasetPaths{}, fmt.Errorf("create proof dir: %w", err)
	}

	txBlobPath := filepath.Join(txDir, "tx_dataset.bin")
	blob, err := os.Create(txBlobPath)
	if err != nil {
		return DatasetPaths{}, fmt.Errorf("create tx blob: %w", err)
	}
	defer blob.Close()

	manifest := TxDatasetManifest{
		Version: "v1",
		Name:    cfg.Name,
		Blocks:  make([]TxBlockIndex, 0),
	}

	offset := int64(0)
	blockIndex := uint32(0)
	phases := []struct {
		name  string
		phase config.Phase
	}{
		{name: "warmup", phase: cfg.Warmup},
		{name: "certification", phase: cfg.Certification},
	}

	for _, p := range phases {
		blocks := cfg.BlocksForPhase(p.phase)
		txPerBlock := cfg.TxPerBlock(p.phase.TPS)
		for i := 0; i < blocks; i++ {
			txs := GenerateSyntheticTxs(blockIndex, txPerBlock)
			for _, tx := range txs {
				enc := tx.MarshalBinary()
				n, err := blob.Write(enc[:])
				if err != nil {
					return DatasetPaths{}, fmt.Errorf("write tx blob: %w", err)
				}
				offset += int64(n)
			}
			manifest.Blocks = append(manifest.Blocks, TxBlockIndex{
				BlockIndex:  blockIndex,
				OffsetBytes: offset - int64(txPerBlock*12),
				TxCount:     txPerBlock,
				Phase:       p.name,
			})
			blockIndex++
		}
	}

	txManifestPath := filepath.Join(txDir, "tx_dataset_manifest.json")
	manifestPayload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return DatasetPaths{}, fmt.Errorf("marshal tx manifest: %w", err)
	}
	if err := os.WriteFile(txManifestPath, manifestPayload, 0o644); err != nil {
		return DatasetPaths{}, fmt.Errorf("write tx manifest: %w", err)
	}

	proofPath := filepath.Join(proofDir, "proof_dataset.jsonl")
	proofBook := NewProofBook(cfg.ProofSeed)
	if err := proofBook.SaveJSONL(proofPath, int(blockIndex)); err != nil {
		return DatasetPaths{}, fmt.Errorf("write proof dataset: %w", err)
	}

	return DatasetPaths{
		TxBlobPath:       txBlobPath,
		TxManifestPath:   txManifestPath,
		ProofDatasetPath: proofPath,
	}, nil
}
