# HyperL2 Certification Pipeline

This repository implements a certification-focused high-throughput pipeline for private networks:

- L2 executes and batches synthetic `Tx12` payloads.
- L1 verifies deterministic certification proofs and applies state deltas once per accepted proof/batch.
- Throughput success metric is **L1 verified TPS**.

## Project Layout

- `cmd/l2-ingress`: load generator / stream sender (`Tx12` binary stream)
- `cmd/l2-sequencer`: stream receiver, lane distribution, lane batch emission
- `cmd/l2-aggregator`: lane commitment aggregation
- `cmd/proof-replayer`: block commitment to proof submission bridge
- `cmd/l1-cert-node`: certification verifier node API
- `cmd/bench-orchestrator`: in-process end-to-end benchmark runner and report generator
- `internal/frame`: `Tx12`, `BatchFrameV1`, `ProofRecordV1`
- `internal/queue`: lock-free MPMC ring queue
- `internal/metrics`: metrics registry and `/metrics` exporter
- `internal/pipeline`: pipeline runtime, proof book, report writer, dataset generator

## Config Profiles

- `configs/dev-air/config.json`: M3 MacBook Air development profile (`20k warmup`, `60k certification`)
- `configs/dev-air/smoke.json`: short local smoke profile
- `configs/cert-ultra/config.json`: Mac Ultra certification profile (`20k warmup`, `200k certification`)

## Run

Smoke test:

```bash
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go run ./cmd/bench-orchestrator --config configs/dev-air/smoke.json --mode run
```

Generate datasets:

```bash
GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
  go run ./cmd/bench-orchestrator --config configs/cert-ultra/config.json --mode generate-datasets
```

Scripts:

- `scripts/run-air-dev.sh`
- `scripts/run-ultra-cert.sh`
- `scripts/run-vectis-sumproof-bench.sh`
- `scripts/prepare-kaia-preflight.sh`
- `./kaia-proofctl`

## Output

Each run writes:

- JSON report with pass/fail, TPS, proof results, block logs
- CSV block log report
- Markdown summary report

Under the configured `report_dir`.

## Certification Assumptions

- Certification mode skips signature/nonce/double-spend/per-tx validity checks.
- The Rust proving path models a fixed-slot state transition:
  - `D1`: Pedersen commitment to the pre-state vector
  - `D2`: Pedersen commitment to the post-state vector
- Each proof only certifies `sum(open(D1)) == sum(open(D2))`.
- `10` verified batch-transition proofs represent `200,000 tx` certification.
- Numbers are for private-network certification, not public-network decentralization/security claims.

## Experimental Rust Prover

- `rust/vectis-prover`: VECTIS-based `ccGroth16` prover crate for HyperL2 batch-transition research
- The new `hyperl2` module models a `20k tx -> 1 proof` flow where each proof binds two Pedersen vector commitments:
  - `D1`: pre-state commitment for a fixed 20k-slot account table
  - `D2`: post-state commitment for the same slot table
- The proof relation is intentionally minimal: `sum(open(D1)) == sum(open(D2))`
- `10` verified batch-transition proofs correspond to `200,000 tx` certification
- Bench helper: `scripts/run-vectis-sumproof-bench.sh`
- Endpoint preflight:
  - `scripts/prepare-kaia-preflight.sh`
  - `docs/kaia-endpoint-preflight.md`
  - `configs/kaia-endpoint.env.example`
- Endpoint client:
  - `cmd/kaia-proofctl`
  - `./kaia-proofctl discover --rpc-url https://testnet.zkrypton.zkrypto.com`
- On-chain verifier:
  - `contracts/CCGroth16BatchVerifier.sol`: BN254 precompile-based ccGroth16 verifier
  - `contracts/SumPreservingBatchVerifier.sol`: `10`-proof certification contract and lane-head bookkeeping
- Solidity/export helper:
  - `cargo +stable run --manifest-path rust/vectis-prover/Cargo.toml --bin export_sumproof_fixture -- --batch-size 20000`
  - `cargo +stable run --release --manifest-path rust/vectis-prover/Cargo.toml --bin export_sumproof_bundle -- --batch-size 20000 --lane-count 10`
