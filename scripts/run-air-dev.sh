#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.gomodcache}"
mkdir -p "$GOCACHE" "$GOMODCACHE"

echo "[air-dev] generating datasets (optional)"
go run ./cmd/bench-orchestrator --config configs/dev-air/config.json --mode generate-datasets

echo "[air-dev] starting benchmark"
go run ./cmd/bench-orchestrator --config configs/dev-air/config.json --mode run
