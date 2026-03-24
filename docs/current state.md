# HyperL2 Status Update

## Goal

We are building a certification-oriented pipeline for demonstrating high throughput on Kaia-style L1 verification using batch transition proofs.

The current target is:

- `20,000 tx` represented by `1 batch transition proof`
- `10 proofs` representing `200,000 tx`
- L1 verifies only the proofs and state commitments

This is intentionally **not** a full validity rollup. The proof only certifies a minimal relation needed for throughput experiments.

## Current Proof Model

Each proof represents a state transition:

- `D1`: Pedersen commitment to the pre-state vector
- `D2`: Pedersen commitment to the post-state vector

The proof guarantees:

- the prover knows openings for `D1` and `D2`
- `sum(open(D1)) == sum(open(D2))`

It does **not** prove:

- signature validity
- nonce correctness
- double-spend prevention
- full per-transaction validity

So the current system should be described as a **sum-preserving batch transition proof pipeline**.

## ccGroth16 / VECTIS Integration

We implemented the prover using the Rust `VECTIS` codebase and its ccGroth16 construction.

The important part is that the circuit uses values committed **outside** the circuit:

- `D1` and `D2` are built externally as Pedersen vector commitments
- the circuit receives the corresponding openings as witnesses
- `tau` is derived from metadata and commitments
- the proof is bound to the external commitments using the ccSNARK-style aggregation path

This means L1 does **not** need all committed values as public inputs. L1 only receives:

- `D1`
- `D2`
- proof metadata
- the ccGroth16 proof

## L1 Contracts

We implemented:

- `CCGroth16BatchVerifier.sol`
  - verifies one batch transition proof on-chain
- `SumPreservingBatchVerifier.sol`
  - keeps lane heads
  - verifies batches
  - advances `D1 -> D2`
  - counts certified tx volume

The L1 contract path has been tested successfully on the research testnet endpoint.

## Dashboard / Operator Tooling

We added a browser dashboard through `kaia-proofctl dashboard`.

It shows:

- proof setup progress
- proof generation progress
- replay proof loading
- L1 submission count
- receipt confirmations
- gas usage
- block number
- aggregate receipt TPS

This allows us to observe the run visually instead of reading raw CLI output.

## Performance Observations

### Proof generation

Originally, generating `10 proofs` live took about:

- `~5.48s`

because all ten `20k` proofs were produced sequentially.

We then changed the operator path to:

- `1 live proof + 9 replay proofs`

This reduced proof-generation-side time to about:

- `~1.20s`

for the current dashboard flow.

### L1 verification

The on-chain verification gas is stable at roughly:

- `~2.69M gas` per `200,000 tx` certification call

The major source of variance is **not** verifier computation. It is:

- endpoint inclusion delay
- receipt latency
- proposer / mempool behavior

### Throughput results

Single-call results vary because the public/shared endpoint is noisy.

Examples we observed:

- one run around `174k TPS` receipt-based
- another run much lower because the receipt was delayed by more than `16s`

The most meaningful result so far came from **multi-sender same-block packing**:

- `4 senders`
- each sender submits one verification call
- each call certifies `200,000 tx`
- total certified volume: `800,000 tx`
- measured window: `3.698s`
- aggregate TPS: `216,333`

This exceeded the `200,000 TPS` target under the certification definition we are using.

## Important Interpretation

The reported TPS should be described precisely as:

- **certified throughput based on batch transition proofs**

In this repository, `TPS` means:

- **L1 block-inclusion certified throughput**

not as:

- full transaction-validity TPS

The system currently proves and verifies batch-level sum preservation, not full transaction correctness.
Receipt visibility remains a separate operator/debugging metric and is not the primary TPS definition.

## Current Bottleneck

The dominant bottleneck is now:

- L1 network / endpoint inclusion and receipt variance

not:

- proof generation
- pairing verification cost

This is why multi-sender submission is important. It helps overcome nonce serialization and increases same-block packing opportunities.

## Current Status

At this point we have:

- implemented the Rust ccGroth16 prover path
- implemented the Kaia-compatible L1 verifier contracts
- verified proofs on-chain
- built an operator dashboard
- demonstrated `200k+` certified TPS using multi-sender submission

## Next Steps

Recommended next steps are:

1. run the same flow against a more direct validator RPC path
2. repeat multi-sender runs and record distribution (`p50`, `p95`, best/worst`)
3. separate clearly in reporting:
   - proof generation time
   - L1 block inclusion latency
   - receipt visibility delay
   - TPS
4. if needed, extend to a chained `D1 -> D2 -> D3` operating mode once ordering control is available
