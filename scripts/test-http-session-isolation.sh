#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
SERVER_URL="${SERVER_URL:-http://${SERVER_HOST}:${SERVER_PORT}/mcp}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2025-11-25}"

log_file="$(mktemp /tmp/godot-mcp-go-session-isolation.log.XXXXXX)"
init_headers_a="$(mktemp /tmp/godot-mcp-go-session-isolation.a.headers.XXXXXX)"
init_headers_b="$(mktemp /tmp/godot-mcp-go-session-isolation.b.headers.XXXXXX)"
sync_body_a="$(mktemp /tmp/godot-mcp-go-session-isolation.a.sync.body.XXXXXX)"
sync_body_b="$(mktemp /tmp/godot-mcp-go-session-isolation.b.sync.body.XXXXXX)"
state_body_a="$(mktemp /tmp/godot-mcp-go-session-isolation.a.state.body.XXXXXX)"
state_body_b="$(mktemp /tmp/godot-mcp-go-session-isolation.b.state.body.XXXXXX)"
runtime_config="$(mktemp /tmp/godot-mcp-go-session-isolation.config.XXXXXX.json)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$init_headers_a" "$init_headers_b" "$sync_body_a" "$sync_body_b" "$state_body_a" "$state_body_b" "$runtime_config"
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

extract_session_id() {
  header_file="$1"
  awk -F': ' 'tolower($1)=="mcp-session-id" {gsub("\r","",$2); print $2}' "$header_file" | tail -n1
}

cp "./config/mcp_config.json" "$runtime_config"
sed -E "s/\"port\"[[:space:]]*:[[:space:]]*[0-9]+/\"port\": ${SERVER_PORT}/" "$runtime_config" > "${runtime_config}.tmp"
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

if ! kill -0 "$server_pid" >/dev/null 2>&1; then
  echo "server process exited unexpectedly"
  cat "$log_file"
  exit 1
fi

curl -sS -D "$init_headers_a" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -X POST "$SERVER_URL" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-session-a\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"make-session-a\",\"version\":\"0.1.0\"}}}" >/dev/null

curl -sS -D "$init_headers_b" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -X POST "$SERVER_URL" \
  --data "{\"jsonrpc\":\"2.0\",\"id\":\"init-session-b\",\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$PROTOCOL_VERSION\",\"capabilities\":{},\"clientInfo\":{\"name\":\"make-session-b\",\"version\":\"0.1.0\"}}}" >/dev/null

session_a="$(extract_session_id "$init_headers_a")"
session_b="$(extract_session_id "$init_headers_b")"
test -n "$session_a"
test -n "$session_b"
test "$session_a" != "$session_b"

status_notify_a="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_a" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify_a" = "202"

status_notify_b="$(curl -sS -o /dev/null -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_b" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}')"
test "$status_notify_b" = "202"

status_sync_a="$(curl -sS -o "$sync_body_a" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_a" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"sync-a","method":"tools/call","params":{"name":"sync-editor-runtime","arguments":{"snapshot":{"root_summary":{"active_scene":"res://SessionA.tscn","active_script":"res://scripts/A.gd"},"scene_tree":{"path":"/RootA","name":"RootA","type":"Node2D","child_count":0},"node_details":{"/RootA":{"path":"/RootA","name":"RootA","type":"Node2D","child_count":0}}}}}}')"
test "$status_sync_a" = "200"

status_sync_b="$(curl -sS -o "$sync_body_b" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_b" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"sync-b","method":"tools/call","params":{"name":"sync-editor-runtime","arguments":{"snapshot":{"root_summary":{"active_scene":"res://SessionB.tscn","active_script":"res://scripts/B.gd"},"scene_tree":{"path":"/RootB","name":"RootB","type":"Node2D","child_count":0},"node_details":{"/RootB":{"path":"/RootB","name":"RootB","type":"Node2D","child_count":0}}}}}}')"
test "$status_sync_b" = "200"

compact_sync_a="$(tr -d '[:space:]' < "$sync_body_a")"
compact_sync_b="$(tr -d '[:space:]' < "$sync_body_b")"
require_not_contains "$compact_sync_a" '"isError":true' "sync a should not be error"
require_not_contains "$compact_sync_b" '"isError":true' "sync b should not be error"

status_state_a="$(curl -sS -o "$state_body_a" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_a" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"state-a","method":"tools/call","params":{"name":"get-editor-state","arguments":{}}}')"
test "$status_state_a" = "200"

status_state_b="$(curl -sS -o "$state_body_b" -w "%{http_code}" \
  -H 'Content-Type: application/json' \
  -H "MCP-Protocol-Version: $PROTOCOL_VERSION" \
  -H "MCP-Session-Id: $session_b" \
  -X POST "$SERVER_URL" \
  --data '{"jsonrpc":"2.0","id":"state-b","method":"tools/call","params":{"name":"get-editor-state","arguments":{}}}')"
test "$status_state_b" = "200"

compact_state_a="$(tr -d '[:space:]' < "$state_body_a")"
compact_state_b="$(tr -d '[:space:]' < "$state_body_b")"

require_not_contains "$compact_state_a" '"isError":true' "state a should not be error"
require_not_contains "$compact_state_b" '"isError":true' "state b should not be error"

require_contains "$compact_state_a" '"active_scene":"res://SessionA.tscn"' "session a should read scene a"
require_contains "$compact_state_b" '"active_scene":"res://SessionB.tscn"' "session b should read scene b"
require_not_contains "$compact_state_a" 'res://SessionB.tscn' "session a should not read scene b"
require_not_contains "$compact_state_b" 'res://SessionA.tscn' "session b should not read scene a"

echo "HTTP session isolation passed (session_a=$session_a session_b=$session_b)"
