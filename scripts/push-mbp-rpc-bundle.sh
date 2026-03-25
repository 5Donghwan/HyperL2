#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
ENV_FILE=${1:-$ROOT/configs/mbp-rpc/service-chain.env.example}
STAGE_DIR=${2:-$ROOT/build/mbp-rpc/staged}
REMOTE_SUBDIR=${REMOTE_SUBDIR:-hyperl2-mbp-rpc-bundle}

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE" >&2
  exit 1
fi

set -a
source "$ENV_FILE"
set +a

: "${MBP_HOST:?missing MBP_HOST}"
: "${MBP_USER:?missing MBP_USER}"
: "${MBP_PORT:=22}"
: "${KAIA_INSTALL_DIR:?missing KAIA_INSTALL_DIR}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd ssh
need_cmd rsync

bash "$ROOT/scripts/stage-mbp-rpc-bundle.sh" "$ENV_FILE" "$STAGE_DIR" >/dev/null

REMOTE_ROOT="${KAIA_INSTALL_DIR%/}/${REMOTE_SUBDIR}"
SSH_TARGET="${MBP_USER}@${MBP_HOST}"

ssh -p "$MBP_PORT" "$SSH_TARGET" "mkdir -p '$REMOTE_ROOT'"
rsync -az --delete -e "ssh -p $MBP_PORT" "$STAGE_DIR"/ "$SSH_TARGET":"$REMOTE_ROOT"/

cat <<EOF
Bundle uploaded to:
  ${SSH_TARGET}:${REMOTE_ROOT}

Suggested remote follow-up:
  1. Copy ${REMOTE_ROOT}/conf/ksend.conf into your extracted Kaia package conf directory.
  2. Copy ${REMOTE_ROOT}/bootstrap/static-nodes.json into ${KAIA_DATA_DIR}/static-nodes.json.
  3. If present, copy ${REMOTE_ROOT}/bootstrap/nodekey into ${KAIA_DATA_DIR}/klay/nodekey.
  4. Initialize the data directory with ${REMOTE_ROOT}/bootstrap/genesis.json before startup.
  5. Start the Kaia SEN/endpoint process and verify RPC health from this repo.
EOF
