#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-modern-lifecycle.XXXXXX.log)"
initialize_body="$(mktemp /tmp/godot-mcp-go-modern-lifecycle.initialize.XXXXXX.body)"
notify_body="$(mktemp /tmp/godot-mcp-go-modern-lifecycle.notify.XXXXXX.body)"
tools_body="$(mktemp /tmp/godot-mcp-go-modern-lifecycle.tools.XXXXXX.body)"
cleanup() {
  stop_test_server
  rm -f "$log_file" "$initialize_body" "$notify_body" "$tools_body"
}
trap cleanup EXIT

start_test_server "$log_file"

ready=0
for _ in $(seq 1 80); do
  if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.2
done
test "$ready" = 1

initialize_payload="$(mcp_request initialize initialize '{}' editor-modern-lifecycle)"
status_initialize="$(curl -sS -o "$initialize_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: initialize' \
  -X POST "$SERVER_URL" \
  --data "$initialize_payload")"
test "$status_initialize" = 404
case "$(tr -d '[:space:]' < "$initialize_body")" in
  *'"code":-32601'*) ;;
  *) echo "expected initialize to be rejected:"; cat "$initialize_body"; exit 1 ;;
esac

notify_payload="$(mcp_notification notifications/initialized '{}' editor-modern-lifecycle)"
status_notify="$(curl -sS -o "$notify_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: notifications/initialized' \
  -X POST "$SERVER_URL" \
  --data "$notify_payload")"
test "$status_notify" = 400

tools_payload="$(mcp_request tools-direct tools/list '{}' editor-modern-lifecycle)"
status_tools="$(curl -sS -o "$tools_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: tools/list' \
  -X POST "$SERVER_URL" \
  --data "$tools_payload")"
test "$status_tools" = 200
case "$(tr -d '[:space:]' < "$tools_body")" in
  *'"resultType":"complete"'*) ;;
  *) echo "expected tools/list without lifecycle handshake:"; cat "$tools_body"; exit 1 ;;
esac

echo "Modern lifecycle checks passed (initialize/initialized rejected; direct tools/list accepted)"
