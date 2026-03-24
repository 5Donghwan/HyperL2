#!/usr/bin/env bash
set -euo pipefail

MANIFEST="$(cd -- "$(dirname -- "$0")/.." && pwd)/rust/vectis-prover/Cargo.toml"

cargo run --manifest-path "$MANIFEST" --bin sumproof_bench --release -- "$@"
