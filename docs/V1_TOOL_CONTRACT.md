# v1 Tool Contract

This document defines the v1 public tool contract for `godot-mcp-go`.

## Naming

v1 canonical tool names:

### Scene
- `godot-scene-list`
- `godot-scene-read`
- `godot-scene-create`
- `godot-scene-save`
- `godot-scene-apply`

### Node
- `godot-node-get-tree`
- `godot-node-get-properties`
- `godot-node-create`
- `godot-node-delete`
- `godot-node-modify`

### Script
- `godot-script-list`
- `godot-script-read`
- `godot-script-create`
- `godot-script-modify`
- `godot-script-analyze`

### Project
- `godot-project-get-settings`
- `godot-project-list-resources`
- `godot-editor-get-state`
- `godot-project-run`
- `godot-project-stop`

### Utility
- `godot-offerings-list`
- `godot-runtime-sync` (internal bridge)
- `godot-runtime-ping` (internal bridge)
- `godot-runtime-ack` (internal bridge)
- `godot-prompts-reload`

## Name Binding Policy

Canonical tool names are strictly required.
Legacy tool names are rejected with `tool not found`.

## Transport Support Matrix

- `streamable_http`
  - Supports all read and mutating tools.
  - Mutating tools require initialized MCP session and active Godot plugin bridge.
- `stdio`
  - Supports non-runtime and read-oriented tool operations.
  - Mutating runtime tools that require `_mcp` session context return semantic `kind=not_available`.

## Error Semantics

Tool result errors use semantic kinds in `result.error.kind`:
- `invalid_params`
- `not_supported`
- `not_available`
- `execution_failed`

Non-semantic runtime failures are normalized to `execution_failed`.

## Mutating Tool Result Envelope

Mutating tool success/failure responses include:
- `success`
- `command_id`
- `result`
- `error`
- `acknowledged_at`

When present from runtime ack metadata:
- `schema_version`
- `reason`
- `retryable`
