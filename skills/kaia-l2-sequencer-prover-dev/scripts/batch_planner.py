#!/usr/bin/env python3
"""
Batch planning helper for Kaia L2.
"""

import argparse
import json
from dataclasses import asdict, dataclass


@dataclass
class BatchPlan:
    target_tps: float
    batch_interval_sec: float
    tx_per_batch: float
    raw_batch_mb: float
    compressed_batch_mb: float
    required_da_mb_per_sec: float
    fits_max_batch: bool


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Estimate batch size and DA requirements.")
    parser.add_argument("--target-tps", type=float, required=True)
    parser.add_argument("--batch-interval-sec", type=float, required=True)
    parser.add_argument("--avg-tx-bytes", type=float, default=180.0)
    parser.add_argument("--compression-ratio", type=float, default=0.30)
    parser.add_argument("--max-batch-mb", type=float, default=64.0)
    parser.add_argument("--json", action="store_true")
    return parser.parse_args()


def validate_positive(name: str, value: float) -> None:
    if value <= 0:
        raise ValueError(f"{name} must be > 0, got {value}")


def validate_ratio(name: str, value: float) -> None:
    if value < 0 or value > 1:
        raise ValueError(f"{name} must be in [0,1], got {value}")


def model(
    target_tps: float,
    batch_interval_sec: float,
    avg_tx_bytes: float,
    compression_ratio: float,
    max_batch_mb: float,
) -> BatchPlan:
    validate_positive("target_tps", target_tps)
    validate_positive("batch_interval_sec", batch_interval_sec)
    validate_positive("avg_tx_bytes", avg_tx_bytes)
    validate_positive("max_batch_mb", max_batch_mb)
    validate_ratio("compression_ratio", compression_ratio)

    tx_per_batch = target_tps * batch_interval_sec
    raw_batch_bytes = tx_per_batch * avg_tx_bytes
    compressed_batch_bytes = raw_batch_bytes * compression_ratio

    raw_batch_mb = raw_batch_bytes / (1024 * 1024)
    compressed_batch_mb = compressed_batch_bytes / (1024 * 1024)
    required_da_mb_per_sec = compressed_batch_mb / batch_interval_sec
    fits_max_batch = compressed_batch_mb <= max_batch_mb

    return BatchPlan(
        target_tps=target_tps,
        batch_interval_sec=batch_interval_sec,
        tx_per_batch=tx_per_batch,
        raw_batch_mb=raw_batch_mb,
        compressed_batch_mb=compressed_batch_mb,
        required_da_mb_per_sec=required_da_mb_per_sec,
        fits_max_batch=fits_max_batch,
    )


def print_table(result: BatchPlan) -> None:
    print("L2 Batch Planner")
    print(f"target_tps            : {result.target_tps:,.0f}")
    print(f"batch_interval_sec    : {result.batch_interval_sec:,.2f}")
    print(f"tx_per_batch          : {result.tx_per_batch:,.0f}")
    print(f"raw_batch_mb          : {result.raw_batch_mb:,.3f}")
    print(f"compressed_batch_mb   : {result.compressed_batch_mb:,.3f}")
    print(f"required_da_mb_per_sec: {result.required_da_mb_per_sec:,.3f}")
    print(f"fits_max_batch        : {result.fits_max_batch}")


def main() -> int:
    args = parse_args()
    try:
        result = model(
            target_tps=args.target_tps,
            batch_interval_sec=args.batch_interval_sec,
            avg_tx_bytes=args.avg_tx_bytes,
            compression_ratio=args.compression_ratio,
            max_batch_mb=args.max_batch_mb,
        )
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
