#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.XXXXXX.log)"
init_headers="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.XXXXXX.headers)"
log_get_body="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.XXXXXX.get.body)"
log_clear_body="$(mktemp /tmp/godot-mcp-go-runtime-log-smoke.XXXXXX.clear.body)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$init_headers" "$log_get_body" "$log_clear_body"
}
trap cleanup EXIT

require_contains() {
  haystack="$1"
  needle="$2"
  label="$3"
  case "$haystack" in
    *"$needle"*) ;;
    *)
      echo "assert failed: $label"
      echo "expected fragment: $needle"
      exit 1
      ;;
  esac
}

"$GO_BIN" run main.go >"$log_file" 2>&1 &
server_pid=$!

for _ in $(seq 1 80); do
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "server process exited before readiness"
    cat "$log_file"
    exit 1
  fi
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
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-runtime-log-smoke\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{\"godot\":{\"mutating\":true}},\"clientInfo\":{\"name\":\"runtime-log-smoke\",\"version\":\"0.2.0\"}}}" >/dev/null

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$init_headers" | tail -n1)"
test -n "$session_id"

status_notify="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify" = "202"

status_log_get="$(curl -sS -o "$log_get_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"runtime-log-get-smoke","method":"tools/call","params":{"name":"godot.runtime.log.get","arguments":{"session_id":"game_missing","level":"error","limit":10}}}')"
test "$status_log_get" = "200"
compact_log_get="$(tr -d '[:space:]' < "$log_get_body")"
require_contains "$compact_log_get" '"isError":true' "runtime log get should return semantic error for missing session"
require_contains "$compact_log_get" '"kind":"not_available"' "runtime log get should report not_available"
require_contains "$compact_log_get" '"code":"game_session_missing"' "runtime log get should report game_session_missing"
require_contains "$compact_log_get" '"tool":"godot.runtime.log.get"' "runtime log get should identify the tool"

status_log_clear="$(curl -sS -o "$log_clear_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"runtime-log-clear-smoke","method":"tools/call","params":{"name":"godot.runtime.log.clear","arguments":{"session_id":"game_missing"}}}')"
test "$status_log_clear" = "200"
compact_log_clear="$(tr -d '[:space:]' < "$log_clear_body")"
require_contains "$compact_log_clear" '"isError":true' "runtime log clear should return semantic error for missing session"
require_contains "$compact_log_clear" '"kind":"not_available"' "runtime log clear should report not_available"
require_contains "$compact_log_clear" '"code":"game_session_missing"' "runtime log clear should report game_session_missing"
require_contains "$compact_log_clear" '"tool":"godot.runtime.log.clear"' "runtime log clear should identify the tool"

echo "HTTP runtime log smoke passed (session=$session_id)"
