# SLO and Risk Checklist

## Performance SLO

1. Throughput
- Target TPS: declare explicit value (for example, 200,000 TPS).
- Sustain test duration: at least 30 minutes at target load.

2. Latency
- L2 p95 inclusion latency target.
- End-to-end finality lag target (submit -> L1 proven root).

3. Finality
- Maximum tolerated proof backlog window.
- Recovery objective for prover outage.

## Reliability Gates

1. Sequencer failure mode
- Define failover RTO and RPO.
- Preserve deterministic ordering guarantees across failover.

2. Prover failure mode
- Queue depth alerts.
- Auto-scale triggers by proof wait time.

3. L1 congestion mode
- Backpressure policy when proof submission is delayed.
- Batch interval adaptation and temporary rate control.

## Security Gates

1. Contract safety
- Verifier and bridge contracts audited.
- Upgrade path protected by timelock + multisig.

2. Proof integrity
- Explicit invalid-proof rejection tests.
- Proven-state root mismatch alarms.

3. Cross-domain messaging
- Replay protection for withdrawal/finalization proofs.
- Message nonce and domain separation checks.

## Economic Controls

1. Sequencer and prover incentives
- Ensure fee model covers proving and DA costs at target TPS.

2. Bridge risk limits
- Daily withdrawal cap.
- Per-asset emergency circuit breaker.

## Test Matrix

1. Load tests
- 1x, 1.5x, 2x target TPS.

2. Adversarial tests
- Sequencer restart during heavy load.
- Prover lag spike.
- L1 gas spike scenario.

3. Data availability tests
- Delayed DA publication.
- Corrupted commitment rejection.

## Ship Criteria

Only ship when all are true:
1. Capacity model shows non-negative headroom on execution, proving, and L1 verification.
2. Security review and external audit findings are closed or accepted with explicit waivers.
3. Incident runbooks exist for sequencer outage, prover outage, and L1 congestion.

