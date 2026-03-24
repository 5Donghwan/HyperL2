#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.gomodcache}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

echo "[cert-ultra] preparing datasets"
go run ./cmd/bench-orchestrator --config configs/cert-ultra/config.json --mode generate-datasets

echo "[cert-ultra] starting certification run"
go run ./cmd/bench-orchestrator --config configs/cert-ultra/config.json --mode run
