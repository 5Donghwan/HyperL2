# Contracts Blueprint

## Contract Set

1. `RollupCore`
- Hold latest accepted `stateRoot`.
- Hold monotonic `batchIndex`.
- Accept root update only from verifier path.

2. `VerifierAdapter`
- Wrap proof-system verifier call.
- Expose `verifyAndUpdate(bytes proof, bytes publicInputs, bytes32 newRoot, uint256 batchIndex)`.

3. `BridgeInbox`
- Accept deposits from L1 to L2.
- Emit deposit events with canonical nonce.

4. `BridgeOutbox`
- Finalize L2-to-L1 withdrawals after proven root inclusion.
- Enforce message replay protection.

## Minimal Interfaces

```solidity
interface IRollupCore {
    function latestStateRoot() external view returns (bytes32);
    function latestBatchIndex() external view returns (uint256);
    function updateStateRoot(bytes32 newRoot, uint256 batchIndex) external;
}

interface IVerifierAdapter {
    function verify(bytes calldata proof, bytes calldata publicInputs) external view returns (bool);
}
```

## Storage and Upgrade Notes

1. Pin storage slots for upgradeable proxies.
2. Keep verifier address mutable only through timelock governance.
3. Keep pause flags scoped by subsystem (`bridgePaused`, `rootUpdatePaused`).

## Required Events

1. `StateRootUpdated(uint256 batchIndex, bytes32 oldRoot, bytes32 newRoot)`
2. `WithdrawalFinalized(bytes32 messageId, address to, uint256 amount)`
3. `BridgePaused(address actor, string scope)`
4. `BridgeUnpaused(address actor, string scope)`

