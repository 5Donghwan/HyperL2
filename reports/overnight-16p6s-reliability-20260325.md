# Overnight Reliability Report (2026-03-25)

## Context

Goal: validate whether the current public-RPC operating point can sustain `>= 200,000 receipt-TPS` reliably enough to hit at least `19/20` successful runs.

Baseline production branch before this work:

- branch: `codex/publish-main`
- public preset: `16 proofs / call + 6 senders`
- per call certified tx: `320,000`
- per burst total certified tx: `1,920,000`

All work in this report was done on:

- branch: `codex/16p6s-overnight-reliability`

## What Changed On This Branch

### 1. Measurement-path reliability fixes

Files:

- `/Users/5d0ng/dev/HyperL2/cmd/kaia-proofctl/main.go`
- `/Users/5d0ng/dev/HyperL2/scripts/run-kaia-16p6s-repeat.sh`

Changes:

- retried `HeaderByNumber` lookups instead of failing immediately on transient `not found`
- fell back to `receiptSeenAt` if the header remained temporarily unavailable
- added backwards-compatible TPS JSON aliases so the repeat harness can read either field name
- fixed the repeat script so `ALIGN_TO_NEXT_BLOCK=0` no longer crashes on an empty bash array

### 2. Strategy change: idle-gap broadcast

New CLI strategy on this branch:

- `--broadcast-after-idle <duration>`

Intent:

- do not broadcast immediately after a new block
- instead, wait until the chain has been idle for a configured gap (`6s` in testing)
- broadcast during the lull so the burst lands on the next tx-bearing block, and if it spills, the spill is more likely to land in the immediately following `+1s` block instead of a `+10s` block

### 3. Benchmark script generalization

`/Users/5d0ng/dev/HyperL2/scripts/run-kaia-16p6s-repeat.sh` now supports:

- `HYPERL2_LANE_COUNT`
- `HYPERL2_SENDER_COUNT`
- `ALIGN_TO_NEXT_BLOCK`
- `BROADCAST_AFTER_IDLE`

This made it possible to probe `16/6`, `12/6`, and transport/strategy variants using the same harness.

## Experiments Performed

### A. Baseline `16/6` on public RPC with simple alignment

Result:

- could not plausibly reach `19/20`
- failures were dominated by two patterns:
  - the burst packed as `4+2` or `5+1`
  - the last block's receipt visibility lagged by `~10s` or worse

Representative failed cases:

- repeat 1 (partial overnight baseline): `125,860 TPS`
- repeat 3 (partial overnight baseline): `98,481 TPS`
- repeat 4 (partial overnight baseline): `74,780 TPS`

### B. `16/6 + idle-gap(6s)` probe

Representative success probe:

- run dir: `/Users/5d0ng/dev/HyperL2/build/benchmarks/16p6s-20260325-014809`
- result: `342,307 TPS`
- packing: `5+1`
- important detail: the second block followed after `1s`, so receipt visibility stayed low

### C. `16/6 + idle-gap(6s) + wss://` probe

Result:

- run dir: `/Users/5d0ng/dev/HyperL2/build/benchmarks/16p6s-20260325-015316`
- result: `131,606 TPS`
- failure mode persisted
- last two txs were in the second block and both had `~10s` receipt visibility delay

Interpretation:

- switching from `https://` to `wss://` on the same public zkrypto endpoint did **not** remove the receipt-visibility outlier behavior

### D. `12/6 + idle-gap(6s)` probe

Result:

- run dir: `/Users/5d0ng/dev/HyperL2/build/benchmarks/12p6s-20260325-015622`
- result: `97,999 TPS`
- packing: `5+1`
- last tx receipt visibility delay: `10,060 ms`

Interpretation:

- reducing gas per call helped packing, but did **not** eliminate the last-block receipt lag problem

## Main Candidate Run: `16/6 + idle-gap(6s)`

To test whether the new strategy could approach the original `19/20` target, I launched a 20-run benchmark and stopped once `19/20` became mathematically impossible.

Run directory:

- `/Users/5d0ng/dev/HyperL2/build/benchmarks/16p6s-idle20-20260325-015851`

Observed before stopping:

- completed runs: `8`
- successful runs (`>= 200,000 receipt-TPS`): `6`
- failed runs: `2`
- therefore `19/20` was already impossible at run `8`

Per-run receipt TPS:

1. `384,461 TPS`
2. `344,827 TPS`
3. `325,976 TPS`
4. `384,615 TPS`
5. `71,394 TPS`  <- fail
6. `383,080 TPS`
7. `365,783 TPS`
8. `130,381 TPS` <- fail

Failure details:

### Repeat 5

- packing: `5+1`
- first block receipts were visible quickly
- last block receipt visibility delay: `22,056 ms`
- final receipt TPS: `71,394`

### Repeat 8

- packing: `5+1`
- first block receipts were visible quickly
- last block receipt visibility delay: `10,126 ms`
- final receipt TPS: `130,381`

## Root Cause Analysis

At this point the dominant instability is **not** proof generation and **not** on-chain verifier cost.

The evidence points to a public-RPC visibility problem layered on top of proposer packing:

1. proof generation is stable enough
2. on-chain verification gas is stable enough
3. bursts often pack as `4+2` or `5+1`
4. when the **last** included block is followed by a long RPC visibility lag, receipt-TPS collapses

This means the current public zkrypto RPC path still has a non-deterministic observation delay that we cannot reliably amortize away with the tested strategies.

## Bottom Line

### What this branch achieved

This branch is still useful.

It adds:

- better measurement robustness
- a reusable idle-gap broadcast strategy
- a generalized repeat harness for proofs/senders tuning
- better evidence about where the public path actually fails

### What this branch did *not* achieve

It did **not** achieve the user target of:

- `20` runs
- with `19` or more runs succeeding at `>= 200,000 receipt-TPS`

The strongest candidate tested here (`16/6 + idle-gap(6s)`) was already at `6/8` over target when the run was stopped.

## Morning Recommendation

If the question is "which branch should we continue from for reliability work?", keep:

- `codex/16p6s-overnight-reliability`

because it contains the reliability and strategy tooling we did not have before.

If the question is "which branch currently represents the clean public preset that we already accepted?", keep:

- `codex/publish-main`

because it remains the cleaner baseline and does not claim a reliability improvement it cannot yet guarantee.

## Next Best Step

The next meaningful lever is **RPC path control**, not more batch/sender tuning on the current public endpoint.

Specifically:

1. run the same `16/6 + idle-gap(6s)` strategy against a direct validator/private RPC path
2. keep the same success metric (`receipt-TPS >= 200k`)
3. compare whether the last-block receipt visibility outliers disappear

If they do, this branch's strategy is worth keeping and refining.
