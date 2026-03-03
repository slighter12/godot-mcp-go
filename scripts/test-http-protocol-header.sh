#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-protocol-header.XXXXXX.log)"
dup_headers="$(mktemp /tmp/godot-mcp-go-protocol-header.dup.headers.XXXXXX)"
dup_body="$(mktemp /tmp/godot-mcp-go-protocol-header.dup.body.XXXXXX)"
mixed_body="$(mktemp /tmp/godot-mcp-go-protocol-header.mixed.body.XXXXXX)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$log_file" "$dup_headers" "$dup_body" "$mixed_body"
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

status_dup="$(curl -sS -D "$dup_headers" -o "$dup_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION, $PROTOCOL_VERSION" \
  -X POST "$SERVER_URL" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-dup-header\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"protocol-header\",\"version\":\"0.1.0\"}}}")"
test "$status_dup" = "200"

session_dup="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$dup_headers" | tail -n1)"
test -n "$session_dup"

status_mixed="$(curl -sS -o "$mixed_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION, 2024-11-05" \
  -X POST "$SERVER_URL" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-mixed-header\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"protocol-header\",\"version\":\"0.1.0\"}}}")"
test "$status_mixed" = "400"

compact_mixed="$(tr -d '[:space:]' < "$mixed_body")"
case "$compact_mixed" in
  *'"InvalidMCP-Protocol-Versionheader"'*) ;;
  *)
    echo "expected invalid protocol header error for mixed header values:"
    cat "$mixed_body"
    exit 1
    ;;
esac

echo "HTTP protocol header checks passed (duplicate accepted, mixed rejected)"
