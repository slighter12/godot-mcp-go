#!/usr/bin/env sh

# Shared payload helpers for the MCP 2026-07-28 HTTP smoke scripts.

mcp_meta() {
  mcp_editor_id="$1"
  printf '{"io.modelcontextprotocol/protocolVersion":"%s","io.modelcontextprotocol/clientInfo":{"name":"godot-mcp-smoke","version":"0.3.0"},"io.modelcontextprotocol/clientCapabilities":{"extensions":{"com.slighter12/godot-mcp":{"version":"1","editor_session_id":"%s","mutating":true},"com.slighter12/godot-mcp-command-stream":{"version":"1"}}}}' "$PROTOCOL_VERSION" "$mcp_editor_id"
}

mcp_params_with_meta() {
  mcp_params="$1"
  mcp_meta_json="$2"
  case "$mcp_params" in
    "{}")
      printf '{"_meta":%s}' "$mcp_meta_json"
      ;;
    *)
      printf '%s,"_meta":%s}' "${mcp_params%?}" "$mcp_meta_json"
      ;;
  esac
}

mcp_request() {
  mcp_id="$1"
  mcp_method="$2"
  mcp_params="$3"
  mcp_editor_id="$4"
  mcp_meta_json="$(mcp_meta "$mcp_editor_id")"
  mcp_params_json="$(mcp_params_with_meta "$mcp_params" "$mcp_meta_json")"
  printf '{"jsonrpc":"2.0","id":"%s","method":"%s","params":%s}' "$mcp_id" "$mcp_method" "$mcp_params_json"
}

mcp_notification() {
  mcp_method="$1"
  mcp_params="$2"
  mcp_editor_id="$3"
  mcp_meta_json="$(mcp_meta "$mcp_editor_id")"
  mcp_params_json="$(mcp_params_with_meta "$mcp_params" "$mcp_meta_json")"
  printf '{"jsonrpc":"2.0","method":"%s","params":%s}' "$mcp_method" "$mcp_params_json"
}
