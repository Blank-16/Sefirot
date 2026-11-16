#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-$(rustc -vV | sed -n 's/^host: //p')}"
if [[ -z "$TARGET" ]]; then
  echo "Could not determine Rust target triple. Pass it explicitly." >&2
  exit 1
fi

case "$TARGET" in
  x86_64-pc-windows-msvc) GOOS=windows GOARCH=amd64 EXT=.exe ;;
  aarch64-pc-windows-msvc) GOOS=windows GOARCH=arm64 EXT=.exe ;;
  x86_64-unknown-linux-gnu) GOOS=linux GOARCH=amd64 EXT= ;;
  aarch64-unknown-linux-gnu) GOOS=linux GOARCH=arm64 EXT= ;;
  x86_64-apple-darwin) GOOS=darwin GOARCH=amd64 EXT= ;;
  aarch64-apple-darwin) GOOS=darwin GOARCH=arm64 EXT= ;;
  *) echo "Unsupported target triple: $TARGET" >&2; exit 2 ;;
esac

OUT="$ROOT/desktop/src-tauri/binaries/sefirot-server-$TARGET$EXT"
LDFLAGS="-s -w"
if [[ "$GOOS" == windows ]]; then LDFLAGS="-s -w -H=windowsgui"; fi
mkdir -p "$(dirname "$OUT")"
cd "$ROOT/backend"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="$LDFLAGS" -o "$OUT" .
if [[ "$GOOS" != windows ]]; then chmod +x "$OUT"; fi
echo "Built sidecar: $OUT"
