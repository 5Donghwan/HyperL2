# TX Dataset

Generated files:
- `tx_dataset.bin`: concatenated `Tx12` payloads (12 bytes each)
- `tx_dataset_manifest.json`: block offset and tx count index

Create with:

```bash
go run ./cmd/bench-orchestrator --config configs/cert-ultra/config.json --mode generate-datasets
```
