---
name: kaia-l2-rollup-architect
description: Design and stress-test Kaia high-throughput rollup architectures where L1 only verifies proofs and finalizes state while L2 executes transactions, batches data, and generates proofs. Use when asked to target very high TPS (including 200,000 TPS), define L1/L2 responsibility split, size sequencer/prover capacity, plan bridge and finality flow, or produce phased implementation and risk-mitigation plans for Kaia-based L2 systems.
---

# Kaia L2 Rollup Architect

## Overview

Define an execution-heavy L2 plus verification-only L1 architecture for Kaia and validate whether the design can hit target TPS with realistic proving and L1 gas constraints. Produce concrete outputs: architecture diagram text, capacity numbers, contract responsibilities, rollout phases, and risk controls.

## Workflow

1. Capture target envelope:
- Fix target TPS, p95 latency, max finality lag, and expected average tx bytes.
- Fix trust assumptions (single sequencer, committee, or decentralized sequencer).

2. Choose architecture mode:
- Default to validity rollup (zk rollup) when L1 should only verify proofs.
- Keep L1 execution minimal: verification, state root update, bridge accounting, challenge windows only if hybrid design is used.

3. Build component map:
- Use [architecture-blueprint.md](references/architecture-blueprint.md) to map sequencer, execution engine, batcher, prover, proof aggregator, and L1 verifier contracts.
- Declare which components can scale horizontally.

4. Run capacity sizing:
- Run `scripts/capacity_model.py` with target assumptions.
- Reject designs where required proof submissions exceed L1 verification capacity.

5. Check non-functional gates:
- Apply [slo-and-risk-checklist.md](references/slo-and-risk-checklist.md).
- Confirm reorg handling, fraud/invalid-proof fallback policy, prover outage recovery, and bridge limits.

6. Produce implementation plan:
- Phase 0: single-sequencer testnet and synthetic load.
- Phase 1: prover parallelization and proof aggregation.
- Phase 2: decentralized sequencing and production hardening.
- Phase 3: governance, upgrade controls, and audit closure.

## Required Output Format

Use this exact section order when answering:

1. `Target Envelope`
2. `L1 vs L2 Responsibility Matrix`
3. `Architecture Draft`
4. `Capacity Model Results`
5. `Risk and Mitigation`
6. `Phased Delivery Plan`

Include at least one numeric bottleneck in `Capacity Model Results`.

## L1 vs L2 Guardrails

Keep L1-only verification model strict:
- Never execute user transactions on L1.
- Keep L1 contracts deterministic and minimal.
- Fail closed if proof verification fails.

Use L2 for throughput:
- Execute all transactions on L2.
- Batch state transitions and publish commitments.
- Generate validity proofs per batch or proof-aggregation window.

## Capacity Model Script

Run:

```bash
python3 scripts/capacity_model.py \
  --target-tps 200000 \
  --lane-execution-tps 25000 \
  --batch-interval-sec 2 \
  --proof-time-sec 30 \
  --verify-gas-per-proof 600000 \
  --l1-gas-limit 120000000 \
  --l1-block-time-sec 1 \
  --l1-gas-share 0.35 \
  --avg-tx-bytes 180 \
  --compression-ratio 0.30
```

Interpretation:
- `execution_lanes_needed` tells how many parallel L2 lanes (or rollup instances) are required.
- `prover_lanes_needed` estimates concurrent proving workers required to avoid backlog.
- `l1_verification_headroom` below `1.0` means L1 can sustain required proof verification throughput.
- `da_bandwidth_mb_per_sec` gives an order-of-magnitude data-availability footprint.

## References

Use these files directly:
- [architecture-blueprint.md](references/architecture-blueprint.md): component responsibilities, contract surface, and data flow.
- [slo-and-risk-checklist.md](references/slo-and-risk-checklist.md): production gates, incident policy, and rollout checklist.

## Constraints

Do not claim 200,000 TPS is feasible unless all three are explicitly satisfied:
- L2 execution capacity (`target_tps <= lane_execution_tps * execution_lanes`).
- Proving throughput (`required_proofs_per_sec <= prover_concurrency / proof_time_sec`).
- L1 verification capacity (`required_proofs_per_sec <= l1_max_proofs_per_sec`).
