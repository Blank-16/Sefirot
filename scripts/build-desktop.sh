#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-}"
cd "$ROOT"
npm install
npm run frontend:build
if [[ -n "$TARGET" ]]; then
  ./scripts/build-desktop-sidecar.sh "$TARGET"
else
  ./scripts/build-desktop-sidecar.sh
fi
npm --prefix desktop install
if [[ -n "$TARGET" ]]; then
  npm --prefix desktop run build -- --target "$TARGET"
else
  npm --prefix desktop run build
fi
