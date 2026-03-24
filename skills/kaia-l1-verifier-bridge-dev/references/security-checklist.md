# Security Checklist

## Access Control

1. Gate upgrades behind timelock + multisig.
2. Require explicit role checks for pause/unpause.
3. Remove unused privileged functions.

## Proof Verification Path

1. Reject invalid proofs before any state mutation.
2. Prevent stale or out-of-order `batchIndex`.
3. Bind proof public inputs to expected chain/domain identifiers.

## Bridge Safety

1. Prevent replay of withdrawal messages.
2. Prevent double-finalization by consumed message mapping.
3. Use pull-based withdrawal payout when practical.

## Reentrancy and External Calls

1. Use checks-effects-interactions ordering.
2. Guard withdrawal finalization with reentrancy protections.
3. Keep external calls after message-consumed state update.

## Upgrade and Pause Operations

1. Ensure pause mode blocks all unsafe bridge state transitions.
2. Keep emergency pause bounded and auditable.
3. Document break-glass procedure and unpause criteria.

## Testing Expectations

1. Include invalid-proof regression tests.
2. Include replay and duplicate-withdrawal tests.
3. Include reorg-like ordering tests for root updates.

