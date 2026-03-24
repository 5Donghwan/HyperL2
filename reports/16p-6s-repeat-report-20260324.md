# HyperL2 16-Proof 6-Sender Repeat Report (2026-03-24)

## Configuration

- `16 proofs / call`
- `6 senders`
- `6 fresh certifiers` per run
- `1,920,000 tx` certified per burst
- public RPC: `https://testnet.zkrypton.zkrypto.com`

## Five Repeats

1. `238,746 TPS` (`window_ms = 8042`)
2. `836,601 TPS` (`window_ms = 2295`)
3. `237,476 TPS` (`window_ms = 8085`)
4. `509,689 TPS` (`window_ms = 3767`)
5. `1,072,625 TPS` (`window_ms = 1790`)

## Summary

- best: `1,072,625 TPS`
- median: `509,689 TPS`
- worst: `237,476 TPS`
- average: `579,027 TPS`
- over `200k TPS`: `5 / 5`

## Interpretation

This is the strongest repeated configuration observed so far on the public zkrypto endpoint.

Most importantly:

- every run exceeded `200,000 TPS`
- the worst run still stayed above `237k TPS`
- this means the target is now reproducible in this sample under the current public-RPC setup

This does not remove public-endpoint variability entirely, but it does show that with the right combination of:

- larger batch size
- enough parallel senders
- fresh certifiers

we can consistently remain above the `200k TPS` target in repeated trials.

## Raw Outputs

- runs JSONL: [/tmp/hyperl2-16p-6s-20260324-152704/runs.jsonl](/tmp/hyperl2-16p-6s-20260324-152704/runs.jsonl)
- summary JSON: [/tmp/hyperl2-16p-6s-20260324-152704/summary.json](/tmp/hyperl2-16p-6s-20260324-152704/summary.json)
