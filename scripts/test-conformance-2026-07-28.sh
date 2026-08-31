#!/usr/bin/env sh
set -eu

GO_BIN="${GO:-go}"
SERVER_HOST="${CONFORMANCE_HOST:-127.0.0.1}"
SERVER_PORT="${CONFORMANCE_PORT:-39080}"
SERVER_URL="http://${SERVER_HOST}:${SERVER_PORT}/mcp"
OUTPUT_DIR="${CONFORMANCE_OUTPUT_DIR:-}"
temporary_output=0

fixture_dir="$(mktemp -d /tmp/godot-mcp-conformance-server.XXXXXX)"
if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$fixture_dir/output"
  temporary_output=1
fi
mkdir -p "$OUTPUT_DIR"

fixture_bin="$fixture_dir/conformance-fixture"
server_log="$fixture_dir/server.log"

cleanup() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" >/dev/null 2>&1 || true
  fi
  rm -rf "$fixture_dir"
}
trap cleanup EXIT INT TERM

"$GO_BIN" build -o "$fixture_bin" ./cmd/conformance-fixture
"$fixture_bin" --host "$SERVER_HOST" --port "$SERVER_PORT" >"$server_log" 2>&1 &
server_pid=$!

ready=0
for _ in $(seq 1 120); do
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "conformance fixture exited before readiness"
    sed -n '1,240p' "$server_log"
    exit 1
  fi
  if curl -fsS "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.25
done

if [ "$ready" -ne 1 ]; then
  echo "conformance fixture did not become ready"
  sed -n '1,240p' "$server_log"
  exit 1
fi

echo "Conformance output: $OUTPUT_DIR"
runner_status=0
bunx @modelcontextprotocol/conformance@0.2.0-alpha.11 server \
  --url "$SERVER_URL" \
  --requirements 2026-07-28 \
  --output-dir "$OUTPUT_DIR" || runner_status=$?

if [ "$runner_status" -ne 0 ]; then
  if [ "$temporary_output" -eq 1 ]; then
    retained_output="$(mktemp -d /tmp/godot-mcp-conformance-20260728.failed.XXXXXX)"
    cp -R "$OUTPUT_DIR"/. "$retained_output"/
    echo "Conformance failure output retained: $retained_output"
  else
    echo "Conformance failure output retained: $OUTPUT_DIR"
  fi
  exit "$runner_status"
fi
