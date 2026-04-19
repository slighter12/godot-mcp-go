# v1 Runtime Command Protocol

This document defines the runtime command bridge contract between Go server and Godot plugin.

Tool names in this protocol are strictly canonical v1 names.

## Server -> Plugin Notification

Method:

- `notifications/godot/command`

Payload:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/godot/command",
  "params": {
    "command_id": "cmd_...",
    "name": "godot.scene.create",
    "arguments": {
      "path": "res://scenes/Main.tscn"
    }
  }
}
```

## Plugin -> Server Ack Tool

Tool name:

- `godot.bridge.command.ack`

Payload:

```json
{
  "command_id": "cmd_...",
  "success": true,
  "result": {
    "schema_version": "v1"
  },
  "retryable": false
}
```

Notes:

- Notification payload requires canonical `command_id`.
- `error`, `reason`, `retryable`, `schema_version` are optional and may be provided in top-level ack fields and/or within `result`.
- Server normalizes metadata into command responses when present.

## Runtime Log Push Tool

Tool name:

- `godot.bridge.runtime.log.push`

Payload:

```json
{
  "session_id": "game_...",
  "entries": [
    {
      "time": "2026-04-06T15:00:00+08:00",
      "level": "error",
      "message": "runtime diagnostics failure | reason=property_not_supported",
      "source": "runtime_command:godot.runtime.node_properties.get",
      "stack_trace": ""
    }
  ]
}
```

Notes:

- log push is session-scoped and must target the current game session
- `entries[].level` is normalized by the server to `debug`, `info`, `warning`, `error`, or `all`
- `entries[].stack_trace` is optional
- current diagnostics sources are:
  - `runtime_companion`
  - `runtime_lifecycle`
  - `runtime_command:<tool_name>`

## Timeout and Failure Mapping

Common bridge reasons:

- `command_transport_unavailable`
- `command_ack_timeout`
- `session_not_initialized`
- `unknown_or_expired_command`

These are surfaced as semantic `kind=not_available` at tool boundary.

## Runtime Progress Notification

When `tool_controls.emit_progress_notifications=true`, runtime command tools may emit best-effort SSE notifications:

- Method: `notifications/progress`
- Payload:

```json
{
  "jsonrpc": "2.0",
  "method": "notifications/progress",
  "params": {
    "progressToken": "tool-call-123",
    "progress": 1.0,
    "total": 1.0,
    "message": "runtime command acknowledged"
  }
}
```

Notes:

- Progress notifications are emitted only when tool calls include `_meta.progressToken`.
- Notification delivery is best-effort and transport-dependent.
- Missing progress notifications must not be treated as command failure by clients.

## Session Scope

All bridge operations are bound to initialized MCP HTTP sessions.
Acks with mismatched session/command are rejected.

Lifecycle requirements:

1. `initialize` must succeed.
2. `notifications/initialized` must be delivered before bridge tools are callable.

## Safety Expectations

Plugin command handlers must:

- Validate argument types and required fields.
- Enforce scene/script path safety (`res://`, no traversal).
- Reject node operations outside edited scene subtree.
- Return deterministic ack payload (`success`, `reason`, `retryable`, `schema_version`).

## Permission Policy Interaction

Runtime bridge internal tools are treated as transport health plumbing:

- `godot.bridge.editor.sync`
- `godot.bridge.editor.ping`
- `godot.bridge.command.ack`

These tools bypass `tool_controls.permission_mode` filters so runtime synchronization remains available in `read_only` and `allow_list` modes.
