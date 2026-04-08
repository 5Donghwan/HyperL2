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
- `scripts/run-kaia-16p6s-dashboard.sh`
- `scripts/run-kaia-16p6s-repeat.sh`
- `scripts/render-kaia-service-rpc-conf.sh`
- `scripts/stage-mbp-rpc-bundle.sh`
- `scripts/push-mbp-rpc-bundle.sh`
- `scripts/check-kaia-rpc.sh`
- `./kaia-proofctl`

## Progress Dashboard

You can watch proof generation and L1 submission progress in a browser while a run is active.

## Recommended Public-RPC Preset

The current best public-RPC operating point is:

- `16 proofs / call`
- `6 senders`
- `6 fresh certifiers`
- `1 live proof + 15 replay proofs`

Measured on 2026-03-24 against `https://testnet.zkrypton.zkrypto.com`:

- average: `579,028 TPS`
- median: `509,689 TPS`
- worst: `237,477 TPS`
- best: `1,072,626 TPS`
- `200k+ TPS` success: `5 / 5`

Comparison report:

- `reports/12p-8s-vs-16p-6s-compare-20260324.md`
- `reports/16p-6s-repeat-report-20260324.md`

Environment:

```bash
export KAIA_RPC_URL="https://testnet.zkrypton.zkrypto.com"
export KAIA_VERIFIER_ADDRESS="0xeb654CdB749ECe55B656A9382fC38fB48Ee2eF95"
export KAIA_PRIVATE_KEYS="<six-comma-separated-funded-private-keys>"
```

Run the recommended dashboard flow:

```bash
scripts/run-kaia-16p6s-dashboard.sh
```

Run the recommended repeated benchmark flow:

```bash
scripts/run-kaia-16p6s-repeat.sh
```

Generation-only dashboard:

```bash
./kaia-proofctl dashboard \
  --listen :8088 \
  --batch-size 20000 \
  --lane-count 10 \
  --bundle-path build/dashboard/certification-bundle.json
```

Then open `http://127.0.0.1:8088`.

What it shows:

- proof setup progress
- `N / total` proof generation progress
- generated certified tx total
- L1 submission count, receipt count, success/failure count
- current TPS based on L1 block inclusion
- receipt-visible TPS as an operator-side auxiliary metric
- per-transaction gas, block number, latency, tx hash

To enable live L1 submission tracking, provide the endpoint and submission targets:

```bash
./kaia-proofctl dashboard \
  --listen :8088 \
  --skip-generate \
  --bundle-path build/kaia-preflight/certification-bundle.json \
  --rpc-url https://testnet.zkrypton.zkrypto.com \
  --private-keys <comma-separated-private-keys> \
  --certifier-addresses <comma-separated-certifier-addresses>
```

Recommended `16 proofs / 6 senders` dashboard helper:

```bash
scripts/run-kaia-16p6s-dashboard.sh
```

This helper will:

- build `./kaia-proofctl`
- generate a `16`-proof replay bundle if missing
- deploy `6` fresh certifiers
- initialize their lane heads
- launch the dashboard with `1 live + 15 replay proofs`


## Dedicated MBP RPC

If you have an accessible MacBook Pro and want to stop depending on the shared public zkrypto RPC path, use the MBP as a dedicated Kaia RPC node instead of a simple HTTP proxy.

Based on the current lab bootstrap files (`static-nodes.json` with `cn` / `pn` peers), the default target is an Endpoint Node (`ken` / `kend`) rather than a service-chain endpoint.

After the `2026-04-08` zkrypto testnet restart, the refreshed lab bootstrap uses:

- `chainId = 4669126`
- `networkId = 4669126`
- peer subnet `172.168.10.x`

Added materials:

- `/Users/5d0ng/dev/HyperL2/docs/mbp-dedicated-rpc-setup.md`
- `/Users/5d0ng/dev/HyperL2/configs/mbp-rpc/service-chain.env.example`
- `/Users/5d0ng/dev/HyperL2/scripts/render-kaia-service-rpc-conf.sh`
- `/Users/5d0ng/dev/HyperL2/scripts/stage-mbp-rpc-bundle.sh`
- `/Users/5d0ng/dev/HyperL2/scripts/push-mbp-rpc-bundle.sh`
- `/Users/5d0ng/dev/HyperL2/scripts/check-kaia-rpc.sh`

Stage the bootstrap bundle for the MBP:

```bash
scripts/stage-mbp-rpc-bundle.sh \
  configs/mbp-rpc/service-chain.env.example \
  build/mbp-rpc/staged
```

Once the MBP node is synchronized and healthy, point HyperL2 at it with:

```bash
export KAIA_RPC_URL="http://<MBP-IP>:8551"
scripts/run-kaia-16p6s-repeat.sh
```

## Output

Each run writes:

- JSON report with pass/fail, TPS, proof results, block logs
- CSV block log report
- Markdown summary report

Under the configured `report_dir`.

## Certification Assumptions

In this repository, `TPS` refers to `L1 block-inclusion throughput` unless stated otherwise.

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
