#!/usr/bin/env python3
"""
Generate benchmark experiment matrix rows.
"""

import argparse
import csv
import itertools
import json
import sys
from dataclasses import asdict, dataclass
from typing import List


@dataclass
class ExperimentRow:
    id: str
    target_tps: float
    sequencer_count: int
    batch_interval_sec: float
    prover_workers: int


def parse_list(raw: str, cast):
    values = []
    for part in raw.split(","):
        item = part.strip()
        if not item:
            continue
        values.append(cast(item))
    if not values:
        raise ValueError("list input cannot be empty")
    return values


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Create a small experiment matrix for Kaia performance runs.")
    parser.add_argument("--target-tps", type=float, required=True)
    parser.add_argument("--sequencer-counts", required=True, help="Comma-separated integers.")
    parser.add_argument("--batch-intervals", required=True, help="Comma-separated seconds.")
    parser.add_argument("--prover-workers", required=True, help="Comma-separated integers.")
    parser.add_argument("--csv", help="Optional CSV output path.")
    parser.add_argument("--json", action="store_true")
    return parser.parse_args()


def build_matrix(
    target_tps: float,
    sequencer_counts: List[int],
    batch_intervals: List[float],
    prover_workers: List[int],
) -> List[ExperimentRow]:
    rows = []
    i = 1
    for seq_count, interval, workers in itertools.product(
        sequencer_counts, batch_intervals, prover_workers
    ):
        rows.append(
            ExperimentRow(
                id=f"exp-{i:03d}",
                target_tps=target_tps,
                sequencer_count=seq_count,
                batch_interval_sec=interval,
                prover_workers=workers,
            )
        )
        i += 1
    return rows


def write_csv(path: str, rows: List[ExperimentRow]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as f:
        writer = csv.writer(f)
        writer.writerow(["id", "target_tps", "sequencer_count", "batch_interval_sec", "prover_workers"])
        for row in rows:
            writer.writerow(
                [row.id, row.target_tps, row.sequencer_count, row.batch_interval_sec, row.prover_workers]
            )


def print_table(rows: List[ExperimentRow]) -> None:
    print("id,target_tps,sequencer_count,batch_interval_sec,prover_workers")
    for row in rows:
        print(
            f"{row.id},{row.target_tps:.0f},{row.sequencer_count},"
            f"{row.batch_interval_sec:.2f},{row.prover_workers}"
        )


def main() -> int:
    args = parse_args()

    try:
        if args.target_tps <= 0:
            raise ValueError("target_tps must be > 0")
        sequencer_counts = parse_list(args.sequencer_counts, int)
        batch_intervals = parse_list(args.batch_intervals, float)
        prover_workers = parse_list(args.prover_workers, int)
    except ValueError as err:
        print(f"Input error: {err}")
        return 2

    rows = build_matrix(
        target_tps=args.target_tps,
        sequencer_counts=sequencer_counts,
        batch_intervals=batch_intervals,
        prover_workers=prover_workers,
    )

    if args.csv:
        write_csv(args.csv, rows)
        print(f"Wrote {len(rows)} rows to {args.csv}")

    if args.json:
        print(json.dumps([asdict(r) for r in rows], indent=2))
    else:
        print_table(rows)

    return 0


if __name__ == "__main__":
    sys.exit(main())
