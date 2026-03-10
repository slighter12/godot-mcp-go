# v1 Tool Contract

This document defines the public tool contract for `godot-mcp-go`.

## Naming

Canonical tool names:

- Pattern: `^[A-Za-z0-9._-]{1,128}$`
- Shape: `godot.<domain>.<action>` or `godot.<domain>.<object>.<action>`

### Scene

- `godot.scene.list`
- `godot.scene.read`
- `godot.scene.create`
- `godot.scene.save`
- `godot.scene.apply`

### Node

- `godot.node.tree.get`
- `godot.node.properties.get`
- `godot.node.create`
- `godot.node.delete`
- `godot.node.modify`

### Script

- `godot.script.list`
- `godot.script.read`
- `godot.script.create`
- `godot.script.modify`
- `godot.script.analyze`

### Project

- `godot.project.settings.get`
- `godot.project.resources.list`
- `godot.editor.state.get`
- `godot.project.run`
- `godot.project.stop`

### Utility

- `godot.offerings.list`
- `godot.runtime.health.get`
- `godot.runtime.sync` (internal bridge)
- `godot.runtime.ping` (internal bridge)
- `godot.runtime.ack` (internal bridge)
- `godot.prompts.reload`

## Name Binding Policy

Canonical names are strictly required. Legacy aliases are rejected with `tool not found`.

## Session Lifecycle Gate

For both transports, tool execution requires lifecycle completion:

1. `initialize` succeeds with `protocolVersion=2025-11-25`
2. `initialized` or `notifications/initialized` is received
3. regular methods are accepted

Before lifecycle completion, regular requests return JSON-RPC `invalid_request` with message `Session is not initialized`.
If `initialized`/`notifications/initialized` is sent before successful `initialize`, server returns JSON-RPC `invalid_request`.

## Mutating Capability Gate

Mutating tools require session-scoped capability negotiation:

- Client must send `initialize.params.capabilities.godot.mutating=true`
- Without this capability, mutating tools return semantic error:
  - `isError=true`
  - `error.kind=not_supported`
  - `error.reason=mutating_capability_required`

Mutating tools covered by this gate:

- `godot.project.run`, `godot.project.stop`
- `godot.scene.create`, `godot.scene.save`, `godot.scene.apply`
- `godot.node.create`, `godot.node.delete`, `godot.node.modify`
- `godot.script.create`, `godot.script.modify`

## Transport Support Matrix

- `streamable_http`
  - Supports all read and mutating tools.
  - Mutating tools require initialized session + mutating capability + active runtime bridge.
- `stdio`
  - Supports non-runtime and read-oriented operations.
  - Runtime mutating command bridge paths are unavailable.

## Error Semantics

Tool result errors use semantic kinds in `result.error.kind`:

- `invalid_params`
- `not_supported`
- `not_available`
- `execution_failed`

## Mutating Tool Result Envelope

Mutating tool responses include:

- `success`
- `command_id`
- `result`
- `error`
- `acknowledged_at`

Optional metadata fields when provided by runtime ack:

- `schema_version`
- `reason`
- `retryable`

## Project Tool Contracts

### `godot.project.settings.get`

Input:

- optional `cursor`
- optional `section_prefix`

Output:

- `settings`: array of `{key, section, value, raw}`
- optional `nextCursor`

### `godot.project.resources.list`

Input:

- optional `cursor`
- optional `extensions` (array)
- optional `include_hidden` (boolean)

Output:

- `resources`: array of `{path, extension, size_bytes, modified_at}`
- optional `nextCursor`

## Script Create Conflict Policy

`godot.script.create` supports:

- `replace` (optional boolean, default `false`)

When target file exists and `replace=false`, the runtime ack envelope returns:

- `success=false`
- `reason=script_exists_requires_replace`
- `error` contains the human-readable conflict message

## Tool Controls

`tool_controls` fields:

- `schema_validation_enabled`
- `reject_unknown_arguments`
- `permission_mode` (`allow_all`, `read_only`, `allow_list`)
- `allowed_tools`
- `emit_progress_notifications`

Controls are additive and do not change canonical naming.

### Internal Bridge Permission Exemption

Runtime bridge internal tools bypass `permission_mode` filtering to preserve session health:

- `godot.runtime.sync`
- `godot.runtime.ping`
- `godot.runtime.ack`

All other tools continue to follow `allow_all` / `read_only` / `allow_list` policy rules.
