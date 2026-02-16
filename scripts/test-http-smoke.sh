#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-smoke.XXXXXX.log)"
init_headers="$(mktemp /tmp/godot-mcp-go-smoke.XXXXXX.headers)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$log_file" "$init_headers"
}
trap cleanup EXIT

"$GO_BIN" run main.go >"$log_file" 2>&1 &
server_pid=$!

for _ in $(seq 1 80); do
  if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

curl -sS -D "$init_headers" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -X POST "$SERVER_URL" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-smoke\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"make-smoke\",\"version\":\"0.1.0\"}}}" >/dev/null

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$init_headers" | tail -n1)"
test -n "$session_id"

status_no_session="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}')"
test "$status_no_session" = "400"

status_bad_session="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'MCP-Session-Id: session_invalid' \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}')"
test "$status_bad_session" = "404"

status_ok="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}')"
test "$status_ok" = "200"

status_notify="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify" = "202"

echo "HTTP smoke passed (session=$session_id)"
