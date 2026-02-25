#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
INSPECTOR_SERVER_URL="${INSPECTOR_SERVER_URL:-http://host.docker.internal:${SERVER_PORT}/mcp}"
INSPECTOR_IMAGE="${INSPECTOR_IMAGE:-ghcr.io/modelcontextprotocol/inspector:latest}"

log_file="$(mktemp /tmp/godot-mcp-go-inspector.XXXXXX.log)"
runtime_config="$(mktemp /tmp/godot-mcp-go-inspector.config.XXXXXX.json)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -f "$log_file" "$runtime_config"
}
trap cleanup EXIT

cp "./config/mcp_config.json" "$runtime_config"
sed -E \
  -e "s/\"port\"[[:space:]]*:[[:space:]]*[0-9]+/\"port\": ${SERVER_PORT}/" \
  -e "s|\"url\"[[:space:]]*:[[:space:]]*\"http://localhost:[0-9]+/mcp\"|\"url\": \"http://localhost:${SERVER_PORT}/mcp\"|" \
  "$runtime_config" > "${runtime_config}.tmp"
mv "${runtime_config}.tmp" "$runtime_config"

MCP_CONFIG_PATH="$runtime_config" "$GO_BIN" run main.go >"$log_file" 2>&1 &
server_pid=$!

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

run_inspector() {
  method="$1"
  shift
  attempt=1
  while [ "$attempt" -le 5 ]; do
    if docker run --rm --add-host host.docker.internal:host-gateway --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method "$method" "$@" >/dev/null; then
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
run_inspector tools/call --tool-name godot-offerings-list

echo "Inspector docker checks passed"
