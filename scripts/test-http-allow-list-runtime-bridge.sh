#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-19080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.config.XXXXXX.json)"
init_headers="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.init.headers.XXXXXX)"
sync_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.sync.body.XXXXXX)"
state_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.state.body.XXXXXX)"
ping_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.ping.body.XXXXXX)"
ack_body="$(mktemp /tmp/godot-mcp-go-allow-list-bridge.ack.body.XXXXXX)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$runtime_config" "$init_headers" "$sync_body" "$state_body" "$ping_body" "$ack_body"
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

require_not_contains() {
  haystack="$1"
  needle="$2"
  label="$3"
  case "$haystack" in
    *"$needle"*)
      echo "assert failed: $label"
      echo "unexpected fragment: $needle"
      exit 1
      ;;
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
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-allow-list\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"allow-list-test\",\"version\":\"0.1.0\"}}}" >/dev/null

session_id="$(awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$init_headers" | tail -n1)"
test -n "$session_id"

status_notify="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify" = "202"

status_sync="$(curl -sS -o "$sync_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"sync-allow-list","method":"tools/call","params":{"name":"godot.runtime.sync","arguments":{"snapshot":{"root_summary":{"active_scene":"res://AllowList.tscn"},"scene_tree":{"path":"/Root","name":"Root","type":"Node2D","child_count":0},"node_details":{"/Root":{"path":"/Root","name":"Root","type":"Node2D","child_count":0}}}}}}')"
test "$status_sync" = "200"
compact_sync="$(tr -d '[:space:]' < "$sync_body")"
require_not_contains "$compact_sync" '"isError":true' "runtime sync should be allowed in allow_list mode"

status_state="$(curl -sS -o "$state_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"state-allow-list","method":"tools/call","params":{"name":"godot.editor.state.get","arguments":{}}}')"
test "$status_state" = "200"
compact_state="$(tr -d '[:space:]' < "$state_body")"
require_not_contains "$compact_state" '"isError":true' "editor state should be allowed in allow_list mode"
require_contains "$compact_state" '"active_scene":"res://AllowList.tscn"' "editor state should reflect synced snapshot"

status_ping="$(curl -sS -o "$ping_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"ping-allow-list","method":"tools/call","params":{"name":"godot.runtime.ping","arguments":{}}}')"
test "$status_ping" = "200"
compact_ping="$(tr -d '[:space:]' < "$ping_body")"
require_not_contains "$compact_ping" '"isError":true' "runtime ping should be allowed in allow_list mode"

status_ack="$(curl -sS -o "$ack_body" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_id" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"ack-allow-list","method":"tools/call","params":{"name":"godot.runtime.ack","arguments":{"command_id":"cmd-not-exist","success":true,"result":{}}}}')"
test "$status_ack" = "200"
compact_ack="$(tr -d '[:space:]' < "$ack_body")"
require_contains "$compact_ack" '"isError":true' "runtime ack should report semantic error for unknown command"
require_contains "$compact_ack" '"reason":"unknown_or_expired_command"' "runtime ack should fail by command reason, not permission"
require_not_contains "$compact_ack" '"reason":"permission_denied"' "runtime ack must bypass allow_list permission"

echo "HTTP allow_list runtime bridge chain passed (session=$session_id)"
