# HyperL2 — Sum-Preserving Batch Certification Pipeline (L2 → L1)

**What this is.** An end-to-end research pipeline that batches 20,000 transfers on an L2, proves each batch as a single *sum-preserving state-transition proof* (commit-and-prove Groth16 over Pedersen vector commitments), and has an EVM L1 verify only the proof and the two state commitments. Built to measure **L1-verified throughput** for the final report of an IITP-funded blockchain finality/scalability research project (2021–2025); implemented and measured in early 2026.

**What it is not.** This is *not* a full validity rollup. The proof certifies `sum(pre-state) == sum(post-state)` for a batch; it does **not** check signatures, nonces, double-spends, or per-transaction validity. See [docs/current state.md](docs/current%20state.md).

## Architecture

```
ingress ──► sequencer ──► aggregator ──► proof-replayer ──► L1 cert node ──► EVM L1
(load gen)  (lanes,        (lane          (proof            (verify +        (CCGroth16BatchVerifier
             batches)       commitments)   submission)       submit)          SumPreservingBatchVerifier)
```

| Layer | Language | Components |
|---|---|---|
| L2 pipeline | Go | `cmd/l2-ingress`, `cmd/l2-sequencer`, `cmd/l2-aggregator`, `cmd/proof-replayer`, `cmd/l1-cert-node`, `cmd/bench-orchestrator`, lock-free MPMC ring queue, Prometheus metrics |
| Prover | Rust | `rust/vectis-prover/src/hyperl2/` — sum-preserving batch circuit, `prove_batch`/`verify_batch`, slot-map state builder, benchmark binaries. Built on a vendored copy of the lab's **VECTIS** ccGroth16 library |
| L1 verifier | Solidity | `contracts/CCGroth16BatchVerifier.sol` (BN254 precompiles, pairing check), `contracts/SumPreservingBatchVerifier.sol` (lane heads, D1→D2 advance, certified-tx accounting) |
| Ops | Go/JS | `kaia-proofctl dashboard` — proof generation, submission, receipts, gas, block height |

## Proof model (one batch = one proof)

- `D1 = Com(pre-state)`, `D2 = Com(post-state)` — Pedersen vector commitments built *outside* the circuit.
- Relation proven: prover knows openings of `D1`, `D2` and `sum(open(D1)) == sum(open(D2))`.
- **Binding.** `tau = H(domain ‖ lane_id ‖ batch_id ‖ tx_count ‖ D1 ‖ D2 ‖ d0)`; the aggregated witness `A = tau·X + tau²·Y` ties the proof to the external commitments. L1 recomputes `tau`, forms `Agg = tau·D1 + tau²·D2`, adjusts `proof.d`, and runs the ccGroth16 pairing check — so committed values never appear as public inputs.
- Details: [rust/vectis-prover/docs/hyperl2-sumproof.md](rust/vectis-prover/docs/hyperl2-sumproof.md)

## Results (public RPC, `testnet.zkrypton.zkrypto.com`, 2026-03-24)

| Metric | Value |
|---|---|
| Proof generation (20k-tx batch, single machine) | ~0.474 s |
| Off-chain verification | ~0.002 s / proof |
| On-chain cost | 3 G1 scalar mults + 1 pairing check per proof |
| L1-verified throughput, 16 proofs/call · 6 senders (5 runs) | avg **579,028 TPS**, median 509,689, worst 237,477, ≥200k in 5/5 |

Throughput runs use 1 freshly generated proof + 15 replayed proofs per call to measure the **L1 verification path** rather than prover throughput. Full comparison: [reports/](reports/).

## Run

```bash
# smoke test (dev profile)
go run ./cmd/bench-orchestrator --config configs/dev-air/smoke.json --mode run
# prover benchmark
cd rust/vectis-prover && cargo run --release --bin sumproof_bench
```

## Status / roadmap
Research prototype. Natural extensions: per-tx validity (signature/nonce) inside the circuit, recursive aggregation across lanes, data-availability commitments for batch payloads.
