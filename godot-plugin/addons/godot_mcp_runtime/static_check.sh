#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if rg -n 'res://addons/godot_mcp/' "$ROOT_DIR"/*.gd >/dev/null; then
  echo "runtime addon static check failed: found cross-addon dependency on godot_mcp"
  rg -n 'res://addons/godot_mcp/' "$ROOT_DIR"/*.gd
  exit 1
fi

if rg -n 'class_name (VariantUtils|RuntimeSnapshotCollector)\b' "$ROOT_DIR"/*.gd >/dev/null; then
  echo "runtime addon static check failed: found colliding global class_name in runtime addon"
  rg -n 'class_name (VariantUtils|RuntimeSnapshotCollector)\b' "$ROOT_DIR"/*.gd
  exit 1
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

echo "runtime addon static check passed"
