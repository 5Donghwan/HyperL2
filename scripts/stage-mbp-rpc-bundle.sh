#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
ENV_FILE=${1:-$ROOT/configs/mbp-rpc/service-chain.env.example}
OUT_DIR=${2:-$ROOT/build/mbp-rpc/staged}

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

: "${KAIA_NODE_KIND:=EN}"

case "$KAIA_NODE_KIND" in
  EN)
    CONF_BASENAME=kend.conf
    NODE_LABEL="EN"
    ;;
  SEN)
    CONF_BASENAME=ksend.conf
    NODE_LABEL="SEN"
    ;;
  *)
    echo "unsupported KAIA_NODE_KIND: $KAIA_NODE_KIND" >&2
    exit 1
    ;;
esac

need_file() {
  local path=$1
  local label=$2
  if [[ -z "$path" || ! -f "$path" ]]; then
    echo "missing required ${label}: ${path:-<empty>}" >&2
    exit 1
  fi
}

need_file "${LAB_GENESIS_JSON:-}" "LAB_GENESIS_JSON"
need_file "${LAB_STATIC_NODES_JSON:-}" "LAB_STATIC_NODES_JSON"
if [[ -n "${LAB_NODEKEY:-}" ]]; then
  need_file "$LAB_NODEKEY" "LAB_NODEKEY"
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR/bootstrap" "$OUT_DIR/conf" "$OUT_DIR/notes"

"$ROOT/scripts/render-kaia-service-rpc-conf.sh" "$ENV_FILE" "$OUT_DIR/conf/$CONF_BASENAME" >/dev/null
cp "$LAB_GENESIS_JSON" "$OUT_DIR/bootstrap/genesis.json"
cp "$LAB_STATIC_NODES_JSON" "$OUT_DIR/bootstrap/static-nodes.json"
if [[ -n "${LAB_NODEKEY:-}" ]]; then
  cp "$LAB_NODEKEY" "$OUT_DIR/bootstrap/nodekey"
fi

cat > "$OUT_DIR/notes/README.txt" <<EOF
HyperL2 MBP dedicated RPC staging bundle

Remote host:
  host: ${MBP_HOST:-unset}
  user: ${MBP_USER:-unset}
  ssh_port: ${MBP_PORT:-22}

Suggested remote layout:
  install_dir: ${KAIA_INSTALL_DIR:-unset}
  data_dir: ${KAIA_DATA_DIR:-unset}
  log_dir: ${KAIA_LOG_DIR:-unset}

Contents:
  conf/${CONF_BASENAME}
  bootstrap/genesis.json
  bootstrap/static-nodes.json
  bootstrap/nodekey   (if provided)

Next steps on the MBP:
  1. Place ${CONF_BASENAME} into the extracted Kaia package conf directory.
  2. Copy bootstrap/static-nodes.json into the node DATA_DIR root.
  3. Copy bootstrap/nodekey into DATA_DIR/klay/nodekey if a dedicated nodekey was supplied.
  4. Initialize the data directory with the chain genesis before starting the node.
  5. Start the ${NODE_LABEL}/endpoint process and verify rpc_modules, eth_chainId, and block progression.
EOF

echo "$OUT_DIR"
