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

wait_for_test_server() {
  test_server_ready=0
  for _ in $(seq 1 "${TEST_SERVER_READY_ATTEMPTS:-80}"); do
    if ! kill -0 "$server_pid" >/dev/null 2>&1; then
      echo "server process exited before readiness"
      cat "$test_server_log_file"
      return 1
    fi
    if curl -sSf "http://${SERVER_HOST}:${SERVER_PORT}/" >/dev/null 2>&1; then
      test_server_ready=1
      break
    fi
    sleep "${TEST_SERVER_READY_DELAY:-0.2}"
  done
  if [ "$test_server_ready" -ne 1 ]; then
    echo "server did not become ready in time"
    cat "$test_server_log_file"
    return 1
  fi
}

require_contains() {
  test_haystack="$1"
  test_needle="$2"
  test_label="$3"
  case "$test_haystack" in
    *"$test_needle"*) ;;
    *)
      echo "assert failed: $test_label"
      echo "expected fragment: $test_needle"
      return 1
      ;;
  esac
}

require_not_contains() {
  test_haystack="$1"
  test_needle="$2"
  test_label="$3"
  case "$test_haystack" in
    *"$test_needle"*)
      echo "assert failed: $test_label"
      echo "unexpected fragment: $test_needle"
      return 1
      ;;
    *) ;;
  esac
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
