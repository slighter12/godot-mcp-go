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
    "name": "godot-scene-create",
    "arguments": {
      "path": "res://scenes/Main.tscn"
    }
  }
}
```

## Plugin -> Server Ack Tool

Tool name:
- `godot-runtime-ack`

Payload:

```json
{
  "command_id": "cmd_...",
  "success": true,
  "result": {
    "schema_version": "v1"
  },
  "error": "",
  "reason": "",
  "retryable": false
}
```

Notes:
- Notification payload accepts `command_id` (canonical) and `commandId` (backward-compatible alias).
- `reason`, `retryable`, `schema_version` are optional and may be provided in top-level ack fields and/or within `result`.
- Server normalizes metadata into command responses when present.

## Timeout and Failure Mapping

Common bridge reasons:
- `command_transport_unavailable`
- `command_ack_timeout`
- `session_not_initialized`
- `unknown_or_expired_command`

These are surfaced as semantic `kind=not_available` at tool boundary.

## Session Scope

All bridge operations are bound to initialized MCP HTTP sessions.
Acks with mismatched session/command are rejected.

## Safety Expectations

Plugin command handlers must:
- Validate argument types and required fields.
- Enforce scene/script path safety (`res://`, no traversal).
- Reject node operations outside edited scene subtree.
- Return deterministic ack payload (`success`, `reason`, `retryable`, `schema_version`).
