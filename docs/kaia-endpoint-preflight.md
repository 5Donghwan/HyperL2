# Kaia Endpoint Preflight

This document covers everything that should be finished before the project is attached to a real Kaia endpoint.

## Scope

The goal is to arrive at the endpoint with:

- compiled verifier contracts
- a verifying key payload
- a `10`-proof certification bundle
- proof-path benchmark results
- a fixed transcript format shared by Rust and Solidity

## What is already prepared

1. `contracts/CCGroth16BatchVerifier.sol`
   - reconstructs `tau`
   - reconstructs `Agg = tau * D1 + tau^2 * D2`
   - reconstructs prepared public inputs
   - verifies the ccGroth16 pairing equation with BN254 precompiles

2. `contracts/SumPreservingBatchVerifier.sol`
   - stores `laneHead[laneId]`
   - validates `prevStateCommitment == laneHead[laneId]`
   - verifies `10` batch artifacts in one call

3. `rust/vectis-prover/src/hyperl2/challenge.rs`
   - uses the same fixed-width transcript format the contract uses

4. `rust/vectis-prover/src/bin/export_sumproof_bundle.rs`
   - exports a Solidity-ready verifying key
   - exports lane heads
   - exports a ready-to-submit `verifyTenBatches` input bundle

5. `scripts/prepare-kaia-preflight.sh`
   - runs the local checks
   - compiles contracts
   - exports the fixture and the `10`-proof bundle
   - writes a hash manifest

## Local preflight command

```bash
scripts/prepare-kaia-preflight.sh
```

Default output:

```text
build/kaia-preflight/
```

## Expected output files

- `build/kaia-preflight/contracts/*.abi`
- `build/kaia-preflight/contracts/*.bin`
- `build/kaia-preflight/single-fixture.json`
- `build/kaia-preflight/certification-bundle.json`
- `build/kaia-preflight/bench.txt`
- `build/kaia-preflight/test-sum-proof.txt`
- `build/kaia-preflight/test-metadata-tamper.txt`
- `build/kaia-preflight/test-state-builder.txt`
- `build/kaia-preflight/SHA256SUMS`

## What remains only after endpoint attachment

1. deploy `CCGroth16BatchVerifier`
2. initialize the verifying key
3. deploy `SumPreservingBatchVerifier`
4. initialize `laneHead` for each lane
5. send `verifyTenBatches`
6. measure gas, receipt latency, and finalized TPS on the real endpoint

## Endpoint values already observed on 2026-03-10

Using `https://testnet.zkrypton.zkrypto.com`, the current client discovered:

- `chain_id = 113230`
- `network_id = 411691`
- `gas_price = 27_500_000_000 wei`
- `base_fee = 25_000_000_000 wei`

These values were obtained from the live endpoint on 2026-03-10 and may change later.

## First commands to run after endpoint attachment

```bash
./kaia-proofctl generate-key
./kaia-proofctl account-status --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key>
./kaia-proofctl discover --rpc-url https://testnet.zkrypton.zkrypto.com
./kaia-proofctl deploy-verifier --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key>
./kaia-proofctl init-vk --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key> --verifier-address <addr>
./kaia-proofctl deploy-certifier --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key> --verifier-address <addr>
./kaia-proofctl init-lane-heads --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key> --certifier-address <addr>
./kaia-proofctl estimate-bundle --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key> --certifier-address <addr>
./kaia-proofctl submit-bundle --rpc-url https://testnet.zkrypton.zkrypto.com --private-key <key> --certifier-address <addr>
```

## Success criteria before endpoint attachment

- Rust proof round-trip passes
- metadata tampering fails verification
- state-transition builder preserves total sum
- Solidity contracts compile cleanly
- `20k -> 1 proof` benchmark is regenerated locally
- a deterministic `10`-proof bundle is exported for later endpoint submission

## Deployment account requirements

Before sending deployment transactions, the deployment account must have:

- a locally generated secp256k1 private key
- the derived sender address
- enough native test token balance to pay deployment and verify-call gas
- permission to deploy contracts on the target endpoint, if the lab network restricts deployment
