#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
INSPECTOR_SERVER_URL="${INSPECTOR_SERVER_URL:-http://host.docker.internal:${SERVER_PORT}/mcp}"
INSPECTOR_IMAGE="${INSPECTOR_IMAGE:-ghcr.io/modelcontextprotocol/inspector:latest}"
PROTOCOL_VERSION="${PROTOCOL_VERSION:-2026-07-28}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"

log_file="$(mktemp /tmp/godot-mcp-go-inspector.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-inspector.config.XXXXXX.json)"
inspector_config="$(mktemp /tmp/godot-mcp-go-inspector.cli.XXXXXX.json)"

cleanup() {
  stop_test_server
  rm -f "$log_file" "$runtime_config" "$inspector_config"
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
  if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
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

run_inspector() {
  method="$1"
  shift
  attempt=1
  while [ "$attempt" -le 5 ]; do
    if docker run --rm --no-healthcheck --add-host host.docker.internal:host-gateway \
      -v "$inspector_config:/tmp/godot-mcp-inspector.json:ro" "$INSPECTOR_IMAGE" \
      --cli --config /tmp/godot-mcp-inspector.json --server godot-mcp \
      --header "MCP-Protocol-Version: $PROTOCOL_VERSION" --method "$method" "$@" >/dev/null; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "inspector check failed: method=$method"
  cat "$log_file"
  exit 1
}

run_inspector tools/list
run_inspector resources/list
run_inspector prompts/list
run_inspector tools/call --tool-name godot.offerings.list

echo "Inspector docker checks passed"
