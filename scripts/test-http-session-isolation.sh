#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-19080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-session-isolation.XXXXXX.log)"
sync_a_body="$(mktemp /tmp/godot-mcp-go-session-isolation.a.sync.XXXXXX.body)"
sync_b_body="$(mktemp /tmp/godot-mcp-go-session-isolation.b.sync.XXXXXX.body)"
state_a_body="$(mktemp /tmp/godot-mcp-go-session-isolation.a.state.XXXXXX.body)"
state_b_body="$(mktemp /tmp/godot-mcp-go-session-isolation.b.state.XXXXXX.body)"
cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$sync_a_body" "$sync_b_body" "$state_a_body" "$state_b_body"
}
trap cleanup EXIT

require_contains() {
  isolation_haystack="$1"
  isolation_needle="$2"
  isolation_label="$3"
  case "$isolation_haystack" in
    *"$isolation_needle"*) ;;
    *) echo "assert failed: $isolation_label"; echo "expected fragment: $isolation_needle"; exit 1 ;;
  esac
}

require_not_contains() {
  isolation_haystack="$1"
  isolation_needle="$2"
  isolation_label="$3"
  case "$isolation_haystack" in
    *"$isolation_needle"*) echo "assert failed: $isolation_label"; exit 1 ;;
    *) ;;
  esac
}

"$GO_BIN" run main.go >"$log_file" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 80); do
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "server process exited before readiness"; cat "$log_file"; exit 1
  fi
  if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.2
done
test "$ready" = 1

sync_payload_a="$(mcp_request sync-a tools/call '{"name":"godot.bridge.editor.sync","arguments":{"snapshot":{"root_summary":{"active_scene":"res://SessionA.tscn"},"scene_tree":{"path":"/RootA","name":"RootA","type":"Node2D","child_count":0},"node_details":{"/RootA":{"path":"/RootA","name":"RootA","type":"Node2D","child_count":0}}}}}' editor-session-a)"
status_sync_a="$(curl -sS -o "$sync_a_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.bridge.editor.sync' -X POST "$SERVER_URL" --data "$sync_payload_a")"
test "$status_sync_a" = 200
require_not_contains "$(tr -d '[:space:]' < "$sync_a_body")" '"isError":true' "editor A sync should succeed"

sync_payload_b="$(mcp_request sync-b tools/call '{"name":"godot.bridge.editor.sync","arguments":{"snapshot":{"root_summary":{"active_scene":"res://SessionB.tscn"},"scene_tree":{"path":"/RootB","name":"RootB","type":"Node2D","child_count":0},"node_details":{"/RootB":{"path":"/RootB","name":"RootB","type":"Node2D","child_count":0}}}}}' editor-session-b)"
status_sync_b="$(curl -sS -o "$sync_b_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.bridge.editor.sync' -X POST "$SERVER_URL" --data "$sync_payload_b")"
test "$status_sync_b" = 200
require_not_contains "$(tr -d '[:space:]' < "$sync_b_body")" '"isError":true' "editor B sync should succeed"

state_payload_a="$(mcp_request state-a tools/call '{"name":"godot.editor.state.get","arguments":{}}' editor-session-a)"
status_state_a="$(curl -sS -o "$state_a_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.editor.state.get' -X POST "$SERVER_URL" --data "$state_payload_a")"
test "$status_state_a" = 200
state_a_compact="$(tr -d '[:space:]' < "$state_a_body")"
require_contains "$state_a_compact" '"active_scene":"res://SessionA.tscn"' "editor A state should stay isolated"
require_not_contains "$state_a_compact" '"active_scene":"res://SessionB.tscn"' "editor A must not see editor B"

state_payload_b="$(mcp_request state-b tools/call '{"name":"godot.editor.state.get","arguments":{}}' editor-session-b)"
status_state_b="$(curl -sS -o "$state_b_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.editor.state.get' -X POST "$SERVER_URL" --data "$state_payload_b")"
test "$status_state_b" = 200
state_b_compact="$(tr -d '[:space:]' < "$state_b_body")"
require_contains "$state_b_compact" '"active_scene":"res://SessionB.tscn"' "editor B state should stay isolated"
require_not_contains "$state_b_compact" '"active_scene":"res://SessionA.tscn"' "editor B must not see editor A"

echo "Modern explicit editor-session isolation passed"
