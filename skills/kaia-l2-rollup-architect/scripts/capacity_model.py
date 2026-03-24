#!/usr/bin/env python3
"""
Capacity model for Kaia L2 rollup planning.

Estimate whether a target TPS is feasible when:
- L2 executes transactions and generates proofs.
- L1 only verifies proofs and finalizes state roots.
"""

import argparse
import json
import math
from dataclasses import asdict, dataclass


@dataclass
class CapacityResult:
    target_tps: float
    execution_lanes_needed: int
    txs_per_batch_per_lane: float
    required_proofs_per_sec: float
    prover_lanes_needed: int
    l1_max_proofs_per_sec: float
    l1_verification_headroom: float
    da_bandwidth_mb_per_sec: float
    estimated_finality_lag_sec: float
    feasible: bool


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Estimate execution/proving/L1-verification capacity for a Kaia L2 rollup.",
    )
    parser.add_argument("--target-tps", type=float, required=True, help="Target transactions per second.")
    parser.add_argument(
        "--lane-execution-tps",
        type=float,
        required=True,
        help="Execution TPS per single L2 lane under production assumptions.",
    )
    parser.add_argument(
        "--batch-interval-sec",
        type=float,
        default=2.0,
        help="Seconds between proof batches per lane.",
    )
    parser.add_argument(
        "--proof-time-sec",
        type=float,
        default=30.0,
        help="Average time to produce one proof for one lane batch.",
    )
    parser.add_argument(
        "--verify-gas-per-proof",
        type=float,
        default=600000.0,
        help="L1 gas consumed to verify one proof submission.",
    )
    parser.add_argument(
        "--l1-gas-limit",
        type=float,
        default=120000000.0,
        help="L1 block gas limit used for planning.",
    )
    parser.add_argument(
        "--l1-block-time-sec",
        type=float,
        default=1.0,
        help="L1 average block time in seconds.",
    )
    parser.add_argument(
        "--l1-gas-share",
        type=float,
        default=0.35,
        help="Fraction of L1 gas budget reserved for rollup verification (0 to 1).",
    )
    parser.add_argument(
        "--avg-tx-bytes",
        type=float,
        default=180.0,
        help="Average bytes per L2 transaction before compression.",
    )
    parser.add_argument(
        "--compression-ratio",
        type=float,
        default=0.30,
        help="DA compressed_size / raw_size ratio (0 to 1).",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Emit JSON instead of table text.",
    )
    return parser.parse_args()


def positive(name: str, value: float) -> None:
    if value <= 0:
        raise ValueError(f"{name} must be > 0, got {value}")


def zero_to_one(name: str, value: float) -> None:
    if value < 0 or value > 1:
        raise ValueError(f"{name} must be in [0, 1], got {value}")


def model(args: argparse.Namespace) -> CapacityResult:
    positive("target_tps", args.target_tps)
    positive("lane_execution_tps", args.lane_execution_tps)
    positive("batch_interval_sec", args.batch_interval_sec)
    positive("proof_time_sec", args.proof_time_sec)
    positive("verify_gas_per_proof", args.verify_gas_per_proof)
    positive("l1_gas_limit", args.l1_gas_limit)
    positive("l1_block_time_sec", args.l1_block_time_sec)
    positive("avg_tx_bytes", args.avg_tx_bytes)
    zero_to_one("l1_gas_share", args.l1_gas_share)
    zero_to_one("compression_ratio", args.compression_ratio)

    execution_lanes_needed = math.ceil(args.target_tps / args.lane_execution_tps)
    txs_per_batch_per_lane = args.lane_execution_tps * args.batch_interval_sec
    required_proofs_per_sec = execution_lanes_needed / args.batch_interval_sec

    # Minimum concurrent proof workers to keep up with arrival rate.
    prover_lanes_needed = math.ceil(required_proofs_per_sec * args.proof_time_sec)

    l1_gas_per_sec_available = (args.l1_gas_limit / args.l1_block_time_sec) * args.l1_gas_share
    l1_max_proofs_per_sec = l1_gas_per_sec_available / args.verify_gas_per_proof
    l1_verification_headroom = (
        l1_max_proofs_per_sec / required_proofs_per_sec if required_proofs_per_sec > 0 else float("inf")
    )

    da_bytes_per_sec = args.target_tps * args.avg_tx_bytes * args.compression_ratio
    da_bandwidth_mb_per_sec = da_bytes_per_sec / (1024 * 1024)

    # Rough optimistic lag from batching + proving + one L1 block.
    estimated_finality_lag_sec = args.batch_interval_sec + args.proof_time_sec + args.l1_block_time_sec

    feasible = l1_verification_headroom >= 1.0

    return CapacityResult(
        target_tps=args.target_tps,
        execution_lanes_needed=execution_lanes_needed,
        txs_per_batch_per_lane=txs_per_batch_per_lane,
        required_proofs_per_sec=required_proofs_per_sec,
        prover_lanes_needed=prover_lanes_needed,
        l1_max_proofs_per_sec=l1_max_proofs_per_sec,
        l1_verification_headroom=l1_verification_headroom,
        da_bandwidth_mb_per_sec=da_bandwidth_mb_per_sec,
        estimated_finality_lag_sec=estimated_finality_lag_sec,
        feasible=feasible,
    )


def print_table(result: CapacityResult) -> None:
    lines = [
        "Kaia L2 Rollup Capacity Model",
        f"target_tps                : {result.target_tps:,.0f}",
        f"execution_lanes_needed    : {result.execution_lanes_needed}",
        f"txs_per_batch_per_lane    : {result.txs_per_batch_per_lane:,.0f}",
        f"required_proofs_per_sec   : {result.required_proofs_per_sec:,.3f}",
        f"prover_lanes_needed       : {result.prover_lanes_needed}",
        f"l1_max_proofs_per_sec     : {result.l1_max_proofs_per_sec:,.3f}",
        f"l1_verification_headroom  : {result.l1_verification_headroom:,.3f}",
        f"da_bandwidth_mb_per_sec   : {result.da_bandwidth_mb_per_sec:,.3f}",
        f"estimated_finality_lag_sec: {result.estimated_finality_lag_sec:,.1f}",
        f"feasible                  : {result.feasible}",
    ]
    print("\n".join(lines))


def main() -> int:
    try:
        args = parse_args()
        result = model(args)
    except ValueError as err:
        print(f"Input error: {err}")
        return 2

    if args.json:
        print(json.dumps(asdict(result), indent=2))
    else:
        print_table(result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
