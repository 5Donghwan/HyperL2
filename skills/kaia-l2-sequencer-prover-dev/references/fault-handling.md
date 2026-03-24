# Fault Handling

## Sequencer Halt

1. Freeze new intake.
2. Preserve last committed ordering cursor.
3. Resume with deterministic continuation.

## Prover Backlog Spike

1. Alert when proof queue wait exceeds SLO.
2. Auto-scale proof workers within cap.
3. Increase batch interval only through controlled policy switch.

## L1 Congestion

1. Separate submission and confirmation workers.
2. Raise gas strategy adaptively with cap.
3. Hold finalization until confirmed inclusion.

## Reorg or Receipt Uncertainty

1. Require N confirmations before irreversible marking.
2. Reconcile by batch id and proof hash.
3. Re-submit only when dedupe key indicates non-final state.

## DA Delay

1. Block proof promotion if DA publish is not confirmed.
2. Fail safe and retain batch in replayable queue.

## Incident Response Data

1. Affected batch range.
2. Current pipeline state counts.
3. Last known canonical root.
4. Actions taken and rollback boundary.

