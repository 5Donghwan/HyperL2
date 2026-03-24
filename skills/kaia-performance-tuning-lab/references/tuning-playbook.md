# Tuning Playbook

## If Execution-Limited

1. Increase execution lanes.
2. Optimize state access hot paths.
3. Reduce per-transaction heavy opcodes/features in workload path.

## If Proving-Limited

1. Increase prover worker count.
2. Increase proof aggregation ratio.
3. Optimize batch interval for proof amortization.

## If DA-Limited

1. Improve compression ratio.
2. Reduce redundant payload in batch encoding.
3. Tune batch interval and payload target size.

## If L1-Verification-Limited

1. Increase aggregation to reduce proofs per second.
2. Reduce verify gas per proof via proof-system optimization.
3. Reserve larger L1 gas share for proof submissions.

## Iteration Rules

1. Change one dominant parameter per iteration.
2. Re-run baseline row daily to detect drift.
3. Stop when bottleneck shifts and retarget next limiter.

