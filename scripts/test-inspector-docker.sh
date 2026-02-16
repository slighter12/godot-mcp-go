#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${SERVER_HOST:-localhost}"
SERVER_PORT="${SERVER_PORT:-9080}"
INSPECTOR_SERVER_URL="${INSPECTOR_SERVER_URL:-http://host.docker.internal:${SERVER_PORT}/mcp}"
INSPECTOR_IMAGE="${INSPECTOR_IMAGE:-ghcr.io/modelcontextprotocol/inspector:latest}"

log_file="$(mktemp /tmp/godot-mcp-go-inspector.XXXXXX.log)"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$log_file"
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

docker run --rm --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method tools/list >/dev/null
docker run --rm --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method resources/list >/dev/null
docker run --rm --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method prompts/list >/dev/null
docker run --rm --entrypoint node "$INSPECTOR_IMAGE" /app/cli/build/index.js "$INSPECTOR_SERVER_URL" --transport http --method tools/call --tool-name list-offerings >/dev/null

echo "Inspector docker checks passed"
