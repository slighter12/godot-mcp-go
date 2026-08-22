#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
INSPECTOR_SERVER_URL="${INSPECTOR_SERVER_URL:-http://host.docker.internal:${SERVER_PORT}/mcp}"
INSPECTOR_IMAGE="${INSPECTOR_IMAGE:-ghcr.io/modelcontextprotocol/inspector:latest}"

. "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/http-test-server.sh"

log_file="$(mktemp /tmp/godot-mcp-go-inspector-negative.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-inspector-negative.config.XXXXXX.json)"
inspector_output="$(mktemp /tmp/godot-mcp-go-inspector-negative.output.XXXXXX.log)"

cleanup() {
  stop_test_server
  rm -f "$log_file" "$runtime_config" "$inspector_output"
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

if docker run --rm --add-host host.docker.internal:host-gateway --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method tools/list >"$inspector_output" 2>&1; then
  echo "expected inspector call to fail when MCP-Protocol-Version header is missing"
  cat "$inspector_output"
  exit 1
fi

if ! grep -Eq "Protocol version header does not match|Invalid request metadata|MCP-Protocol-Version" "$inspector_output"; then
  echo "expected protocol-header validation error, got:"
  cat "$inspector_output"
  exit 1
fi

echo "Inspector negative protocol-header check passed"
