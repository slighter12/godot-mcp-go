#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
INSPECTOR_SERVER_URL="${INSPECTOR_SERVER_URL:-http://host.docker.internal:${SERVER_PORT}/mcp}"
INSPECTOR_IMAGE="${INSPECTOR_IMAGE:-ghcr.io/modelcontextprotocol/inspector:2.4.0}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"
INSPECTOR_COMMAND_TIMEOUT_SECONDS="${INSPECTOR_COMMAND_TIMEOUT_SECONDS:-60}"
INSPECTOR_MAX_ATTEMPTS="${INSPECTOR_MAX_ATTEMPTS:-2}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"
. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/process-utils.sh"

log_file="$(mktemp /tmp/godot-mcp-go-inspector.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-inspector.config.XXXXXX.json)"
inspector_config="$(mktemp /tmp/godot-mcp-go-inspector.cli.XXXXXX.json)"
inspector_output="$(mktemp /tmp/godot-mcp-go-inspector.output.XXXXXX.log)"
inspector_cidfile="${inspector_config}.cid"

cleanup_inspector_container() {
  if [ -f "$inspector_cidfile" ]; then
    container_id="$(sed -n '1p' "$inspector_cidfile")"
    if [ -n "$container_id" ]; then
      docker rm -f "$container_id" >/dev/null 2>&1 || true
    fi
    rm -f "$inspector_cidfile"
  fi
}

cleanup() {
  terminate_process "${RUN_WITH_DEADLINE_PID:-}"
  cleanup_inspector_container
  stop_test_server
  rm -f "$log_file" "$runtime_config" "$inspector_config" "$inspector_output"
}
trap cleanup EXIT

cp "./config/mcp_config.json" "$runtime_config"
sed -E \
  -e "s/\"host\"[[:space:]]*:[[:space:]]*\"[^\"]+\"/\"host\": \"0.0.0.0\"/" \
  -e "s/\"port\"[[:space:]]*:[[:space:]]*[0-9]+/\"port\": ${SERVER_PORT}/" \
  -e "s|\"url\"[[:space:]]*:[[:space:]]*\"http://localhost:[0-9]+/mcp\"|\"url\": \"http://localhost:${SERVER_PORT}/mcp\"|" \
  "$runtime_config" > "${runtime_config}.tmp"
mv "${runtime_config}.tmp" "$runtime_config"

start_test_server "$log_file" "$runtime_config"

ready=0
for _ in $(seq 1 120); do
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "server process exited before readiness"
    cat "$log_file"
    exit 1
  fi
  if curl --connect-timeout 1 --max-time 1 -sS "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done

if [ "$ready" -ne 1 ]; then
  echo "server did not become ready in time"
  cat "$log_file"
  exit 1
fi

printf '%s\n' \
  '{' \
  '  "mcpServers": {' \
  '    "godot-mcp": {' \
  '      "type": "streamable-http",' \
  "      \"url\": \"$INSPECTOR_SERVER_URL\"," \
  '      "protocolEra": "modern"' \
  '    }' \
  '  }' \
  '}' >"$inspector_config"
chmod 0644 "$inspector_config"

run_inspector() {
  method="$1"
  shift
  attempt=1
  while [ "$attempt" -le "$INSPECTOR_MAX_ATTEMPTS" ]; do
    rm -f "$inspector_cidfile"
    : >"$inspector_output"
    inspector_status=0
    run_with_deadline "$INSPECTOR_COMMAND_TIMEOUT_SECONDS" docker run --rm --cidfile "$inspector_cidfile" --no-healthcheck --add-host host.docker.internal:host-gateway \
      -v "$inspector_config:/tmp/godot-mcp-inspector.json:ro" "$INSPECTOR_IMAGE" \
      --cli --config /tmp/godot-mcp-inspector.json --server godot-mcp \
      --header "MCP-Protocol-Version: $PROTOCOL_VERSION" --method "$method" "$@" >"$inspector_output" 2>&1 || inspector_status=$?
    cleanup_inspector_container
    if [ "$inspector_status" -eq 0 ]; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "inspector check failed: method=$method status=$inspector_status"
  cat "$inspector_output"
  cat "$log_file"
  exit 1
}

run_inspector tools/list
run_inspector resources/list
run_inspector prompts/list
run_inspector tools/call --tool-name godot.offerings.list

echo "Inspector docker checks passed"
