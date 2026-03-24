#!/usr/bin/env python3
"""
Simple bridge risk model.

Estimate operational withdrawal caps from TVL assumptions.
"""

import argparse
import json
from dataclasses import asdict, dataclass


@dataclass
class BridgeRisk:
    tvl_usd: float
    daily_cap_usd: float
    hourly_spike_cap_usd: float
    warning_threshold_usd: float
    critical_threshold_usd: float


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Model bridge withdrawal risk caps from TVL.")
    parser.add_argument("--tvl-usd", type=float, required=True, help="Total bridge TVL in USD.")
    parser.add_argument(
        "--daily-cap-pct",
        type=float,
        default=0.08,
        help="Daily withdrawal cap as fraction of TVL (0 to 1).",
    )
    parser.add_argument(
        "--hourly-spike-cap-pct",
        type=float,
        default=0.015,
        help="Hourly spike cap as fraction of TVL (0 to 1).",
    )
    parser.add_argument("--json", action="store_true", help="Emit JSON.")
    return parser.parse_args()


def validate_fraction(name: str, value: float) -> None:
    if value < 0 or value > 1:
        raise ValueError(f"{name} must be in [0,1], got {value}")


def model(tvl_usd: float, daily_cap_pct: float, hourly_spike_cap_pct: float) -> BridgeRisk:
    if tvl_usd <= 0:
        raise ValueError(f"tvl_usd must be > 0, got {tvl_usd}")
    validate_fraction("daily_cap_pct", daily_cap_pct)
    validate_fraction("hourly_spike_cap_pct", hourly_spike_cap_pct)

    daily_cap = tvl_usd * daily_cap_pct
    hourly_cap = tvl_usd * hourly_spike_cap_pct
    warning = daily_cap * 0.7
    critical = daily_cap * 0.9

    return BridgeRisk(
        tvl_usd=tvl_usd,
        daily_cap_usd=daily_cap,
        hourly_spike_cap_usd=hourly_cap,
        warning_threshold_usd=warning,
        critical_threshold_usd=critical,
    )


def print_table(result: BridgeRisk) -> None:
    print("Bridge Risk Model")
    print(f"tvl_usd                : {result.tvl_usd:,.2f}")
    print(f"daily_cap_usd          : {result.daily_cap_usd:,.2f}")
    print(f"hourly_spike_cap_usd   : {result.hourly_spike_cap_usd:,.2f}")
    print(f"warning_threshold_usd  : {result.warning_threshold_usd:,.2f}")
    print(f"critical_threshold_usd : {result.critical_threshold_usd:,.2f}")


def main() -> int:
    args = parse_args()
    try:
        result = model(args.tvl_usd, args.daily_cap_pct, args.hourly_spike_cap_pct)
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
