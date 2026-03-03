#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-lifecycle-initialized-id.XXXXXX.log)"
init_headers="$(mktemp /tmp/godot-mcp-go-lifecycle-initialized-id.init.headers.XXXXXX)"
bad_notify_body="$(mktemp /tmp/godot-mcp-go-lifecycle-initialized-id.bad-notify.body.XXXXXX)"
tools_body="$(mktemp /tmp/godot-mcp-go-lifecycle-initialized-id.tools.body.XXXXXX)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$init_headers" "$bad_notify_body" "$tools_body"
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
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-lifecycle-id\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"lifecycle-id\",\"version\":\"0.1.0\"}}}" >/dev/null

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$init_headers" | tail -n1)"
test -n "$session_id"

status_bad_notify="$(curl -sS -o "$bad_notify_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"bad-initialized-id","method":"notifications/initialized"}')"
test "$status_bad_notify" = "200"

compact_bad_notify="$(tr -d '[:space:]' < "$bad_notify_body")"
case "$compact_bad_notify" in
  *'"code":-32600'*'"message":"Invalidrequest"'*) ;;
  *)
    echo "expected invalid_request for notifications/initialized with id:"
    cat "$bad_notify_body"
    exit 1
    ;;
esac

status_notify="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify" = "202"

status_tools="$(curl -sS -o "$tools_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"tools-after-good-initialized","method":"tools/list","params":{}}')"
test "$status_tools" = "200"

compact_tools="$(tr -d '[:space:]' < "$tools_body")"
case "$compact_tools" in
  *'"result":{"tools":['*) ;;
  *'"result":{"tools":[]'* ) ;;
  *'"error":'*)
    echo "expected tools/list success after valid initialized:"
    cat "$tools_body"
    exit 1
    ;;
  *)
    echo "unexpected tools/list response after valid initialized:"
    cat "$tools_body"
    exit 1
    ;;
esac

"$GO_BIN" test ./transport/stdio -run '^TestInitializedNotificationWithIDRejected$'

echo "Lifecycle initialized-id checks passed (http + stdio)"
