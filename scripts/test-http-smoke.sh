#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-smoke.XXXXXX.log)"
discover_body="$(mktemp /tmp/godot-mcp-go-smoke.discover.XXXXXX.body)"
tools_body="$(mktemp /tmp/godot-mcp-go-smoke.tools.XXXXXX.body)"
removed_method_body="$(mktemp /tmp/godot-mcp-go-smoke.removed-method.XXXXXX.body)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$discover_body" "$tools_body" "$removed_method_body"
}
trap cleanup EXIT

require_contains() {
  smoke_haystack="$1"
  smoke_needle="$2"
  smoke_label="$3"
  case "$smoke_haystack" in
    *"$smoke_needle"*) ;;
    *)
      echo "assert failed: $smoke_label"
      echo "expected fragment: $smoke_needle"
      exit 1
      ;;
  esac
}

"$GO_BIN" run main.go >"$log_file" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 80); do
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "server process exited before readiness"
    cat "$log_file"
    exit 1
  fi
  if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.2
done
test "$ready" = 1

editor_id="editor-http-smoke"
discover_payload="$(mcp_request discover server/discover '{}' "$editor_id")"
status_discover="$(curl -sS -o "$discover_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: server/discover' \
  -X POST "$SERVER_URL" \
  --data "$discover_payload")"
test "$status_discover" = 200
discover_compact="$(tr -d '[:space:]' < "$discover_body")"
require_contains "$discover_compact" '"resultType":"complete"' "server/discover should use modern result envelope"
require_contains "$discover_compact" '"supportedVersions":["2026-07-28"]' "server/discover should advertise 2026-07-28"

tools_payload="$(mcp_request tools-list tools/list '{}' "$editor_id")"
status_tools="$(curl -sS -o "$tools_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: tools/list' \
  -X POST "$SERVER_URL" \
  --data "$tools_payload")"
test "$status_tools" = 200
tools_compact="$(tr -d '[:space:]' < "$tools_body")"
require_contains "$tools_compact" '"resultType":"complete"' "tools/list should use modern result envelope"

status_missing_header="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'Mcp-Method: tools/list' \
  -X POST "$SERVER_URL" \
  --data "$tools_payload")"
test "$status_missing_header" = 400

removed_method_payload="$(mcp_request removed initialize '{}' "$editor_id")"
status_removed_method="$(curl -sS -o "$removed_method_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H 'Mcp-Method: initialize' \
  -X POST "$SERVER_URL" \
  --data "$removed_method_payload")"
test "$status_removed_method" = 404
require_contains "$(tr -d '[:space:]' < "$removed_method_body")" '"code":-32601' "removed initialize method should be rejected"

status_get="$(curl -sS -o /dev/null -w "%{http_code}" -X GET "$SERVER_URL")"
test "$status_get" = 405

echo "Modern HTTP smoke passed (protocol=$PROTOCOL_VERSION)"
