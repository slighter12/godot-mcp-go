#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

RUNTIME_AUTOLOAD_FILES="
$ROOT_DIR/runtime_companion.gd
$ROOT_DIR/runtime_mcp_interface.gd
$ROOT_DIR/runtime_mcp_server.gd
$ROOT_DIR/runtime_streamable_http_client.gd
$ROOT_DIR/runtime_mcp_protocol_adapter.gd
$ROOT_DIR/runtime_variant_utils.gd
$ROOT_DIR/runtime_snapshot_collector.gd
"

set -- $RUNTIME_AUTOLOAD_FILES
for runtime_file in "$@"; do
  if [ ! -f "$runtime_file" ]; then
    echo "runtime addon static check failed: missing runtime script: $runtime_file"
    exit 1
  fi
done

if rg -n 'EditorInterface' "$@" >/dev/null; then
  echo "runtime addon static check failed: runtime autoload scripts reference EditorInterface"
  rg -n 'EditorInterface' "$@"
  exit 1
else
  rg_status=$?
  if [ "$rg_status" -gt 1 ]; then
    echo "runtime addon static check failed: unable to scan runtime scripts"
    exit 1
  fi
fi

if rg -n 'GDScriptFunctionState' "$ROOT_DIR/runtime_companion.gd" >/dev/null; then
  echo "runtime addon static check failed: found GDScriptFunctionState reference"
  exit 1
fi

if ! rg -n 'return "%sZ" % Time.get_datetime_string_from_system\(true\)' "$ROOT_DIR/runtime_companion.gd" >/dev/null; then
  echo "runtime addon static check failed: _now_rfc3339 is not explicit UTC RFC3339 with Z"
  exit 1
fi

if ! rg -n 'var now = _now_rfc3339\(\)' "$ROOT_DIR/runtime_snapshot_collector.gd" >/dev/null; then
  echo "runtime addon static check failed: runtime snapshot collector is not using _now_rfc3339 for updated_at"
  exit 1
fi

if ! rg -n 'return "%sZ" % Time.get_datetime_string_from_system\(true\)' "$ROOT_DIR/runtime_snapshot_collector.gd" >/dev/null; then
  echo "runtime addon static check failed: runtime snapshot collector _now_rfc3339 is not explicit UTC RFC3339 with Z"
  exit 1
fi

if ! rg -n 'const DEFAULT_PROTOCOL_VERSION := "2026-07-28"' "$ROOT_DIR/mcp_server.gd" >/dev/null; then
  echo "runtime addon static check failed: MCP client is not pinned to protocol 2026-07-28"
  exit 1
fi

if rg -n '2025-11-25|MCP-Session-Id|mcp_client\.get\("session_id"\)' \
  "$ROOT_DIR/mcp_server.gd" \
  "$ROOT_DIR/godot_mcp.gd" \
  "$ROOT_DIR/runtime_companion.gd" >/dev/null; then
  echo "runtime addon static check failed: found removed protocol/session identifiers in the modern client"
  exit 1
fi

echo "runtime addon static check passed"
