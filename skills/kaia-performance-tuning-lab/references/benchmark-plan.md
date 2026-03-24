# Benchmark Plan

## Scope Definition

1. Specify TPS target.
2. Specify latency SLOs (`p95`, `p99`).
3. Specify finality lag target.
4. Specify workload mix and tx size profile.

## Run Phases

1. Warmup
- 5 to 10 minutes.
- Verify stable metrics and no backlog.

2. Steady-state
- 20 to 30 minutes at target load.
- Record throughput sustainability.

3. Stress burst
- 1.5x to 2x target TPS.
- Observe degradation mode and recovery time.

## Required Metrics

1. Sequencer: queue depth, inclusion latency.
2. Execution: block/build time, CPU saturation.
3. Prover: queue delay, proof latency, worker utilization.
4. DA: publish latency, throughput.
5. L1: proof submission success and confirmation lag.

## Reporting Rules

1. Report baseline and changed runs side by side.
2. Include only comparable runs with same workload profile.
3. Label every chart/table with commit hash and config id.

