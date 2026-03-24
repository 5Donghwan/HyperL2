---
name: kaia-l1-verifier-bridge-dev
description: Implement and review Kaia L1 contracts for validity-proof verification, canonical state-root updates, and bridge settlement in L2 rollup architectures. Use when writing Solidity code for verifier/bridge contracts, defining L1 invariants, designing upgrade and pause controls, creating contract test plans, or performing security-focused reviews of Kaia L1 rollup components.
---

# Kaia L1 Verifier and Bridge Dev

## Overview

Implement minimal and safe L1 contracts that verify proofs, update canonical L2 state roots, and settle cross-domain bridge actions. Keep all user transaction execution off L1.

## Workflow

1. Define L1 boundaries:
- Accept only proof verification, state-root updates, and bridge settlement.
- Reject any requirement that adds user transaction execution to L1.

2. Draft contract surface:
- Use [contracts-blueprint.md](references/contracts-blueprint.md) to define `RollupCore`, `VerifierAdapter`, `BridgeInbox`, and `BridgeOutbox`.
- Keep storage compact and upgrade-safe.

3. Encode invariants:
- Enforce monotonic `batchIndex`.
- Enforce proof validity before state-root update.
- Enforce single-use withdrawal messages.

4. Design controls:
- Add timelock + multisig upgrade gates.
- Add emergency pause scope with clear resume policy.

5. Produce tests:
- Add positive-path tests for proof verify and withdrawal finalization.
- Add negative-path tests for invalid proof, replayed withdrawal, and unauthorized upgrades.

6. Run security pass:
- Apply [security-checklist.md](references/security-checklist.md).
- Run `scripts/bridge_risk_model.py` for operational withdrawal limits.

## Required Output Format

Always output in this order:

1. `Contract Surface`
2. `State and Invariants`
3. `Access Control and Upgrade Policy`
4. `Security Findings and Mitigations`
5. `Test Plan`
6. `Open Risks`

## Mandatory Invariants

Require all:
- `batchIndex` increases by exactly 1 for accepted batches.
- `stateRoot` updates only after verifier success.
- Withdrawal proofs are domain-separated and non-replayable.
- Bridge pause does not permit bypass through alternative entry points.

## Operational Risk Script

Run:

```bash
python3 scripts/bridge_risk_model.py \
  --tvl-usd 500000000 \
  --daily-cap-pct 0.08 \
  --hourly-spike-cap-pct 0.015
```

Use output to set initial withdrawal limits and incident thresholds.

## References

- [contracts-blueprint.md](references/contracts-blueprint.md)
- [security-checklist.md](references/security-checklist.md)
