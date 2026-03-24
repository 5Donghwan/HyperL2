# Architecture Blueprint

## Components

1. Client / Wallet
- Sign transactions.
- Send transactions to sequencer gateway.

2. Sequencer Gateway
- Accept transactions, apply rate limit, and assign ordering.
- Forward ordered transactions to L2 execution lanes.

3. L2 Execution Lanes
- Execute transactions and build candidate state roots.
- Emit lane-local batch artifacts (`pre_state_root`, `post_state_root`, tx commitment).

4. Batcher + Data Publisher
- Bundle lane artifacts into proof jobs.
- Publish compressed transaction/data commitments to the selected DA layer.

5. Prover Farm
- Generate validity proofs for each batch window.
- Scale horizontally with multiple prover lanes.

6. Proof Aggregator
- Aggregate many batch proofs into fewer proofs for L1 verification cost control.
- Emit final aggregated proof payload for L1.

7. L1 Verifier Contract (Kaia)
- Verify proof.
- Accept new canonical state root only when proof passes.
- Reject invalid proof and keep previous canonical root unchanged.

8. L1 Bridge / Settlement Contracts
- Lock/unlock assets and message commitments.
- Finalize withdrawals only after proven state root is accepted.

## Responsibility Split

L1 responsibilities:
- Verify proof.
- Update canonical state root.
- Settle bridge accounting.
- Enforce upgrade and governance controls.

L2 responsibilities:
- Execute transactions.
- Sequence and batch transactions.
- Generate proofs.
- Handle mempool policy and fee market.

## Data Flow

1. Users submit transactions to sequencer.
2. L2 lanes execute and produce batch commitments.
3. Batcher publishes commitments to DA.
4. Prover farm generates validity proofs.
5. Aggregator compresses proofs.
6. Aggregated proof is submitted to L1 verifier.
7. If verified, L1 updates canonical root and unlocks bridge finalization.

## Contract Surface (Minimum)

1. `RollupCore`
- Store latest accepted state root.
- Register batch metadata hash.

2. `VerifierAdapter`
- Bind to proving system verifier.
- Expose `verifyAndUpdate(...)`.

3. `BridgeInbox` / `BridgeOutbox`
- Manage deposits and withdrawal proofs.

4. `TimelockGovernance`
- Control upgrades with delay and emergency pause.

## Horizontal Scaling Patterns

1. Multi-lane execution:
- Run multiple L2 execution lanes in parallel.
- Merge lane commitments via deterministic aggregation.

2. Prover parallelization:
- Assign one proof worker per lane-window.
- Use queue-based scheduling to avoid prover starvation.

3. Proof aggregation:
- Trade extra off-chain compute for lower L1 verify gas per transaction.

