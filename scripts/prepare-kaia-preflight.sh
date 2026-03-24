#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$ROOT_DIR/rust/vectis-prover/Cargo.toml"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/build/kaia-preflight}"
BATCH_SIZE="${BATCH_SIZE:-20000}"
LANE_COUNT="${LANE_COUNT:-10}"
BENCH_REPEATS="${BENCH_REPEATS:-1}"
SEED="${SEED:-7}"
LANE_BASE="${LANE_BASE:-0}"
BATCH_ID_BASE="${BATCH_ID_BASE:-1}"

export RUSTUP_HOME="${RUSTUP_HOME:-$HOME/.rustup}"
export CARGO_HOME="${CARGO_HOME:-$HOME/.cargo}"

mkdir -p "$OUTPUT_DIR/contracts"

echo "[preflight] output dir: $OUTPUT_DIR"
echo "[preflight] batch_size=$BATCH_SIZE lane_count=$LANE_COUNT bench_repeats=$BENCH_REPEATS seed=$SEED"

{
  echo "date_utc=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "root_dir=$ROOT_DIR"
  echo "batch_size=$BATCH_SIZE"
  echo "lane_count=$LANE_COUNT"
  echo "bench_repeats=$BENCH_REPEATS"
  echo "seed=$SEED"
  echo "cargo_version=$(cargo +stable --version)"
  echo "rustc_version=$(rustc +stable --version)"
  echo "solc_version=$(solc --version | tr '\n' ' ' | sed 's/  */ /g')"
} > "$OUTPUT_DIR/versions.txt"

echo "[preflight] running Rust checks"
cargo +stable test --manifest-path "$MANIFEST" sum_preserving_batch_round_trip -- --nocapture \
  > "$OUTPUT_DIR/test-sum-proof.txt"
cargo +stable test --manifest-path "$MANIFEST" tampered_metadata_fails_verification -- --nocapture \
  > "$OUTPUT_DIR/test-metadata-tamper.txt"
cargo +stable test --manifest-path "$MANIFEST" state_transition_builder_preserves_total_sum -- --nocapture \
  > "$OUTPUT_DIR/test-state-builder.txt"

echo "[preflight] compiling Solidity contracts"
solc \
  --base-path "$ROOT_DIR" \
  --include-path "$ROOT_DIR" \
  --abi \
  --bin \
  --overwrite \
  -o "$OUTPUT_DIR/contracts" \
  "$ROOT_DIR/contracts/Bn254.sol" \
  "$ROOT_DIR/contracts/ICCGroth16BatchVerifier.sol" \
  "$ROOT_DIR/contracts/CCGroth16BatchVerifier.sol" \
  "$ROOT_DIR/contracts/SumPreservingBatchVerifier.sol" \
  > "$OUTPUT_DIR/solc-build.txt"

echo "[preflight] benchmarking 20k proof path"
cargo +stable run --manifest-path "$MANIFEST" --bin sumproof_bench --release -- \
  --batch-size "$BATCH_SIZE" \
  --repeats "$BENCH_REPEATS" \
  > "$OUTPUT_DIR/bench.txt"

echo "[preflight] exporting single fixture"
cargo +stable run --release --manifest-path "$MANIFEST" --bin export_sumproof_fixture -- \
  --batch-size "$BATCH_SIZE" \
  --lane-id "$LANE_BASE" \
  --batch-id "$BATCH_ID_BASE" \
  --seed "$SEED" \
  > "$OUTPUT_DIR/single-fixture.json"

echo "[preflight] exporting 10-proof certification bundle"
cargo +stable run --release --manifest-path "$MANIFEST" --bin export_sumproof_bundle -- \
  --batch-size "$BATCH_SIZE" \
  --lane-count "$LANE_COUNT" \
  --batch-id-base "$BATCH_ID_BASE" \
  --seed "$SEED" \
  > "$OUTPUT_DIR/certification-bundle.json"

echo "[preflight] hashing outputs"
(
  cd "$OUTPUT_DIR"
  shasum -a 256 versions.txt bench.txt single-fixture.json certification-bundle.json contracts/* \
    > SHA256SUMS
)

cat > "$OUTPUT_DIR/SUMMARY.md" <<EOF
# Kaia Endpoint Preflight

This bundle is ready for endpoint attachment.

Included:
- Solidity ABIs and bytecode in \`contracts/\`
- Single proof fixture in \`single-fixture.json\`
- 10-proof certification bundle in \`certification-bundle.json\`
- Local proof benchmark in \`bench.txt\`
- Rust correctness checks in \`test-*.txt\`
- Hash manifest in \`SHA256SUMS\`

Next step on the endpoint side:
1. Deploy \`CCGroth16BatchVerifier\`
2. Initialize its verifying key from \`certification-bundle.json.verifyingKey\`
3. Deploy \`SumPreservingBatchVerifier\` with the verifier address
4. Initialize each lane head from \`certification-bundle.json.laneHeads\`
5. Call \`verifyTenBatches\` with \`certification-bundle.json.verifyTenBatchesInput.artifacts\`

This preflight does not talk to any RPC endpoint.
EOF

echo "[preflight] done"
