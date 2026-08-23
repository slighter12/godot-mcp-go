#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"

log_file="$(mktemp /tmp/godot-mcp-go-delete.XXXXXX.log)"
cleanup() {
  stop_test_server
  rm -f "$log_file"
}
trap cleanup EXIT

start_test_server "$log_file"

wait_for_test_server

status_delete="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -X DELETE "$SERVER_URL")"
test "$status_delete" = 405

echo "HTTP DELETE rejection passed (modern Streamable HTTP has no session delete)"
