# Pipeline State Machine

## Batch Lifecycle

1. `QUEUED`
- Transactions accepted and ordered.

2. `EXECUTED`
- Batch executed in L2 engine.
- Candidate post-state root computed.

3. `DA_POSTED`
- Commitment published to DA layer.

4. `PROVING`
- Proof worker assigned.

5. `PROVED`
- Proof artifact generated and validated.

6. `L1_SUBMITTED`
- Proof submission sent to L1.

7. `L1_CONFIRMED`
- L1 receipt confirmed and root accepted.

8. `FINALIZED`
- Batch considered finalized for bridge purposes.

## Transition Rules

1. Allow only forward transitions.
2. Permit retry only within same stage (`PROVING` retry, `L1_SUBMITTED` resubmit with dedupe key).
3. Mark terminal failure only after retry budget is exhausted.

## Idempotency Keys

1. Batch key: `rollupId:batchIndex`.
2. Submission key: `rollupId:batchIndex:proofHash`.
3. Withdrawal key: `messageId`.

## Metrics Per State

1. Queue depth by state.
2. Time spent per state.
3. Retry count per state.
4. Failure reason histogram.

