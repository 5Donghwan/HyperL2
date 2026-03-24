# HyperL2 Sum-Preserving ccGroth16 Flow

This crate adds a HyperL2-specific proving path on top of the VECTIS `CCGroth16` implementation.

## Goal

Each proof certifies one `20,000 tx` batch transition over a fixed slot map:

- `slot i`: fixed account address `A_i`
- `D1`: Pedersen vector commitment to the pre-state values
- `D2`: Pedersen vector commitment to the post-state values
- relation: `sum(open(D1)[0..N-1]) == sum(open(D2)[0..N-1])`

The proof does not validate signatures, nonces, ownership, or per-tx correctness.

The raw transfers are processed off-circuit into a state transition:

```text
pre-state + 20,000 transfers = post-state
```

## Witness Layout

For batch size `N`, each commitment has width `W = N + 1`.

- `X = [pre_0, ..., pre_(N-1), r_pre]`
- `Y = [post_0, ..., post_(N-1), r_post]`
- `A = tau * X + tau^2 * Y`

The committed witness layout is:

```text
[A_0 .. A_(W-1), X_0 .. X_(W-1), Y_0 .. Y_(W-1)]
```

The proof-dependent commitment only commits the trailing `X || Y` portion. `A` is encoded as the front aggregation segment required by VECTIS.

## Prover Flow

1. Start from a fixed slot map `slot i <-> account A_i`.
2. Apply the `20,000` raw transfers off-circuit to build `pre-state` and `post-state`.
3. Build `X` and `Y`.
4. Compute `d0 = CCGroth16::commit(ck, X || Y)`.
5. Compute `D1 = Com(X)` and `D2 = Com(Y)`.
6. Derive `tau = H(domain, lane_id, batch_id, tx_count, D1, D2, d0)`.
   - The hash input is EVM-friendly and fixed-width:
   - `domain || lane_id(2B) || batch_id(8B) || tx_count(4B) || D1.x || D1.y || D2.x || D2.y || d0.x || d0.y`
   - Each field/coordinate is encoded as big-endian `bytes32`.
7. Compute `A = tau * X + tau^2 * Y`.
8. Prove the circuit with public input `[tau]`.

## Verifier Flow

1. Recompute `tau` from metadata, `D1`, `D2`, and the proof's original `d` field.
2. Compute `Agg = tau * D1 + tau^2 * D2`.
3. Replace `proof.d` with `proof.d + Agg`.
4. Run `ccGroth16.verify(vk, [tau], proof')`.

## On-Chain Verifier

- `contracts/CCGroth16BatchVerifier.sol` reconstructs `tau`, `Agg`, and the prepared proof on-chain.
- Verification uses the BN254 precompiles with the pairing equation:

```text
e(proof.a, proof.b)
* e(-(proof.d + Agg + prepared_inputs), gamma_g2)
* e(-proof.c, delta_g2)
* e(-alpha_g1, beta_g2)
= 1
```

- `contracts/SumPreservingBatchVerifier.sol` verifies `10` batch artifacts and advances `laneHead[laneId]`.
- `rust/vectis-prover/src/bin/export_sumproof_fixture.rs` exports a Solidity-ready verifying key and sample artifact.

## Performance Envelope

- Current machine measured `~0.474s` to prove one `20k` batch.
- Off-chain verification is `~0.002s` per proof.
- On-chain work per proof is:
  - `3` G1 scalar multiplications
  - `3` G1 additions
  - `1` pairing product with `4` pairs
- This is consistent with a private-net target of `10 proofs` in one L1 transaction, subject to sufficient block gas.
- A practical gas envelope is roughly:
  - `~200k` gas per proof for the cryptographic precompiles
  - `~2.1M - 2.4M` gas for `10` proofs including lane-head updates
- Under a private PoA deployment with a single validator and a high block gas limit, this keeps the verifier path compatible with a `sub-1s` execution target.

## 200k Certification Model

- `1 proof` certifies `20,000 tx`
- `10 proofs` certify `200,000 tx`
- L1 verifier contract stores `laneHead[lane_id]`
- For each proof:
  - require `laneHead[lane_id] == D1`
  - verify the batch proof
  - set `laneHead[lane_id] = D2`

This is a `10 batch transition proof` model, not a `200k individual tx validity proof` model.
