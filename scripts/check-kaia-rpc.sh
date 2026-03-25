#!/usr/bin/env bash
set -euo pipefail

RPC_URL=${1:-${KAIA_RPC_URL:-}}
if [[ -z "$RPC_URL" ]]; then
  echo "usage: $0 <rpc-url>" >&2
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}
need_cmd curl
need_cmd jq

rpc() {
  local method=$1
  local params=${2:-[]}
  curl -sS -H 'Content-Type: application/json' \
    --data "{\"jsonrpc\":\"2.0\",\"method\":\"${method}\",\"params\":${params},\"id\":1}" \
    "$RPC_URL"
}

echo "[rpc] modules"
rpc rpc_modules | jq .

echo "[rpc] chain id"
rpc eth_chainId | jq .

echo "[rpc] block number"
rpc eth_blockNumber | jq .

echo "[rpc] latest 3 block timestamps"
for i in 1 2 3; do
  rpc eth_getBlockByNumber '["latest", false]' | jq '{number: .result.number, timestamp: .result.timestamp, txs: (.result.transactions | length)}'
  sleep 1
done
