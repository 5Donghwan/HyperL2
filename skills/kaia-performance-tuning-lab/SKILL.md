---
name: kaia-performance-tuning-lab
description: Plan, run, and interpret Kaia L1/L2 throughput and latency experiments for high-TPS targets. Use when creating benchmark plans, identifying bottlenecks, defining experiment matrices, tuning sequencer/prover/DA parameters, or reporting evidence for TPS claims such as 200,000 TPS.
---

# Kaia Performance Tuning Lab

## Overview

Turn performance goals into reproducible experiment plans and defensible tuning decisions. Prioritize measurable bottlenecks over intuition.

## Workflow

1. Define benchmark target:
- Set target TPS, p95 latency, p99 latency, and max finality lag.
- Set workload mix and transaction byte profile.

2. Build experiment matrix:
- Run `scripts/experiment_matrix.py` to generate parameter combinations.
- Keep one baseline row as control.

3. Execute staged loads:
- Run warmup, steady-state, and stress phases.
- Collect sequencer, prover, DA, and L1 metrics.

4. Classify bottleneck:
- Execution-limited, proving-limited, DA-limited, or L1-verification-limited.
- Use [tuning-playbook.md](references/tuning-playbook.md) for targeted actions.

5. Propose next iteration:
- Change one dominant factor at a time.
- Re-run the same matrix row for A/B comparability.

6. Produce evidence summary:
- Report whether TPS claim is sustained and under which assumptions.
- Explicitly state unresolved bottlenecks.

## Required Output Format

Always output in this order:

1. `Benchmark Scope`
2. `Experiment Matrix`
3. `Results Summary`
4. `Bottleneck Classification`
5. `Tuning Actions`
6. `Go or No-Go Decision`

## Experiment Matrix Script

Run:

```bash
python3 scripts/experiment_matrix.py \
  --target-tps 200000 \
  --sequencer-counts 1,2,4 \
  --batch-intervals 1,2,4 \
  --prover-workers 32,64,128
```

Use resulting rows as benchmark tickets and keep run metadata per row.

## References

- [benchmark-plan.md](references/benchmark-plan.md)
- [tuning-playbook.md](references/tuning-playbook.md)
