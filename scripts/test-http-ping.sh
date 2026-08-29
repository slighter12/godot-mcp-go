#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-ping.XXXXXX.log)"
ping_body_file="$(mktemp /tmp/godot-mcp-go-ping.XXXXXX.body)"
cleanup() {
  stop_test_server
  rm -f "$log_file" "$ping_body_file"
}
trap cleanup EXIT

start_test_server "$log_file"

wait_for_test_server

payload="$(mcp_request ping ping '{}' editor-http-ping)"
status_ping="$(curl -sS -o "$ping_body_file" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: ping' \
  -X POST "$SERVER_URL" \
  --data "$payload")"
test "$status_ping" = 404

compact_ping="$(tr -d '[:space:]' < "$ping_body_file")"
case "$compact_ping" in
  *'"code":-32601'*) ;;
  *)
    echo "expected modern ping rejection:"
    cat "$ping_body_file"
    exit 1
    ;;
esac

echo "HTTP ping rejection passed (server/discover replaces ping)"
