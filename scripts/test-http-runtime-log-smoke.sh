#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.XXXXXX.log)"
log_get_body="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.get.XXXXXX.body)"
log_clear_body="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.clear.XXXXXX.body)"
cleanup() {
  stop_test_server
  rm -f "$log_file" "$log_get_body" "$log_clear_body"
}
trap cleanup EXIT

start_test_server "$log_file"

wait_for_test_server

get_payload="$(mcp_request runtime-log-get tools/call '{"name":"godot.runtime.log.get","arguments":{"session_id":"game_missing","level":"error","limit":10}}' editor-runtime-log)"
status_get="$(curl -sS -o "$log_get_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: godot.runtime.log.get' \
  -X POST "$SERVER_URL" \
  --data "$get_payload")"
test "$status_get" = 200
compact_get="$(tr -d '[:space:]' < "$log_get_body")"
require_contains "$compact_get" '"isError":true' "runtime log get should return semantic error"
require_contains "$compact_get" '"kind":"not_available"' "runtime log get should report not_available"
require_contains "$compact_get" '"code":"game_session_missing"' "runtime log get should report game_session_missing"

clear_payload="$(mcp_request runtime-log-clear tools/call '{"name":"godot.runtime.log.clear","arguments":{"session_id":"game_missing"}}' editor-runtime-log)"
status_clear="$(curl -sS -o "$log_clear_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: tools/call' \
  -H 'Mcp-Name: godot.runtime.log.clear' \
  -X POST "$SERVER_URL" \
  --data "$clear_payload")"
test "$status_clear" = 200
compact_clear="$(tr -d '[:space:]' < "$log_clear_body")"
require_contains "$compact_clear" '"isError":true' "runtime log clear should return semantic error"
require_contains "$compact_clear" '"kind":"not_available"' "runtime log clear should report not_available"
require_contains "$compact_clear" '"code":"game_session_missing"' "runtime log clear should report game_session_missing"

echo "Modern HTTP runtime log smoke passed"
