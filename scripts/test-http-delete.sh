#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"

log_file="$(mktemp /tmp/godot-mcp-go-delete.XXXXXX.log)"
init_headers="$(mktemp /tmp/godot-mcp-go-delete.XXXXXX.headers)"

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
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"init-delete","method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"make-delete","version":"0.1.0"}}}' >/dev/null

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$init_headers" | tail -n1)"
test -n "$session_id"

status_delete="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Mcp-Session-Id: $session_id" \
  -X DELETE "$SERVER_URL")"
test "$status_delete" = "204"

status_after_delete="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "Mcp-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":99,"method":"tools/list","params":{}}')"
test "$status_after_delete" = "404"

echo "HTTP DELETE lifecycle passed (session=$session_id)"
