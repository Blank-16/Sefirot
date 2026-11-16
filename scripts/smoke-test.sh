#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
BIN="$TMP/sefirot"
READY="$TMP/ready.txt"
LOG="$TMP/server.log"
cleanup() {
  if [[ -n "${PID:-}" ]]; then kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT
cd "$ROOT/backend"
go build -o "$BIN" .
"$BIN" --data-dir "$TMP/data" --port 0 --ready-file "$READY" >"$LOG" 2>&1 &
PID=$!
for _ in $(seq 1 100); do
  if [[ -s "$READY" ]]; then break; fi
  if ! kill -0 "$PID" 2>/dev/null; then cat "$LOG"; exit 1; fi
  sleep 0.1
done
if [[ ! -s "$READY" ]]; then cat "$LOG"; echo "SEFIROT did not become ready" >&2; exit 1; fi
URL="$(sed -n '1p' "$READY")"
TOKEN="$(sed -n '2p' "$READY")"
curl --fail --silent --show-error "$URL/api/health" >/dev/null
curl --fail --silent --show-error "$URL/" | grep -q 'SEFIROT'
curl --fail --silent --show-error -X POST -H "X-Vault-Token: $TOKEN" "$URL/api/shutdown" >/dev/null
wait "$PID"
PID=
echo "SEFIROT smoke test passed at $URL"
