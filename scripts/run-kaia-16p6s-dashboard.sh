#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)

RPC_URL=${KAIA_RPC_URL:-https://testnet.zkrypton.zkrypto.com}
VERIFIER_ADDRESS=${KAIA_VERIFIER_ADDRESS:-0xeb654CdB749ECe55B656A9382fC38fB48Ee2eF95}
ARTIFACTS_DIR=${KAIA_OUTPUT_DIR:-$ROOT/build/kaia-preflight}/contracts
MANIFEST_PATH=${VECTIS_MANIFEST_PATH:-$ROOT/rust/vectis-prover/Cargo.toml}
LISTEN=${DASHBOARD_LISTEN:-:8088}
BATCH_SIZE=${HYPERL2_BATCH_SIZE:-20000}
LANE_COUNT=${HYPERL2_LANE_COUNT:-16}
LIVE_LANE_COUNT=${HYPERL2_LIVE_LANE_COUNT:-1}
SEED=${HYPERL2_SEED:-7}
BATCH_ID_BASE=${HYPERL2_BATCH_ID_BASE:-1}
REPLAY_BUNDLE_PATH=${KAIA_REPLAY_BUNDLE_PATH:-$ROOT/build/dashboard/replay-bundle-16p6s.json}
LIVE_BUNDLE_PATH=${KAIA_BUNDLE_PATH:-$ROOT/build/dashboard/live1-replay15-16p6s.json}
TITLE=${DASHBOARD_TITLE:-HyperL2 16p6s Dashboard}

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

mkdir -p "$(dirname "$REPLAY_BUNDLE_PATH")" "$(dirname "$LIVE_BUNDLE_PATH")"

(
  cd "$ROOT"
  GOCACHE=${GOCACHE:-$ROOT/.gocache} \
  GOMODCACHE=${GOMODCACHE:-$ROOT/.gomodcache} \
  go build -o "$ROOT/kaia-proofctl" ./cmd/kaia-proofctl
)

if [[ ! -f "$REPLAY_BUNDLE_PATH" ]]; then
  echo "[16p6s] generating replay bundle: $REPLAY_BUNDLE_PATH"
  cargo +stable run --release \
    --manifest-path "$MANIFEST_PATH" \
    --bin export_sumproof_bundle -- \
    --batch-size "$BATCH_SIZE" \
    --lane-count "$LANE_COUNT" \
    --batch-id-base "$BATCH_ID_BASE" \
    --seed "$SEED" \
    > "$REPLAY_BUNDLE_PATH"
fi

CERTIFIERS=()
for key in "${KEYS[@]}"; do
  cert=$(KAIA_PRIVATE_KEY="$key" "$ROOT/kaia-proofctl" deploy-certifier \
    --rpc-url "$RPC_URL" \
    --verifier-address "$VERIFIER_ADDRESS" | jq -r '.contract_address')
  CERTIFIERS+=("$cert")
done

echo "[16p6s] deployed certifiers: ${CERTIFIERS[*]}"

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

echo "[16p6s] dashboard: http://127.0.0.1${LISTEN}"
exec "$ROOT/kaia-proofctl" dashboard \
  --listen "$LISTEN" \
  --title "$TITLE" \
  --manifest-path "$MANIFEST_PATH" \
  --bundle-path "$LIVE_BUNDLE_PATH" \
  --batch-size "$BATCH_SIZE" \
  --lane-count "$LANE_COUNT" \
  --live-lane-count "$LIVE_LANE_COUNT" \
  --replay-bundle-path "$REPLAY_BUNDLE_PATH" \
  --batch-id-base "$BATCH_ID_BASE" \
  --seed "$SEED" \
  --rpc-url "$RPC_URL" \
  --private-keys "$JOINED_KEYS" \
  --certifier-addresses "$JOINED_CERTIFIERS" \
  --artifacts-dir "$ARTIFACTS_DIR"
