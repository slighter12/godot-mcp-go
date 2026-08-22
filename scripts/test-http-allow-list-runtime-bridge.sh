#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-19080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-modern-lib.sh"

log_file="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.config.XXXXXX.json)"
sync_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.sync.XXXXXX.body)"
state_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.state.XXXXXX.body)"
ping_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.ping.XXXXXX.body)"
ack_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.ack.XXXXXX.body)"
cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$runtime_config" "$sync_body" "$state_body" "$ping_body" "$ack_body"
}
trap cleanup EXIT

require_contains() {
  allow_haystack="$1"
  allow_needle="$2"
  allow_label="$3"
  case "$allow_haystack" in
    *"$allow_needle"*) ;;
    *) echo "assert failed: $allow_label"; echo "expected fragment: $allow_needle"; exit 1 ;;
  esac
}

require_not_contains() {
  allow_haystack="$1"
  allow_needle="$2"
  allow_label="$3"
  case "$allow_haystack" in
    *"$allow_needle"*) echo "assert failed: $allow_label"; echo "unexpected fragment: $allow_needle"; exit 1 ;;
    *) ;;
  esac
}

cp "./config/mcp_config.json" "$runtime_config"
sed -E \
  -e "s/\"port\"[[:space:]]*:[[:space:]]*[0-9]+/\"port\": ${SERVER_PORT}/" \
  -e "s|\"url\"[[:space:]]*:[[:space:]]*\"http://localhost:[0-9]+/mcp\"|\"url\": \"http://localhost:${SERVER_PORT}/mcp\"|" \
  -e "s/\"permission_mode\"[[:space:]]*:[[:space:]]*\"[^\"]+\"/\"permission_mode\": \"allow_list\"/" \
  -e 's/\"allowed_tools\"[[:space:]]*:[[:space:]]*\[[^]]*\]/\"allowed_tools\": [\"godot.editor.state.get\"]/' \
  "$runtime_config" > "${runtime_config}.tmp"
mv "${runtime_config}.tmp" "$runtime_config"

MCP_CONFIG_PATH="$runtime_config" "$GO_BIN" run main.go >"$log_file" 2>&1 &
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

editor_id="editor-allow-list"
sync_payload="$(mcp_request sync tools/call '{"name":"godot.bridge.editor.sync","arguments":{"snapshot":{"root_summary":{"active_scene":"res://AllowList.tscn"},"scene_tree":{"path":"/Root","name":"Root","type":"Node2D","child_count":0},"node_details":{"/Root":{"path":"/Root","name":"Root","type":"Node2D","child_count":0}}}}}' "$editor_id")"
status_sync="$(curl -sS -o "$sync_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.bridge.editor.sync' -X POST "$SERVER_URL" --data "$sync_payload")"
test "$status_sync" = 200
require_not_contains "$(tr -d '[:space:]' < "$sync_body")" '"isError":true' "runtime sync should bypass allow_list"

state_payload="$(mcp_request state tools/call '{"name":"godot.editor.state.get","arguments":{}}' "$editor_id")"
status_state="$(curl -sS -o "$state_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.editor.state.get' -X POST "$SERVER_URL" --data "$state_payload")"
test "$status_state" = 200
require_not_contains "$(tr -d '[:space:]' < "$state_body")" '"isError":true' "editor state should be allow-listed"
require_contains "$(tr -d '[:space:]' < "$state_body")" '"active_scene":"res://AllowList.tscn"' "editor state should reflect synced snapshot"

ping_payload="$(mcp_request ping tools/call '{"name":"godot.bridge.editor.ping","arguments":{}}' "$editor_id")"
status_ping="$(curl -sS -o "$ping_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.bridge.editor.ping' -X POST "$SERVER_URL" --data "$ping_payload")"
test "$status_ping" = 200
require_not_contains "$(tr -d '[:space:]' < "$ping_body")" '"isError":true' "editor ping should bypass allow_list"

ack_payload="$(mcp_request ack tools/call '{"name":"godot.bridge.command.ack","arguments":{"command_id":"cmd-not-exist","success":true,"result":{}}}' "$editor_id")"
status_ack="$(curl -sS -o "$ack_body" -w "%{http_code}" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "MCP-Protocol-Version: $PROTOCOL_VERSION" -H 'Mcp-Method: tools/call' -H 'Mcp-Name: godot.bridge.command.ack' -X POST "$SERVER_URL" --data "$ack_payload")"
test "$status_ack" = 200
ack_compact="$(tr -d '[:space:]' < "$ack_body")"
require_contains "$ack_compact" '"isError":true' "unknown command ack should be semantic error"
require_contains "$ack_compact" '"reason":"unknown_or_expired_command"' "ack should report command reason"
require_not_contains "$ack_compact" '"reason":"permission_denied"' "ack should bypass allow_list"

echo "Modern HTTP allow_list runtime bridge chain passed"
