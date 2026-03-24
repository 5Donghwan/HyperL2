#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

RPC_URL=${KAIA_RPC_URL:-https://testnet.zkrypton.zkrypto.com}
VERIFIER_ADDRESS=${KAIA_VERIFIER_ADDRESS:-0xeb654CdB749ECe55B656A9382fC38fB48Ee2eF95}
ARTIFACTS_DIR=${KAIA_OUTPUT_DIR:-$ROOT/build/kaia-preflight}/contracts
MANIFEST_PATH=${VECTIS_MANIFEST_PATH:-$ROOT/rust/vectis-prover/Cargo.toml}
BATCH_SIZE=${HYPERL2_BATCH_SIZE:-20000}
LANE_COUNT=${HYPERL2_LANE_COUNT:-16}
SEED=${HYPERL2_SEED:-7}
BATCH_ID_BASE=${HYPERL2_BATCH_ID_BASE:-1}
REPEAT_COUNT=${REPEAT_COUNT:-5}
REPLAY_BUNDLE_PATH=${KAIA_REPLAY_BUNDLE_PATH:-$ROOT/build/dashboard/replay-bundle-16p6s.json}
OUTPUT_DIR=${KAIA_REPEAT_OUTPUT_DIR:-$ROOT/build/benchmarks/16p6s-$(date +%Y%m%d-%H%M%S)}
RUNS_JSONL="$OUTPUT_DIR/runs.jsonl"
SUMMARY_JSON="$OUTPUT_DIR/summary.json"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd cargo
need_cmd go
need_cmd jq

if [[ -z "${KAIA_PRIVATE_KEYS:-}" ]]; then
  echo "set KAIA_PRIVATE_KEYS to 6 comma-separated funded private keys" >&2
  exit 1
fi

IFS=',' read -r -a KEYS <<< "$KAIA_PRIVATE_KEYS"
if [[ ${#KEYS[@]} -ne 6 ]]; then
  echo "expected 6 private keys in KAIA_PRIVATE_KEYS, got ${#KEYS[@]}" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR" "$(dirname "$REPLAY_BUNDLE_PATH")"

(
  cd "$ROOT"
  GOCACHE=${GOCACHE:-$ROOT/.gocache} \
  GOMODCACHE=${GOMODCACHE:-$ROOT/.gomodcache} \
  go build -o "$ROOT/kaia-proofctl" ./cmd/kaia-proofctl
)

if [[ ! -f "$REPLAY_BUNDLE_PATH" ]]; then
  echo "[16p6s-repeat] generating replay bundle: $REPLAY_BUNDLE_PATH"
  cargo +stable run --release \
    --manifest-path "$MANIFEST_PATH" \
    --bin export_sumproof_bundle -- \
    --batch-size "$BATCH_SIZE" \
    --lane-count "$LANE_COUNT" \
    --batch-id-base "$BATCH_ID_BASE" \
    --seed "$SEED" \
    > "$REPLAY_BUNDLE_PATH"
fi

for repeat in $(seq 1 "$REPEAT_COUNT"); do
  echo "[16p6s-repeat] repeat=$repeat deploying 6 fresh certifiers"
  CERTIFIERS=()
  for key in "${KEYS[@]}"; do
    cert=$(KAIA_PRIVATE_KEY="$key" "$ROOT/kaia-proofctl" deploy-certifier \
      --rpc-url "$RPC_URL" \
      --verifier-address "$VERIFIER_ADDRESS" | jq -r '.contract_address')
    CERTIFIERS+=("$cert")
  done

  i=0
  for key in "${KEYS[@]}"; do
    KAIA_PRIVATE_KEY="$key" "$ROOT/kaia-proofctl" init-lane-heads \
      --rpc-url "$RPC_URL" \
      --certifier-address "${CERTIFIERS[$i]}" \
      --bundle "$REPLAY_BUNDLE_PATH" \
      --skip-existing=false >/dev/null
    i=$((i + 1))
  done

  JOINED_KEYS=$(IFS=,; echo "${KEYS[*]}")
  JOINED_CERTIFIERS=$(IFS=,; echo "${CERTIFIERS[*]}")
  out=$("$ROOT/kaia-proofctl" submit-multisender-burst \
    --rpc-url "$RPC_URL" \
    --private-keys "$JOINED_KEYS" \
    --certifier-addresses "$JOINED_CERTIFIERS" \
    --bundle "$REPLAY_BUNDLE_PATH" \
    --artifacts-dir "$ARTIFACTS_DIR")

  echo "$out" | jq --argjson repeat "$repeat" '. + {repeat:$repeat, proofs_per_call:16, sender_count:6}' | tee -a "$RUNS_JSONL"
  tps=$(echo "$out" | jq -r '.aggregate_receipt_tps')
  window=$(echo "$out" | jq -r '.window_ms')
  total=$(echo "$out" | jq -r '.verified_tx_total')
  echo "[16p6s-repeat] repeat=$repeat verified_tx_total=$total window_ms=$window receipt_tps=$tps"
done

jq -s '
  map({
    repeat,
    proofs_per_call,
    sender_count,
    verified_tx_total,
    window_ms,
    receipt_tps:(.aggregate_receipt_tps|tonumber),
    block:(.transactions[0].block_number|tonumber)
  }) as $runs |
  {
    run_count: ($runs | length),
    best: ($runs | max_by(.receipt_tps)),
    median: ($runs | sort_by(.receipt_tps) | .[(length/2|floor)]),
    worst: ($runs | min_by(.receipt_tps)),
    average_tps: (($runs | map(.receipt_tps) | add) / ($runs | length)),
    over_200k_count: ($runs | map(select(.receipt_tps >= 200000)) | length),
    runs: $runs
  }
' "$RUNS_JSONL" | tee "$SUMMARY_JSON"

echo "RUNS_JSONL=$RUNS_JSONL"
echo "SUMMARY_JSON=$SUMMARY_JSON"
