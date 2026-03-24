---
name: kaia-l2-sequencer-prover-dev
description: Design and implement Kaia L2 sequencing, batching, proof generation, proof aggregation, and L1 submission workflows for high-throughput rollups. Use when building sequencer logic, mempool policy, batch production, prover orchestration, submission workers, or failure-recovery behavior between L2 and Kaia L1.
---

# Kaia L2 Sequencer Prover Dev

## Overview

Build the L2 execution and proving pipeline that drives throughput while keeping L1 limited to proof verification and state finalization. Emphasize determinism, idempotency, and recovery safety.

## Workflow

1. Define pipeline stages:
- Intake and ordering.
- Execution and batch assembly.
- Proof job creation.
- Proof submission and confirmation.

2. Enforce deterministic sequencing:
- Define canonical ordering keys.
- Define duplicate transaction handling.
- Define deterministic timeout and drop policies.

3. Plan batch policy:
- Use `scripts/batch_planner.py` to estimate tx-per-batch and DA footprint.
- Keep batch payload within DA and proving constraints.

4. Design prover orchestration:
- Use queue-based scheduling with explicit retry limits.
- Keep proof generation idempotent and replay-safe.

5. Design submission workers:
- Separate submit, confirm, and reconcile loops.
- Handle reorgs and uncertain receipts explicitly.

6. Validate failure handling:
- Apply [fault-handling.md](references/fault-handling.md).
- Confirm runbook actions for sequencer halt, prover lag, and L1 congestion.

## Required Output Format

Always output in this order:

1. `Pipeline State Machine`
2. `Batching and Proving Policy`
3. `Submission and Reconciliation Logic`
4. `Failure Modes and Recovery`
5. `Metrics and Alerts`
6. `Implementation Tasks`

## Guardrails

Do not violate:
- Do not finalize state without proof acceptance on L1.
- Do not use non-deterministic ordering logic in sequencer consensus path.
- Do not rely on at-least-once submission without idempotent dedupe.

## Batch Planner Script

Run:

```bash
python3 scripts/batch_planner.py \
  --target-tps 200000 \
  --batch-interval-sec 2 \
  --avg-tx-bytes 180 \
  --compression-ratio 0.30 \
  --max-batch-mb 64
```

Use `fits_max_batch` and `required_da_mb_per_sec` to validate batch policy.

## References

- [pipeline-state-machine.md](references/pipeline-state-machine.md)
- [fault-handling.md](references/fault-handling.md)
