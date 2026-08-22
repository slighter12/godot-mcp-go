#!/usr/bin/env sh

# Shared server lifecycle helpers for HTTP smoke tests.

start_test_server() {
  test_server_log_file="$1"
  test_server_config_path="${2:-}"
  test_server_tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/godot-mcp-go-http-server.XXXXXX")"
  test_server_binary="$test_server_tmp_dir/server"

  "$GO_BIN" build -o "$test_server_binary" .

  if [ -n "$test_server_config_path" ]; then
    MCP_CONFIG_PATH="$test_server_config_path" MCP_PORT="$SERVER_PORT" \
      "$test_server_binary" >"$test_server_log_file" 2>&1 &
  else
    MCP_PORT="$SERVER_PORT" "$test_server_binary" >"$test_server_log_file" 2>&1 &
  fi
  server_pid=$!
}

stop_test_server() {
  if [ -n "${server_pid:-}" ]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
    server_pid=
  fi

  if [ -n "${test_server_tmp_dir:-}" ]; then
    rm -rf "$test_server_tmp_dir"
    test_server_tmp_dir=
  fi
}
