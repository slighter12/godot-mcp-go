#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-protocol-header.XXXXXX.log)"
dup_body="$(mktemp /tmp/godot-mcp-go-protocol-header.dup.XXXXXX.body)"
mixed_body="$(mktemp /tmp/godot-mcp-go-protocol-header.mixed.XXXXXX.body)"
cleanup() {
  stop_test_server
  rm -f "$log_file" "$dup_body" "$mixed_body"
}
trap cleanup EXIT

start_test_server "$log_file"

wait_for_test_server

payload="$(mcp_request header-test tools/list '{}' editor-http-header)"
status_dup="$(curl -sS -o "$dup_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION, $PROTOCOL_VERSION" \
  -H 'Mcp-Method: tools/list' \
  -X POST "$SERVER_URL" \
  --data "$payload")"
test "$status_dup" = 400
case "$(tr -d '[:space:]' < "$dup_body")" in
  *'"code":-32020'*) ;;
  *) echo "expected duplicate protocol header rejection:"; cat "$dup_body"; exit 1 ;;
esac

status_mixed="$(curl -sS -o "$mixed_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION, 2025-11-25" \
  -H 'Mcp-Method: tools/list' \
  -X POST "$SERVER_URL" \
  --data "$payload")"
test "$status_mixed" = 400
case "$(tr -d '[:space:]' < "$mixed_body")" in
  *'"code":-32020'*|*'"code":-32022'*) ;;
  *) echo "expected mixed protocol header rejection:"; cat "$mixed_body"; exit 1 ;;
esac

echo "HTTP protocol header checks passed (duplicate/mixed values rejected)"
